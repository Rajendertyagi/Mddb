//go:build windows

package main

import (
	"os"
	"path/filepath"
)

// replaceFile atomically replaces dst with src. Windows os.Rename fails when
// dst already exists, so remove it first (callers reload from disk after).
func replaceFile(src, dst string) error {
	if err := os.Remove(filepath.Clean(dst)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}
