package main

import (
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "mddb-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("bolt.Open: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

func TestTemporalManager_RecordAndQuery(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	tm := NewTemporalManager(db)
	if err := tm.EnsureBuckets(); err != nil {
		t.Fatalf("EnsureBuckets: %v", err)
	}
	tm.Start()
	defer tm.Stop()

	collection := "testcol"
	docID := "doc1"
	now := time.Now().Unix()

	tm.RecordAsync(collection, docID, EventCreate, "admin")
	tm.RecordAsync(collection, docID, EventAccess, "user1")
	tm.RecordAsync(collection, docID, EventAccess, "user2")

	// Let the background goroutine flush
	time.Sleep(600 * time.Millisecond)

	events, err := tm.QueryRange(collection, docID, now-60, now+60, "", 100)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(events) < 3 {
		t.Errorf("expected at least 3 events, got %d", len(events))
	}
}

func TestTemporalManager_HotDocs(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	tm := NewTemporalManager(db)
	if err := tm.EnsureBuckets(); err != nil {
		t.Fatalf("EnsureBuckets: %v", err)
	}
	tm.Start()
	defer tm.Stop()

	collection := "hotcol"

	for i := 0; i < 5; i++ {
		tm.RecordAsync(collection, "docA", EventAccess, "u")
	}
	for i := 0; i < 2; i++ {
		tm.RecordAsync(collection, "docB", EventAccess, "u")
	}

	time.Sleep(600 * time.Millisecond)

	entries, err := tm.GetHotDocs(collection, 10, 0)
	if err != nil {
		t.Fatalf("GetHotDocs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected hot docs, got none")
	}
	if entries[0].DocID != "docA" {
		t.Errorf("expected docA at top, got %s", entries[0].DocID)
	}
	if entries[0].AccessCount < 5 {
		t.Errorf("expected accessCount >= 5, got %d", entries[0].AccessCount)
	}
}

func TestTemporalManager_Histogram(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	tm := NewTemporalManager(db)
	if err := tm.EnsureBuckets(); err != nil {
		t.Fatalf("EnsureBuckets: %v", err)
	}
	tm.Start()
	defer tm.Stop()

	collection := "histcol"
	tm.RecordAsync(collection, "d1", EventAccess, "")
	tm.RecordAsync(collection, "d2", EventAccess, "")

	time.Sleep(600 * time.Millisecond)

	now := time.Now().Unix()
	buckets, err := tm.ComputeHistogram(collection, "access", "day", now-3600, now+3600)
	if err != nil {
		t.Fatalf("ComputeHistogram: %v", err)
	}
	if len(buckets) == 0 {
		t.Error("expected at least one histogram bucket")
	}
	if buckets[0].Count < 2 {
		t.Errorf("expected count >= 2, got %d", buckets[0].Count)
	}
}

func TestIsoWeekStart(t *testing.T) {
	// 2026-W14 should start on 2026-03-30 (Monday)
	got := isoWeekStart(2026, 14)
	want := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("isoWeekStart(2026,14) = %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}
