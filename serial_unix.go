//go:build linux || (!windows && cgo)

package serial

import (
	"errors"
	"os"
	"sync"
	"time"
)

// Retain nonblocking mode: File.Fd would switch the descriptor to blocking mode.
func descriptor(f *os.File) (uintptr, error) {
	raw, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	var fd uintptr
	err = raw.Control(func(h uintptr) { fd = h })
	return fd, err
}

type Port struct {
	f           *os.File
	readTimeout time.Duration
	rl          sync.Mutex
	closeOnce   sync.Once
	closeErr    error
}

func (p *Port) Read(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	p.rl.Lock()
	defer p.rl.Unlock()
	if p.readTimeout > 0 {
		if err := p.f.SetReadDeadline(time.Now().Add(p.readTimeout)); err != nil {
			return 0, err
		}
	}
	n, err := p.f.Read(b)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		err = nil
	}
	return n, err
}

func (p *Port) Write(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	return p.f.Write(b)
}

// Flush discards queued input/output; it does not wait for transmission.
func (p *Port) Flush() error {
	if p == nil {
		return os.ErrInvalid
	}
	raw, err := p.f.SyscallConn()
	if err != nil {
		return err
	}
	var flushErr error
	err = raw.Control(func(fd uintptr) { flushErr = flush(fd) })
	return errors.Join(err, flushErr)
}

func (p *Port) Close() error {
	if p == nil {
		return os.ErrInvalid
	}
	p.closeOnce.Do(func() { p.closeErr = p.f.Close() })
	return p.closeErr
}
