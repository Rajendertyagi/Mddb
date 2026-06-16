package main

import (
	"fmt"
	"mddb/internal/cache"
	"mddb/internal/schema"
	"mddb/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// ttlExtraTestServer creates a minimal Server with TTLManager for HTTP handler tests.
func ttlExtraTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "ttl_extra_*.db")
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
		Cache: cache.NewDocumentCache(100, 60),
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s.TTLManager = NewTTLManager(db, s)
	if err := s.TTLManager.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	// IndexQueue needed for addDocument
	s.IndexQueue = NewIndexQueue(s, 2)
	// schema.SchemaManager
	s.SchemaManager = schema.NewSchemaManager(db)
	_ = s.SchemaManager.EnsureBucket()

	// Metrics
	s.Metrics = NewMetrics(s, false)

	cleanup := func() {
		s.IndexQueue.Shutdown()
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

// ttlExtraInsertDoc inserts a doc directly into the docs bucket using JSON serialization
// (matching what handleSetTTL uses with json.Unmarshal).
func ttlExtraInsertDoc(t *testing.T, s *Server, collection, key, lang, content string) {
	t.Helper()
	docID := genID(collection, key, lang)
	doc := storage.Doc{
		ID:        docID,
		Key:       key,
		Lang:      lang,
		ContentMD: content,
		AddedAt:   time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	data, _ := marshalDoc(&doc)
	err := s.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		return bDocs.Put(storage.DocKey(collection, docID), data)
	})
	if err != nil {
		t.Fatalf("insert doc: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleSetTTL
// ---------------------------------------------------------------------------

func TestHandleSetTTL_Success(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	ttlExtraInsertDoc(t, s, "blog", "ttldoc", "en", "content")

	body := `{"collection":"blog","key":"ttldoc","lang":"en","ttl":3600}`
	req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetTTL(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response has ExpiresAt
	var result storage.Doc
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.ExpiresAt == 0 {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestHandleSetTTL_ClearTTL(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	ttlExtraInsertDoc(t, s, "blog", "ttlclear", "en", "content")

	// Set TTL first
	body := `{"collection":"blog","key":"ttlclear","lang":"en","ttl":3600}`
	req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetTTL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("set TTL: expected 200, got %d", w.Code)
	}

	// Clear TTL (ttl=0)
	body = `{"collection":"blog","key":"ttlclear","lang":"en","ttl":0}`
	req = httptest.NewRequest("POST", "/ttl/set", strings.NewReader(body))
	w = httptest.NewRecorder()
	s.handleSetTTL(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("clear TTL: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result storage.Doc
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.ExpiresAt != 0 {
		t.Errorf("expected ExpiresAt=0 after clearing TTL, got %d", result.ExpiresAt)
	}
}

func TestHandleSetTTL_MissingFields(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	tests := []struct {
		name string
		body string
	}{
		{"missing collection", `{"key":"k","lang":"en","ttl":100}`},
		{"missing key", `{"collection":"c","lang":"en","ttl":100}`},
		{"missing lang", `{"collection":"c","key":"k","ttl":100}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			s.handleSetTTL(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleSetTTL_DocumentNotFound(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"blog","key":"nonexistent","lang":"en","ttl":100}`
	req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetTTL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetTTL_InvalidBody(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleSetTTL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: TTLManager cleanup functionality
// ---------------------------------------------------------------------------

func TestTTLCleanup_ExpiredDocs(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	// Insert a doc with JSON serialization (cleanup uses json.Unmarshal)
	docID := genID("blog", "expired", "en")
	doc := storage.Doc{
		ID:        docID,
		Key:       "expired",
		Lang:      "en",
		ContentMD: "expired content",
		AddedAt:   time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	data, _ := marshalDoc(&doc)
	_ = s.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		_ = bDocs.Put(storage.DocKey("blog", docID), data)
		bByK := tx.Bucket(s.BucketNames.ByKey)
		return bByK.Put(storage.ByKeyKey("blog", "expired", "en"), []byte(docID))
	})

	// Set TTL to past time
	pastExpiry := time.Now().Unix() - 100
	_ = s.TTLManager.Set("blog", docID, pastExpiry)

	// Run cleanup
	s.TTLManager.cleanup()

	// The doc should now be cleaned up (deleted)
	var val []byte
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		val = bDocs.Get(storage.DocKey("blog", docID))
		return nil
	})
	if val != nil {
		t.Error("expected expired document to be deleted by cleanup")
	}
}

func TestTTLCleanup_NotExpired(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	// Insert a doc
	docID := genID("blog", "alive", "en")
	doc := storage.Doc{
		ID:        docID,
		Key:       "alive",
		Lang:      "en",
		ContentMD: "still alive",
		AddedAt:   time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	data, _ := marshalDoc(&doc)
	_ = s.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		_ = bDocs.Put(storage.DocKey("blog", docID), data)
		bByK := tx.Bucket(s.BucketNames.ByKey)
		return bByK.Put(storage.ByKeyKey("blog", "alive", "en"), []byte(docID))
	})

	// Set TTL to future time
	futureExpiry := time.Now().Unix() + 3600
	_ = s.TTLManager.Set("blog", docID, futureExpiry)

	// Run cleanup
	s.TTLManager.cleanup()

	// The doc should still exist
	var val []byte
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		val = bDocs.Get(storage.DocKey("blog", docID))
		return nil
	})
	if val == nil {
		t.Error("expected non-expired document to still exist after cleanup")
	}
}

func TestTTLCleanup_EmptyBucket(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	// Cleanup on empty buckets should not panic
	s.TTLManager.cleanup()
}

func TestTTLCleanup_MalformedKey(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	// Insert a malformed TTL key (only 2 parts instead of 3)
	pastExpiry := time.Now().Unix() - 100
	malformedKey := []byte(fmt.Sprintf("%020d|blogonly", pastExpiry))
	_ = s.DB.Update(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		return bTTL.Put(malformedKey, []byte{})
	})

	// Cleanup should not panic on malformed keys
	s.TTLManager.cleanup()
}

// ---------------------------------------------------------------------------
// Test: TTL with handleSetTTL when TTLManager is nil
// ---------------------------------------------------------------------------

func TestHandleSetTTL_NilTTLManager(t *testing.T) {
	s, cleanup := ttlExtraTestServer(t)
	defer cleanup()

	ttlExtraInsertDoc(t, s, "blog", "notm", "en", "content")
	s.TTLManager = nil

	body := `{"collection":"blog","key":"notm","lang":"en","ttl":100}`
	req := httptest.NewRequest("POST", "/ttl/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetTTL(w, req)

	// Should still succeed (just skip TTL bucket operations)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
