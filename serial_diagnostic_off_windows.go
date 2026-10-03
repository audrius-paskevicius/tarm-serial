//go:build !serialdiagnostic

package serial

import "golang.org/x/sys/windows"

func diagnosticWriteStart() int64 { return 0 }
func diagnosticWriteResult(int64, windows.Handle, int, uint32, error, bool, uint32, error, *windows.Overlapped) {
}
func diagnosticPort(string, windows.Handle, string) {}
