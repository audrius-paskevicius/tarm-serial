package serial

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Port struct {
	handle    windows.Handle
	rl, wl    sync.Mutex
	ro, wo    windows.Overlapped
	mu        sync.Mutex // protects submission against Close/Flush; never held during completion waits
	closing   bool
	pending   sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func openPort(name string, baud int, databits byte, parity Parity, stopbits StopBits, readTimeout time.Duration) (_ *Port, err error) {
	if !strings.HasPrefix(name, "\\\\") {
		name = "\\\\.\\" + name
	}
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	p := &Port{handle: h}
	defer func() {
		if err != nil {
			if p.ro.HEvent != 0 {
				err = errors.Join(err, windows.CloseHandle(p.ro.HEvent))
			}
			if p.wo.HEvent != 0 {
				err = errors.Join(err, windows.CloseHandle(p.wo.HEvent))
			}
			err = errors.Join(err, windows.CloseHandle(h))
		}
	}()
	dcb := windows.DCB{DCBlength: uint32(unsafe.Sizeof(windows.DCB{})), BaudRate: uint32(baud),
		Flags: 0x01 | 0x10, ByteSize: databits} // binary mode, DTR enabled, no flow control
	switch parity {
	case ParityNone:
		dcb.Parity = windows.NOPARITY
	case ParityOdd:
		dcb.Parity = windows.ODDPARITY
	case ParityEven:
		dcb.Parity = windows.EVENPARITY
	case ParityMark:
		dcb.Parity = windows.MARKPARITY
	case ParitySpace:
		dcb.Parity = windows.SPACEPARITY
	}
	switch stopbits {
	case Stop1:
		dcb.StopBits = windows.ONESTOPBIT
	case Stop1Half:
		dcb.StopBits = windows.ONE5STOPBITS
	case Stop2:
		dcb.StopBits = windows.TWOSTOPBITS
	}
	if err = windows.SetCommState(h, &dcb); err != nil {
		return nil, err
	}
	if err = windows.SetupComm(h, 64, 64); err != nil {
		return nil, err
	}
	const maxDWORD = uint32(1<<32 - 1)
	ms := int64(maxDWORD - 1)
	if readTimeout > 0 {
		ms = max(1, min(readTimeout.Milliseconds(), int64(maxDWORD-1)))
	}
	timeouts := windows.CommTimeouts{ReadIntervalTimeout: maxDWORD, ReadTotalTimeoutMultiplier: maxDWORD,
		ReadTotalTimeoutConstant: uint32(ms)}
	if err = windows.SetCommTimeouts(h, &timeouts); err != nil {
		return nil, err
	}
	if p.ro.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		return nil, err
	}
	if p.wo.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		return nil, err
	}
	diagnosticPort("open", h, name)
	return p, nil
}

func (p *Port) Close() error {
	if p == nil {
		return os.ErrInvalid
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		diagnosticPort("close_begin", p.handle, "")
		err := windows.CancelIoEx(p.handle, nil)
		if err == windows.ERROR_NOT_FOUND {
			err = nil
		}
		p.mu.Unlock()
		p.pending.Wait()
		p.closeErr = errors.Join(err, windows.CloseHandle(p.ro.HEvent), windows.CloseHandle(p.wo.HEvent), windows.CloseHandle(p.handle))
		diagnosticPort("close_end", p.handle, "")
	})
	return p.closeErr
}

func (p *Port) Read(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	p.rl.Lock()
	defer p.rl.Unlock()
	return p.transfer(b, &p.ro, false)
}

func (p *Port) Write(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	p.wl.Lock()
	defer p.wl.Unlock()
	return p.transfer(b, &p.wo, true)
}

// The direction lock owns ev through completion. The lifecycle lock covers the
// native submission so Close cannot cancel and then miss a newly submitted I/O.
func (p *Port) transfer(b []byte, ev *windows.Overlapped, writing bool) (int, error) {
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return 0, os.ErrClosed
	}
	if len(b) == 0 {
		p.mu.Unlock()
		return 0, nil
	}
	event := ev.HEvent
	*ev = windows.Overlapped{HEvent: event}
	if err := windows.ResetEvent(event); err != nil {
		p.mu.Unlock()
		return 0, err
	}
	var preliminary uint32 // x/sys's race-enabled wrapper requires this pointer
	started := int64(0)
	var submitErr error
	if writing {
		started = diagnosticWriteStart()
		submitErr = windows.WriteFile(p.handle, b, &preliminary, ev)
	} else {
		submitErr = windows.ReadFile(p.handle, b, &preliminary, ev)
	}
	if submitErr != nil && submitErr != windows.ERROR_IO_PENDING {
		p.mu.Unlock()
		if writing {
			diagnosticWriteResult(started, p.handle, len(b), preliminary, submitErr, false, 0, nil, ev)
		}
		return int(preliminary), submitErr
	}
	p.pending.Add(1)
	p.mu.Unlock()
	defer p.pending.Done()
	var completed uint32
	err := windows.GetOverlappedResult(p.handle, ev, &completed, true)
	runtime.KeepAlive(b) // native I/O retains the buffer beyond the submission syscall
	if writing {
		diagnosticWriteResult(started, p.handle, len(b), preliminary, submitErr, true, completed, err, ev)
		if err == nil && int(completed) < len(b) {
			err = io.ErrShortWrite
		}
	}
	return int(completed), err
}

// Flush discards queued input/output; it does not wait for transmission.
func (p *Port) Flush() error {
	if p == nil {
		return os.ErrInvalid
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return os.ErrClosed
	}
	return windows.PurgeComm(p.handle, windows.PURGE_RXABORT|windows.PURGE_TXABORT|windows.PURGE_RXCLEAR|windows.PURGE_TXCLEAR)
}
