//go:build !windows && !linux && cgo

package serial

// #include <termios.h>
// #include <unistd.h>
import "C"

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func openPort(name string, baud int, databits byte, parity Parity, stopbits StopBits, readTimeout time.Duration) (p *Port, err error) {
	if parity != ParityNone && parity != ParityOdd && parity != ParityEven {
		return nil, ErrBadParity
	}
	if stopbits != Stop1 && stopbits != Stop2 {
		return nil, ErrBadStopBits
	}
	f, err := os.OpenFile(name, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0666)
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	rawFD, err := descriptor(f)
	if err != nil {
		return nil, err
	}
	fd := C.int(rawFD)
	if C.isatty(fd) != 1 {
		return nil, errors.New("File is not a tty")
	}

	var st C.struct_termios
	if result, callErr := C.tcgetattr(fd, &st); result != 0 {
		return nil, callErr
	}
	var speed C.speed_t
	switch baud {
	case 115200:
		speed = C.B115200
	case 57600:
		speed = C.B57600
	case 38400:
		speed = C.B38400
	case 19200:
		speed = C.B19200
	case 9600:
		speed = C.B9600
	case 4800:
		speed = C.B4800
	case 2400:
		speed = C.B2400
	case 1200:
		speed = C.B1200
	case 600:
		speed = C.B600
	case 300:
		speed = C.B300
	case 200:
		speed = C.B200
	case 150:
		speed = C.B150
	case 134:
		speed = C.B134
	case 110:
		speed = C.B110
	case 75:
		speed = C.B75
	case 50:
		speed = C.B50
	default:
		return nil, fmt.Errorf("Unknown baud rate %v", baud)
	}

	if result, callErr := C.cfsetispeed(&st, speed); result != 0 {
		return nil, callErr
	}
	if result, callErr := C.cfsetospeed(&st, speed); result != 0 {
		return nil, callErr
	}

	// Turn off break interrupts, CR->NL, Parity checks, strip, and IXON
	st.c_iflag &= ^C.tcflag_t(C.BRKINT | C.ICRNL | C.INPCK | C.ISTRIP | C.IXOFF | C.IXON | C.PARMRK)

	// Select local mode, turn off parity, set to 8 bits
	st.c_cflag &= ^C.tcflag_t(C.CSIZE | C.PARENB | C.CSTOPB)
	st.c_cflag |= (C.CLOCAL | C.CREAD)
	// databits
	switch databits {
	case 5:
		st.c_cflag |= C.CS5
	case 6:
		st.c_cflag |= C.CS6
	case 7:
		st.c_cflag |= C.CS7
	case 8:
		st.c_cflag |= C.CS8
	default:
		return nil, ErrBadSize
	}
	// Parity settings
	switch parity {
	case ParityNone:
		// default is no parity
	case ParityOdd:
		st.c_cflag |= C.PARENB
		st.c_cflag |= C.PARODD
	case ParityEven:
		st.c_cflag |= C.PARENB
		st.c_cflag &= ^C.tcflag_t(C.PARODD)
	default:
		return nil, ErrBadParity
	}
	// Stop bits settings
	switch stopbits {
	case Stop1:
		// as is, default is 1 bit
	case Stop2:
		st.c_cflag |= C.CSTOPB
	default:
		return nil, ErrBadStopBits
	}
	// Select raw mode
	st.c_lflag &= ^C.tcflag_t(C.ICANON | C.ECHO | C.ECHOE | C.ISIG)
	st.c_oflag &= ^C.tcflag_t(C.OPOST)

	// Keep the descriptor nonblocking for Go's file poller.
	st.c_cc[C.VMIN] = 1
	st.c_cc[C.VTIME] = 0

	if result, callErr := C.tcsetattr(fd, C.TCSANOW, &st); result != 0 {
		return nil, callErr
	}

	if err = f.SetReadDeadline(time.Time{}); err != nil {
		return nil, err
	}

	return &Port{f: f, readTimeout: readTimeout}, nil
}

func flush(fd uintptr) error {
	if result, err := C.tcflush(C.int(fd), C.TCIOFLUSH); result != 0 {
		return err
	}
	return nil
}
