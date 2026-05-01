package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeBackupPath_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MDDB_BACKUP_DIR", dir)

	cases := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"/etc/passwd",
		"foo/../../etc/passwd",
		"",
		"with\x00null",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := safeBackupPath(name, false); err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}

func TestSafeBackupPath_AcceptsRelative(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MDDB_BACKUP_DIR", dir)

	got, err := safeBackupPath("snap-1.db", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	if !strings.HasPrefix(got, resolvedDir) {
		t.Fatalf("path %q not inside %q", got, resolvedDir)
	}
}

func TestSafeBackupPath_RestoreRequiresExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MDDB_BACKUP_DIR", dir)

	if _, err := safeBackupPath("missing.db", true); err == nil {
		t.Fatal("expected error when file is missing")
	}

	target := filepath.Join(dir, "snap.db")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := safeBackupPath("snap.db", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSafeBackupPath_RejectsSymlinkEscape(t *testing.T) {
	jail := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(jail, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	t.Setenv("MDDB_BACKUP_DIR", jail)

	if _, err := safeBackupPath("escape", true); err == nil {
		t.Fatal("expected error for symlink that escapes the jail")
	}
}
