//go:build !windows

package main

import (
	"syscall"
)

// processCPUTimes returns the current process's user and system CPU time in
// nanoseconds. ok is false if the OS call fails or is unsupported.
func processCPUTimes() (userNs, systemNs int64, ok bool) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0, 0, false
	}
	return rusage.Utime.Nano(), rusage.Stime.Nano(), true
}
