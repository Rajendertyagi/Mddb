//go:build !windows

package main

import "os"

// replaceFile atomically replaces dst with src. On Unix, rename overwrites.
func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
