package serial

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A real Windows pipe controls pending writes without sending incomplete commands
// to a device. It exercises the same overlapped submission/completion/Close path.
func pipePort(t *testing.T) (*Port, windows.Handle) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("\\\\.\\pipe\\tarm-serial-%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		windows.CloseHandle(h)
		t.Fatal(err)
	}
	p := &Port{handle: h}
	p.ro.HEvent, err = windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		windows.CloseHandle(peer)
		t.Fatal(err)
	}
	p.wo.HEvent, err = windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(p.ro.HEvent)
		windows.CloseHandle(h)
		windows.CloseHandle(peer)
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); windows.CloseHandle(peer) })
	return p, peer
}

func waitPending(t *testing.T, p *Port, ev *windows.Overlapped) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		var n uint32
		err := windows.GetOverlappedResult(p.handle, ev, &n, false)
		p.mu.Unlock()
		if err == windows.ERROR_IO_INCOMPLETE {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not become pending")
}

func TestCloseCancelsPendingIO(t *testing.T) {
	for _, writing := range []bool{false, true} {
		t.Run(fmt.Sprint("write=", writing), func(t *testing.T) {
			p, _ := pipePort(t)
			done := make(chan error, 1)
			go func() {
				var err error
				if writing {
					_, err = p.Write(make([]byte, 1<<20))
				} else {
					_, err = p.Read(make([]byte, 64))
				}
				done <- err
			}()
			ev := &p.ro
			if writing {
				ev = &p.wo
			}
			waitPending(t, p, ev)
			var closes sync.WaitGroup
			for i := 0; i < 8; i++ {
				closes.Add(1)
				go func() {
					defer closes.Done()
					if err := p.Close(); err != nil {
						t.Error(err)
					}
				}()
			}
			select {
			case err := <-done:
				if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
					t.Fatalf("pending result: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Close did not release pending I/O")
			}
			closes.Wait()
			for _, h := range []windows.Handle{p.handle, p.ro.HEvent, p.wo.HEvent} {
				if _, err := windows.WaitForSingleObject(h, 0); err != windows.ERROR_INVALID_HANDLE {
					t.Errorf("handle not closed: %v", err)
				}
			}
			if _, err := p.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
				t.Errorf("read after close: %v", err)
			}
			if _, err := p.Write([]byte{1}); !errors.Is(err, os.ErrClosed) {
				t.Errorf("write after close: %v", err)
			}
		})
	}
}

func TestOverlappedRoundTrip(t *testing.T) {
	p, peer := pipePort(t)
	data := bytes.Repeat([]byte("completion-count"), 1024)
	received := make(chan []byte, 1)
	go func() {
		got := make([]byte, 0, len(data))
		buf := make([]byte, 257)
		for len(got) < len(data) {
			var n uint32
			err := windows.ReadFile(peer, buf, &n, nil)
			if err != nil {
				received <- nil
				return
			}
			got = append(got, buf[:n]...)
		}
		received <- got
	}()
	n, err := p.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write: %d %v", n, err)
	}
	if got := <-received; !bytes.Equal(got, data) {
		t.Fatal("write content mismatch")
	}
	// Concurrent delivery and a confirmed pending read exercise completion counts.
	for _, pending := range []bool{false, true} {
		result := make(chan error, 1)
		read := func() {
			b := make([]byte, len(data))
			_, err := io.ReadFull(p, b)
			if err == nil && !bytes.Equal(b, data) {
				err = fmt.Errorf("read mismatch")
			}
			result <- err
		}
		if pending {
			go read()
			waitPending(t, p, &p.ro)
		}
		writeDone := make(chan error, 1)
		go func() {
			var n uint32
			err := windows.WriteFile(peer, data, &n, nil)
			if err == nil && int(n) != len(data) {
				err = io.ErrShortWrite
			}
			writeDone <- err
		}()
		if !pending {
			go read()
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if err := <-writeDone; err != nil {
			t.Fatal(err)
		}
	}
}
