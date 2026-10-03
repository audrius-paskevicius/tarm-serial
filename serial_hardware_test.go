package serial

import (
	"errors"
	"os"
	"testing"
	"time"
)

// Explicit opt-in: no bytes are transmitted and no device configuration beyond
// ordinary serial open is changed. Requires exclusive port ownership.
func TestHardwareIdleClose(t *testing.T) {
	name := os.Getenv("SERIAL_TEST_PORT")
	if name == "" {
		t.Skip("set SERIAL_TEST_PORT for real-port validation")
	}
	for i := 0; i < 25; i++ {
		p, err := OpenPort(&Config{Name: name, Baud: 115200, ReadTimeout: 50 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1024)
		deadline := time.Now().Add(3 * time.Second)
		for {
			n, err := p.Read(buf)
			if err != nil {
				p.Close()
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
			if time.Now().After(deadline) {
				p.Close()
				t.Fatal("port never became idle")
			}
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
		p, err = OpenPort(&Config{Name: name, Baud: 115200})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			for {
				_, err := p.Read(buf)
				if err != nil {
					done <- err
					return
				}
			}
		}()
		time.Sleep(10 * time.Millisecond)
		closed := make(chan error, 1)
		go func() { closed <- p.Close() }()
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Close blocked")
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("read survived Close")
		}
		if _, err := p.Read(buf); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("read after Close: %v", err)
		}
	}
}
