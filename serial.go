// Package serial provides byte-stream serial ports. This is a maintained fork of
// github.com/tarm/serial; see README.md and DESIGN.md for platform semantics.
package serial

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultSize = 8 // Default value for Config.Size

type StopBits byte
type Parity byte

const (
	Stop1     StopBits = 1
	Stop1Half StopBits = 15
	Stop2     StopBits = 2
)

const (
	ParityNone  Parity = 'N'
	ParityOdd   Parity = 'O'
	ParityEven  Parity = 'E'
	ParityMark  Parity = 'M' // parity bit is always 1
	ParitySpace Parity = 'S' // parity bit is always 0
)

// Config contains the information needed to open a serial port.
//
// Currently few options are implemented, but more may be added in the
// future (patches welcome), so it is recommended that you create a
// new config addressing the fields by name rather than by order.
//
// For example:
//
//	c0 := &serial.Config{Name: "COM45", Baud: 115200, ReadTimeout: time.Millisecond * 500}
//
// or
//
//	c1 := new(serial.Config)
//	c1.Name = "/dev/tty.usbserial"
//	c1.Baud = 115200
//	c1.ReadTimeout = time.Millisecond * 500
type Config struct {
	Name        string
	Baud        int
	ReadTimeout time.Duration // Idle read timeout; zero waits for data, positive expiry returns (0, nil).

	// Size is the number of data bits. If 0, DefaultSize is used.
	Size byte

	// Parity is the bit to use and defaults to ParityNone (no parity bit).
	Parity Parity

	// Number of stop bits to use. Default is 1 (1 stop bit).
	StopBits StopBits
}

// ErrBadSize is returned if Size is not supported.
var ErrBadSize error = errors.New("unsupported serial data size")

// ErrBadStopBits is returned if the specified StopBits setting not supported.
var ErrBadStopBits error = errors.New("unsupported stop bit setting")

// ErrBadParity is returned if the parity is not supported.
var ErrBadParity error = errors.New("unsupported parity setting")

// OpenPort opens a serial port with the specified configuration
func OpenPort(c *Config) (*Port, error) {
	if c == nil {
		return nil, fmt.Errorf("nil serial configuration")
	}
	if c.Name == "" || strings.ContainsRune(c.Name, 0) {
		return nil, fmt.Errorf("invalid serial port name")
	}
	if c.Baud <= 0 || uint64(c.Baud) > 1<<32-1 {
		return nil, fmt.Errorf("invalid baud rate: %d", c.Baud)
	}
	if c.ReadTimeout < 0 {
		return nil, fmt.Errorf("negative read timeout")
	}
	size, par, stop := c.Size, c.Parity, c.StopBits
	if size == 0 {
		size = DefaultSize
	}
	if par == 0 {
		par = ParityNone
	}
	if stop == 0 {
		stop = Stop1
	}
	if size < 5 || size > 8 {
		return nil, ErrBadSize
	}
	switch par {
	case ParityNone, ParityOdd, ParityEven, ParityMark, ParitySpace:
	default:
		return nil, ErrBadParity
	}
	switch stop {
	case Stop1, Stop1Half, Stop2:
	default:
		return nil, ErrBadStopBits
	}
	return openPort(c.Name, c.Baud, size, par, stop, c.ReadTimeout)
}
