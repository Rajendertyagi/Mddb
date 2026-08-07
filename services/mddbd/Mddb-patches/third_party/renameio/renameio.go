// Package renameio provides a minimal, cross-platform compatibility shim for
// github.com/google/renameio.
//
// The real renameio package deliberately exports no functions on Windows
// ("it is not possible to reliably write files atomically on Windows"), which
// breaks compilation of anything that imports it under GOOS=windows — most
// notably github.com/coder/hnsw, a dependency of mddbd's vector index.
//
// mddbd never calls hnsw.SavedGraph.Save() (vector persistence is BoltDB
// backed); the shim only needs to satisfy hnsw's compile-time reference.
// It implements just the surface hnsw uses:
//
//	TempFile(dir, path) (*PendingFile, error)
//	(*PendingFile).Cleanup() error
//	(*PendingFile).CloseAtomicallyReplace() error
//
// This is a shim, not a full port. It uses only the Go standard library and
// favours a best-effort atomic replace: rename is atomic on POSIX, and on
// Windows the destination is removed first when os.Rename cannot overwrite it.
package renameio

import (
	"os"
	"path/filepath"
)

// PendingFile is a temporary file that will replace the destination path when
// CloseAtomicallyReplace is called.
type PendingFile struct {
	*os.File
	path     string
	replaced bool
}

// TempFile creates a temporary file in dir (or, if dir is empty, in the same
// directory as path so the final rename stays on one filesystem) that is
// intended to atomically replace path.
func TempFile(dir, path string) (*PendingFile, error) {
	if dir == "" {
		dir = filepath.Dir(path)
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return nil, err
	}
	return &PendingFile{File: f, path: path}, nil
}

// Cleanup is a no-op if CloseAtomicallyReplace succeeded; otherwise it closes
// and removes the temporary file.
func (t *PendingFile) Cleanup() error {
	if t.replaced {
		return nil
	}
	name := t.File.Name()
	if err := t.File.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

// CloseAtomicallyReplace closes the temporary file and moves it over the
// destination path. On POSIX os.Rename overwrites atomically. On Windows,
// os.Rename fails when the destination exists, so it is removed first and the
// rename retried.
func (t *PendingFile) CloseAtomicallyReplace() error {
	if err := t.File.Sync(); err != nil {
		return err
	}
	if err := t.File.Close(); err != nil {
		return err
	}
	tmp := t.File.Name()
	if err := os.Rename(tmp, t.path); err != nil {
		// Windows: destination likely already exists and blocks the rename.
		if rmErr := os.Remove(t.path); rmErr != nil && !os.IsNotExist(rmErr) {
			_ = os.Remove(tmp)
			return rmErr
		}
		if err := os.Rename(tmp, t.path); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	t.replaced = true
	return nil
}
