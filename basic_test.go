//go:build linux

package serial

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPTYTimeoutAndClose(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	fd := int(master.Fd())
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/dev/pts/%d", number)
	p, err := OpenPort(&Config{Name: path, Baud: 115200, ReadTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	started := time.Now()
	n, err := p.Read(make([]byte, 16))
	if n != 0 || err != nil || time.Since(started) < 40*time.Millisecond {
		t.Fatalf("idle: %d %v %v", n, err, time.Since(started))
	}
	if _, err = master.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err = p.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("data: %q %v", buf[:n], err)
	}
	p.Close()
	p, err = OpenPort(&Config{Name: path, Baud: 115200})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	done := make(chan error, 1)
	go func() { _, err := p.Read(buf); done <- err }()
	time.Sleep(20 * time.Millisecond)
	p.Close()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked reader")
	}
	p, err = OpenPort(&Config{Name: path, Baud: 115200, ReadTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	master.Close()
	_, err = p.Read(buf)
	if err == nil {
		t.Fatal("disconnect hidden as timeout")
	}
}
