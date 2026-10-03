//go:build serialdiagnostic

package serial

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func diagnosticWriteStart() int64 { return time.Now().UnixNano() }

func diagnosticEmit(fields map[string]any) {
	fields["at"] = time.Now().UTC()
	data, _ := json.Marshal(fields)
	fmt.Fprintln(os.Stderr, string(data))
}

func diagnosticPort(event string, handle windows.Handle, name string) {
	diagnosticEmit(map[string]any{"native_serial": event, "handle": uint64(handle), "port": name})
}

// Capture the raw completion facts only on short writes or errors. Querying
// communication error state would clear it, so this observer makes no such call.
func diagnosticWriteResult(started int64, handle windows.Handle, requested int, initial uint32, initialErr error, queried bool, completed uint32, completionErr error, ev *windows.Overlapped) {
	if queried && completionErr == nil && int(completed) == requested {
		return
	}
	fields := map[string]any{
		"native_serial": "write", "started_unix_ns": started, "handle": uint64(handle),
		"requested": requested, "preliminary": initial, "write_error": fmt.Sprint(initialErr),
		"completion_queried": queried, "completed": completed, "completion_error": fmt.Sprint(completionErr),
		"event": uint64(ev.HEvent), "elapsed_ns": time.Now().UnixNano() - started,
	}
	if e, ok := initialErr.(windows.Errno); ok {
		fields["write_errno"] = uint32(e)
	}
	if e, ok := completionErr.(windows.Errno); ok {
		fields["completion_errno"] = uint32(e)
	}
	// Only inspect OVERLAPPED status after a completion query succeeded.
	if queried && completionErr == nil {
		fields["internal"], fields["internal_high"] = uint64(ev.Internal), uint64(ev.InternalHigh)
	}
	diagnosticEmit(fields)
}
