//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

// processCPUTimes returns the current process's user and system CPU time in
// nanoseconds via GetProcessTimes (the Windows equivalent of getrusage).
func processCPUTimes() (userNs, systemNs int64, ok bool) {
	var creationTime, exitTime, kernelTime, userTime windows.Filetime
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return 0, 0, false
	}
	err = windows.GetProcessTimes(process, &creationTime, &exitTime, &kernelTime, &userTime)
	if err != nil {
		return 0, 0, false
	}
	// FILETIME counts 100ns intervals since 1601; only deltas are used by
	// the caller, so the epoch offset is irrelevant.
	return userTime.Nanoseconds(), kernelTime.Nanoseconds(), true
}
