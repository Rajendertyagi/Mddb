//go:build !windows

package main

import (
	"syscall"
)

// diskUsage returns used and total bytes for the filesystem that
// contains path. Falls back to (0, 0, false) on platforms where
// syscall.Statfs is unavailable or the call fails.
func diskUsage(path string) (used, total uint64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	// Statfs_t field widths differ across OSes (Linux Bsize int64,
	// Darwin Bsize uint32, FreeBSD Bavail int64 vs Linux uint64, ???).
	// Bsize is the only field that can plausibly be <= 0 on malformed
	// filesystems; guard it and then cast everything through uint64
	// with explicit nosec so gosec is happy on every target.
	if stat.Bsize <= 0 {
		return 0, 0, false
	}
	total = uint64(stat.Blocks) * uint64(stat.Bsize) // #nosec G115 -- Bsize validated > 0; Blocks is a filesystem size
	free := uint64(stat.Bavail) * uint64(stat.Bsize) // #nosec G115 -- Bsize validated > 0; Bavail is a filesystem size
	if free > total {
		return 0, 0, false
	}
	return total - free, total, true
}
