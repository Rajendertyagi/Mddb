//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

// diskUsage returns used and total bytes for the filesystem that
// contains path via GetDiskFreeSpaceEx (the Windows equivalent of statfs).
func diskUsage(path string) (used, total uint64, ok bool) {
	var freeBytesAvailable, totalBytes, totalFree uint64
	err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(path), &freeBytesAvailable, &totalBytes, &totalFree)
	if err != nil || totalBytes == 0 {
		return 0, 0, false
	}
	return totalBytes - totalFree, totalBytes, true
}
