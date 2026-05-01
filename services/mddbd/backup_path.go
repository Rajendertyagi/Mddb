package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// backupDir returns the directory where backup files must live.
// Configurable via MDDB_BACKUP_DIR; defaults to "./backups".
func backupDir() string {
	if d := strings.TrimSpace(os.Getenv("MDDB_BACKUP_DIR")); d != "" {
		return d
	}
	return "backups"
}

// safeBackupPath validates that name resolves to a regular file inside backupDir().
// It defends against path traversal (`../`, absolute paths, symlinks escaping the
// jail) on the user-controlled `to`/`from` parameters of backup/restore endpoints.
// It returns the cleaned absolute path safe to pass to os.Open / os.Create.
//
// `requireExisting` is true for restore (the file must already be present and be
// a regular file); false for backup (the file may not yet exist, but its parent
// directory must resolve inside the jail).
func safeBackupPath(name string, requireExisting bool) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("empty backup path")
	}
	if strings.ContainsRune(name, 0) {
		return "", errors.New("invalid backup path")
	}

	root, err := filepath.Abs(backupDir())
	if err != nil {
		return "", fmt.Errorf("resolve backup dir: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve backup dir symlinks: %w", err)
	}

	// Treat `name` as relative to the backup dir. Reject anything that, after
	// joining and cleaning, escapes the jail.
	candidate := name
	if filepath.IsAbs(candidate) {
		candidate = filepath.Clean(candidate)
	} else {
		candidate = filepath.Join(rootResolved, candidate)
	}
	candidate = filepath.Clean(candidate)

	// Resolve symlinks for the existing portion of the path; for non-existent
	// targets fall back to the parent directory's resolved form.
	resolved := candidate
	if info, statErr := os.Lstat(candidate); statErr == nil {
		if requireExisting && !info.Mode().IsRegular() {
			return "", errors.New("backup path is not a regular file")
		}
		if r, rerr := filepath.EvalSymlinks(candidate); rerr == nil {
			resolved = r
		}
	} else if requireExisting {
		return "", fmt.Errorf("backup not found: %w", statErr)
	} else {
		parent := filepath.Dir(candidate)
		pr, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			return "", fmt.Errorf("resolve backup parent: %w", perr)
		}
		resolved = filepath.Join(pr, filepath.Base(candidate))
	}

	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("backup path escapes backup directory")
	}
	return resolved, nil
}
