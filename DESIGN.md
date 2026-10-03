# Serial ownership and completion

This fork retains the API and BSD-3-Clause license of [Tarm serial](https://github.com/tarm/serial),
starting at `98f6abe2eb07edd42f6dfa2a934aea469acc29b7`. The package name remains `serial`.

Each open port owns its native handle. Reads and writes may run concurrently;
operations in one direction are serialized. Close rejects new operations, interrupts
pending I/O, waits for ownership to return, then releases resources. Callers must
close ports explicitly. Concurrent and repeated Close calls return the same result.
An OS driver that does not complete cancellation can delay Close; freeing memory
still owned by that driver is not a valid timeout fallback.

Windows keeps one manual-reset event and OVERLAPPED structure per direction.
Submission and cancellation share the lifecycle lock, preventing a new submission
after cancellation. Completion waits do not hold that lock. Close waits for all
submitted operations before closing either event or the port handle. Open failures
release every resource already acquired. OVERLAPPED state is reset only after the
previous operation has completed.

ReadFile/WriteFile immediate success and ERROR_IO_PENDING both lead to an
authoritative GetOverlappedResult count. Errors, including cancellation, remain
visible. A short successful write becomes io.ErrShortWrite with its actual count;
no bytes are replayed. A returned byte count is not a device acknowledgement.
Application protocols own uncertain mutations, deadlines and recovery.

Positive ReadTimeout means an idle read returns (0, nil); zero means wait for data.
Unix uses nonblocking descriptors with Go's file poller and read deadlines, so
timeout is distinguishable from EOF/disconnection and Close can wake an idle reader.
It does not suppress EOF as a timeout. Windows retains Tarm's COMMTIMEOUTS policy.
Empty reads/writes return immediately on an open port.

Flush retains Tarm's discard semantics: discard queued input/output and, on
Windows, abort pending I/O. It is not a transmit-drain operation or protocol barrier.
Port setup retains Tarm's Windows DTR-on/RTS-off policy, disabled flow control and
64-byte driver queue requests. Queue requests are hints, not throughput promises.

The serialdiagnostic build tag logs Windows lifecycle and anomalous native write
completion to stderr, without payloads, successful-operation logging or retries.
Ordinary builds compile these observers out. Native status is captured before
turning a short successful write into io.ErrShortWrite.

Windows and Linux do not require CGo; macOS and BSD retain the upstream CGo
termios backend. Hardware qualification is platform/device specific. Neither
successful compilation nor a successful workload proves the earlier intermittent
Windows zero-byte write has been explained. High-speed tuning is separate work.
