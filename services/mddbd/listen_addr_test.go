package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestParseListenAddr(t *testing.T) {
	cases := []struct {
		in       string
		wantNet  string
		wantAddr string
	}{
		{":11023", "tcp", ":11023"},
		{"localhost:11023", "tcp", "localhost:11023"},
		{"0.0.0.0:11023", "tcp", "0.0.0.0:11023"},
		{"unix:/tmp/mddb.sock", "unix", "/tmp/mddb.sock"},
		{"unix:///tmp/mddb.sock", "unix", "/tmp/mddb.sock"},
		{"unix:/var/run/mddb/http.sock", "unix", "/var/run/mddb/http.sock"},
	}
	for _, c := range cases {
		gotNet, gotAddr := parseListenAddr(c.in)
		if gotNet != c.wantNet || gotAddr != c.wantAddr {
			t.Errorf("parseListenAddr(%q) = (%q, %q); want (%q, %q)",
				c.in, gotNet, gotAddr, c.wantNet, c.wantAddr)
		}
	}
}

func TestIsUnixAddr(t *testing.T) {
	if !isUnixAddr("unix:/tmp/x.sock") {
		t.Error("isUnixAddr(unix:/tmp/x.sock) should be true")
	}
	if isUnixAddr(":11023") {
		t.Error("isUnixAddr(:11023) should be false")
	}
	if isUnixAddr("localhost:11023") {
		t.Error("isUnixAddr(localhost:11023) should be false")
	}
}

func TestOpenListenerUDS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")
	lis, err := openListener("unix:" + path)
	if err != nil {
		t.Fatalf("openListener failed: %v", err)
	}
	defer func() { _ = closeListener(lis, "unix:"+path) }()

	if lis.Addr().Network() != "unix" {
		t.Errorf("Addr().Network() = %q; want unix", lis.Addr().Network())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("socket file missing: %v", err)
	}
	// Require owner-only permissions (0600). Mode bits after Type mask.
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("socket permissions = %o; want 0600", perm)
	}
}

func TestOpenListenerUDSStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.sock")
	// Create a stale file first — openListener should remove it before binding.
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}
	lis, err := openListener("unix:" + path)
	if err != nil {
		t.Fatalf("openListener should have cleaned stale socket: %v", err)
	}
	_ = closeListener(lis, "unix:"+path)
}

func TestCloseListenerRemovesSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cleanup.sock")
	lis, err := openListener("unix:" + path)
	if err != nil {
		t.Fatalf("openListener: %v", err)
	}
	if err := closeListener(lis, "unix:"+path); err != nil {
		t.Fatalf("closeListener: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("socket still exists after closeListener: err=%v", err)
	}
}

func TestOpenListenerTCP(t *testing.T) {
	// Bind to an ephemeral port to avoid collisions.
	lis, err := openListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("openListener tcp: %v", err)
	}
	defer func() { _ = lis.Close() }()
	if lis.Addr().Network() != "tcp" {
		t.Errorf("Addr().Network() = %q; want tcp", lis.Addr().Network())
	}
	// Sanity: verify we can actually dial it.
	conn, err := net.Dial("tcp", lis.Addr().String())
	if err != nil {
		t.Fatalf("dial tcp listener: %v", err)
	}
	_ = conn.Close()
}
