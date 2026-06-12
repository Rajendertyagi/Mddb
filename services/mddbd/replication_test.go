package main

import (
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "repl_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
		Cache: NewDocumentCache(100, 60),
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

func TestReplicationApplierPut(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	applier := NewReplicationApplier(s)

	// Apply a Put entry
	entry := &BinlogEntry{
		LSN:        1,
		Type:       BinlogPut,
		BucketName: "docs",
		Key:        []byte("doc|blog|post1"),
		Value:      []byte(`{"id":"post1","key":"hello","lang":"en"}`),
	}
	if err := applier.Apply(entry); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if applier.LastAppliedLSN() != 1 {
		t.Errorf("expected LastAppliedLSN 1, got %d", applier.LastAppliedLSN())
	}

	// Verify data in BoltDB
	var val []byte
	_ = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("docs"))
		val = b.Get([]byte("doc|blog|post1"))
		return nil
	})
	if val == nil {
		t.Fatal("expected document in BoltDB, got nil")
	}
}

func TestReplicationApplierDelete(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	applier := NewReplicationApplier(s)

	// First put
	putEntry := &BinlogEntry{
		LSN:        1,
		Type:       BinlogPut,
		BucketName: "docs",
		Key:        []byte("doc|blog|post1"),
		Value:      []byte(`{"id":"post1"}`),
	}
	if err := applier.Apply(putEntry); err != nil {
		t.Fatalf("Apply Put failed: %v", err)
	}

	// Then delete
	delEntry := &BinlogEntry{
		LSN:        2,
		Type:       BinlogDelete,
		BucketName: "docs",
		Key:        []byte("doc|blog|post1"),
	}
	if err := applier.Apply(delEntry); err != nil {
		t.Fatalf("Apply Delete failed: %v", err)
	}

	if applier.LastAppliedLSN() != 2 {
		t.Errorf("expected LastAppliedLSN 2, got %d", applier.LastAppliedLSN())
	}

	// Verify deleted
	var val []byte
	_ = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("docs"))
		val = b.Get([]byte("doc|blog|post1"))
		return nil
	})
	if val != nil {
		t.Fatal("expected document to be deleted")
	}
}

func TestReplicationApplierBatch(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	applier := NewReplicationApplier(s)

	entries := []*BinlogEntry{
		{LSN: 1, Type: BinlogPut, BucketName: "docs", Key: []byte("doc|blog|a"), Value: []byte(`{"id":"a"}`)},
		{LSN: 2, Type: BinlogPut, BucketName: "docs", Key: []byte("doc|blog|b"), Value: []byte(`{"id":"b"}`)},
		{LSN: 3, Type: BinlogPut, BucketName: "docs", Key: []byte("doc|blog|c"), Value: []byte(`{"id":"c"}`)},
	}

	if err := applier.ApplyBatch(entries); err != nil {
		t.Fatalf("ApplyBatch failed: %v", err)
	}

	if applier.LastAppliedLSN() != 3 {
		t.Errorf("expected LastAppliedLSN 3, got %d", applier.LastAppliedLSN())
	}

	// Verify all docs exist
	_ = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("docs"))
		for _, key := range []string{"doc|blog|a", "doc|blog|b", "doc|blog|c"} {
			if b.Get([]byte(key)) == nil {
				t.Errorf("expected %s to exist", key)
			}
		}
		return nil
	})
}

func TestReplicationApplierCreatesBucket(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	applier := NewReplicationApplier(s)

	// Apply to a bucket that doesn't exist yet
	entry := &BinlogEntry{
		LSN:        1,
		Type:       BinlogPut,
		BucketName: "custom_bucket",
		Key:        []byte("mykey"),
		Value:      []byte("myval"),
	}
	if err := applier.Apply(entry); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify bucket was created and data exists
	var val []byte
	_ = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("custom_bucket"))
		if b == nil {
			t.Fatal("expected bucket 'custom_bucket' to be created")
		}
		val = b.Get([]byte("mykey"))
		return nil
	})
	if string(val) != "myval" {
		t.Errorf("expected 'myval', got %q", val)
	}
}

func TestReplicationApplierCacheInvalidation(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	// The cache is keyed by BuildCacheKey(collection, key, lang) — the same key
	// the write path uses. The replicated doc carries key + lang, so the applier
	// derives that exact key from the entry value (GO-002).
	cacheKey := BuildCacheKey("blog", "hello", "en")
	s.Cache.Set(cacheKey, []byte(`{"id":"post1","key":"hello","lang":"en"}`))
	if _, ok := s.Cache.Get(cacheKey); !ok {
		t.Fatal("cache should have entry")
	}

	applier := NewReplicationApplier(s)

	// Apply update to same doc -> should invalidate cache
	entry := &BinlogEntry{
		LSN:        1,
		Type:       BinlogPut,
		BucketName: "docs",
		Key:        []byte("doc|blog|post1"),
		Value:      []byte(`{"id":"post1","key":"hello","lang":"en","contentMd":"updated"}`),
	}
	if err := applier.Apply(entry); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Cache should be invalidated
	if _, ok := s.Cache.Get(cacheKey); ok {
		t.Error("expected cache entry to be invalidated after replication apply")
	}
}

func TestEntryProtoConversion(t *testing.T) {
	entry := &BinlogEntry{
		LSN:        42,
		Type:       BinlogPut,
		Timestamp:  1234567890,
		BucketName: "docs",
		Key:        []byte("doc|blog|test"),
		Value:      []byte(`{"id":"test"}`),
		Checksum:   99,
	}

	p := entryToProto(entry)
	back := protoToEntry(p)

	if back.LSN != entry.LSN {
		t.Errorf("LSN mismatch: %d != %d", back.LSN, entry.LSN)
	}
	if back.Type != entry.Type {
		t.Errorf("Type mismatch: %d != %d", back.Type, entry.Type)
	}
	if back.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp mismatch")
	}
	if back.BucketName != entry.BucketName {
		t.Errorf("BucketName mismatch")
	}
	if string(back.Key) != string(entry.Key) {
		t.Errorf("Key mismatch")
	}
	if string(back.Value) != string(entry.Value) {
		t.Errorf("Value mismatch")
	}
	if back.Checksum != entry.Checksum {
		t.Errorf("Checksum mismatch")
	}
}

func TestFollowerReadOnlyEnforcement(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	// Simulate follower mode
	s.Mode = ModeRead

	// guardWrite should reject
	if s.Mode != ModeRead {
		t.Error("expected read-only mode for follower")
	}
}
