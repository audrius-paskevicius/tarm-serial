# tarm-serial

A fork of [tarm/serial](https://github.com/tarm/serial), by Tarm and contributors.
The original [BSD-3-Clause license](LICENSE) is retained. The Go package remains
named `serial`.

```go
import (
    "time"
    serial "github.com/audrius-paskevicius/tarm-serial"
)

port, err := serial.OpenPort(&serial.Config{
    Name: "COM7", Baud: 115200, ReadTimeout: 100 * time.Millisecond,
})
if err != nil { return err }
defer port.Close()
```

Go 1.26 or newer is required. Defaults are eight data bits, no parity, one stop
bit, and no flow control. Read and Write can run concurrently. Always inspect
both the byte count and error. Positive ReadTimeout returns `(0, nil)` on expiry;
zero waits for data. Close interrupts pending I/O. Flush discards queued bytes;
it does not drain transmission. Applications own protocol recovery and must not
automatically replay mutations after an uncertain write.

[DESIGN.md](DESIGN.md) defines handle ownership, completion, cancellation,
timeouts and platform differences. This fork fixes Windows event lifetime and
close coordination and uses Go polling/deadlines on Unix instead of treating
idle timeout as EOF. It retains one Windows event per direction. This is not a
claim of higher throughput or proof that a particular Windows USB write failure
has been fixed.

Windows and Linux build without CGo. macOS and BSD retain the upstream CGo
termios implementation; BSD has lower validation priority. Windows accepts
driver-supported baud rates, Linux retains Tarm's rate table, and the CGo backend
retains its standard rates through 115200. High-speed support and actual reliable
rates must be qualified separately.

Run `go test ./...` and `go vet ./...`. Build with `-tags=serialdiagnostic` for
Windows anomaly/lifecycle JSON on stderr; payloads and successful writes are
not logged. Real-port tests require an explicitly selected `SERIAL_TEST_PORT`
and exclusive ownership. Tests and compile checks do not replace device testing.
