package binlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestBinlog(t *testing.T) (*Binlog, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "binlog_test_*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "test.db")

	bl, err := NewBinlog(dbPath, BinlogConfig{
		MaxSize: 10 * 1024 * 1024, // 10MB for tests
		MaxAge:  1 * time.Hour,
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatal(err)
	}
	cleanup := func() {
		_ = bl.Close()
		_ = os.RemoveAll(dir)
	}
	return bl, cleanup
}

func TestBinlogAppendAndRead(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	// Append entries
	for i := 0; i < 10; i++ {
		entry := &BinlogEntry{
			Type:       BinlogPut,
			BucketName: "docs",
			Key:        []byte("doc|blog|post" + string(rune('0'+i))),
			Value:      []byte(`{"id":"post` + string(rune('0'+i)) + `"}`),
		}
		if err := bl.Append(entry); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Verify LSN
	if bl.CurrentLSN() != 10 {
		t.Errorf("expected LSN 10, got %d", bl.CurrentLSN())
	}

	// ReadFrom is exclusive: ReadFrom(N) returns entries with LSN > N
	// Read all entries (fromLSN=0 means everything)
	entries, err := bl.ReadFrom(0)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected 10 entries, got %d", len(entries))
	}

	// Verify first entry
	if entries[0].LSN != 1 {
		t.Errorf("expected LSN 1, got %d", entries[0].LSN)
	}
	if entries[0].BucketName != "docs" {
		t.Errorf("expected bucket 'docs', got %q", entries[0].BucketName)
	}

	// Read from middle: ReadFrom(4) returns LSN 5-10
	entries, err = bl.ReadFrom(4)
	if err != nil {
		t.Fatalf("ReadFrom(4) failed: %v", err)
	}
	if len(entries) != 6 {
		t.Errorf("expected 6 entries (5-10), got %d", len(entries))
	}
	if entries[0].LSN != 5 {
		t.Errorf("expected first entry LSN=5, got %d", entries[0].LSN)
	}
}

func TestBinlogEntrySerialize(t *testing.T) {
	entry := &BinlogEntry{
		LSN:        42,
		Type:       BinlogPut,
		Timestamp:  time.Now().UnixNano(),
		BucketName: "docs",
		Key:        []byte("doc|blog|hello"),
		Value:      []byte(`{"id":"hello","key":"hello"}`),
	}

	data := MarshalBinlogEntry(entry)

	decoded, _, err := UnmarshalBinlogEntry(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.LSN != entry.LSN {
		t.Errorf("LSN mismatch: %d != %d", decoded.LSN, entry.LSN)
	}
	if decoded.Type != entry.Type {
		t.Errorf("Type mismatch: %d != %d", decoded.Type, entry.Type)
	}
	if decoded.BucketName != entry.BucketName {
		t.Errorf("BucketName mismatch: %q != %q", decoded.BucketName, entry.BucketName)
	}
	if string(decoded.Key) != string(entry.Key) {
		t.Errorf("Key mismatch: %q != %q", decoded.Key, entry.Key)
	}
	if string(decoded.Value) != string(entry.Value) {
		t.Errorf("Value mismatch: %q != %q", decoded.Value, entry.Value)
	}
}

func TestBinlogDeleteEntry(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	entry := &BinlogEntry{
		Type:       BinlogDelete,
		BucketName: "docs",
		Key:        []byte("doc|blog|removed"),
	}
	if err := bl.Append(entry); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	entries, err := bl.ReadFrom(0)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != BinlogDelete {
		t.Errorf("expected BinlogDelete, got %d", entries[0].Type)
	}
	if len(entries[0].Value) != 0 {
		t.Errorf("expected empty value for delete, got %d bytes", len(entries[0].Value))
	}
}

func TestBinlogSubscribe(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	ch := bl.Subscribe("test-follower")
	defer bl.Unsubscribe("test-follower")

	// Append after subscribing
	entry := &BinlogEntry{
		Type:       BinlogPut,
		BucketName: "docs",
		Key:        []byte("doc|blog|live"),
		Value:      []byte(`{"id":"live"}`),
	}
	if err := bl.Append(entry); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Receive from channel
	select {
	case received := <-ch:
		if received.LSN != 1 {
			t.Errorf("expected LSN 1, got %d", received.LSN)
		}
		if string(received.Key) != string(entry.Key) {
			t.Errorf("key mismatch: %q != %q", received.Key, entry.Key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscriber entry")
	}
}

func TestBinlogBatchAppend(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	entries := make([]*BinlogEntry, 5)
	for i := range entries {
		entries[i] = &BinlogEntry{
			Type:       BinlogPut,
			BucketName: "docs",
			Key:        []byte("doc|batch|" + string(rune('a'+i))),
			Value:      []byte(`{}`),
		}
	}

	if err := bl.AppendBatch(entries); err != nil {
		t.Fatalf("AppendBatch failed: %v", err)
	}

	if bl.CurrentLSN() != 5 {
		t.Errorf("expected LSN 5, got %d", bl.CurrentLSN())
	}

	read, err := bl.ReadFrom(0)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if len(read) != 5 {
		t.Errorf("expected 5 entries, got %d", len(read))
	}
}

func TestBinlogStats(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		_ = bl.Append(&BinlogEntry{
			Type:       BinlogPut,
			BucketName: "docs",
			Key:        []byte("k"),
			Value:      []byte("v"),
		})
	}

	stats := bl.Stats()
	if stats.CurrentLSN != 3 {
		t.Errorf("expected CurrentLSN 3, got %d", stats.CurrentLSN)
	}
	if stats.FileSize <= 0 {
		t.Errorf("expected FileSize > 0, got %d", stats.FileSize)
	}
}

func TestBinlogOpsCollector(t *testing.T) {
	bl, cleanup := newTestBinlog(t)
	defer cleanup()

	var bo BinlogOps
	bo.Put("docs", []byte("key1"), []byte("val1"))
	bo.Put("docs", []byte("key2"), []byte("val2"))
	bo.Delete("docs", []byte("key3"))
	bo.FlushTo(bl)

	if bl.CurrentLSN() != 3 {
		t.Errorf("expected LSN 3, got %d", bl.CurrentLSN())
	}

	entries, _ := bl.ReadFrom(0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Type != BinlogPut {
		t.Errorf("expected Put, got %d", entries[0].Type)
	}
	if entries[2].Type != BinlogDelete {
		t.Errorf("expected Delete, got %d", entries[2].Type)
	}
}

func TestBinlogOpsFlushToNil(t *testing.T) {
	// FlushTo with nil binlog should not panic
	var bo BinlogOps
	bo.Put("docs", []byte("key"), []byte("val"))
	bo.FlushTo(nil) // should be a no-op
}

func TestBinlogRecoverLSN(t *testing.T) {
	dir, err := os.MkdirTemp("", "binlog_recover_*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	dbPath := filepath.Join(dir, "test.db")

	// Create and write entries
	bl, err := NewBinlog(dbPath, BinlogConfig{MaxSize: 10 * 1024 * 1024, MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_ = bl.Append(&BinlogEntry{
			Type:       BinlogPut,
			BucketName: "docs",
			Key:        []byte("k"),
			Value:      []byte("v"),
		})
	}
	_ = bl.Close()

	// Reopen and verify LSN recovery
	bl2, err := NewBinlog(dbPath, BinlogConfig{MaxSize: 10 * 1024 * 1024, MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bl2.Close() }()

	if bl2.CurrentLSN() != 5 {
		t.Errorf("expected recovered LSN 5, got %d", bl2.CurrentLSN())
	}

	// Append more
	_ = bl2.Append(&BinlogEntry{
		Type:       BinlogPut,
		BucketName: "docs",
		Key:        []byte("k"),
		Value:      []byte("v"),
	})
	if bl2.CurrentLSN() != 6 {
		t.Errorf("expected LSN 6 after append, got %d", bl2.CurrentLSN())
	}
}
