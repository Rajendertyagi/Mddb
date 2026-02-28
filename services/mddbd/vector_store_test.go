package main

import (
	"math"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestContentHash(t *testing.T) {
	h1 := ContentHash("hello world")
	h2 := ContentHash("hello world")
	h3 := ContentHash("hello world!")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if len(h1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("expected 16 char hash, got %d: %s", len(h1), h1)
	}
}

func TestMarshalUnmarshalEmbeddingRecord(t *testing.T) {
	rec := &EmbeddingRecord{
		DocID:       "test|doc|en_us",
		Vector:      []float32{0.1, 0.2, 0.3, -0.5, 1.0},
		Model:       "text-embedding-3-small",
		Dimensions:  5,
		CreatedAt:   1700000000,
		ContentHash: "abc123def456",
	}

	data := marshalEmbeddingRecord(rec)
	got, err := unmarshalEmbeddingRecord(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.DocID != rec.DocID {
		t.Errorf("DocID: got %q, want %q", got.DocID, rec.DocID)
	}
	if got.Model != rec.Model {
		t.Errorf("Model: got %q, want %q", got.Model, rec.Model)
	}
	if got.Dimensions != rec.Dimensions {
		t.Errorf("Dimensions: got %d, want %d", got.Dimensions, rec.Dimensions)
	}
	if got.CreatedAt != rec.CreatedAt {
		t.Errorf("CreatedAt: got %d, want %d", got.CreatedAt, rec.CreatedAt)
	}
	if got.ContentHash != rec.ContentHash {
		t.Errorf("ContentHash: got %q, want %q", got.ContentHash, rec.ContentHash)
	}
	if len(got.Vector) != len(rec.Vector) {
		t.Fatalf("Vector length: got %d, want %d", len(got.Vector), len(rec.Vector))
	}
	for i := range rec.Vector {
		if math.Abs(float64(got.Vector[i]-rec.Vector[i])) > 0.0001 {
			t.Errorf("Vector[%d]: got %f, want %f", i, got.Vector[i], rec.Vector[i])
		}
	}
}

func TestMarshalUnmarshalLargeVector(t *testing.T) {
	vec := make([]float32, 1536)
	for i := range vec {
		vec[i] = float32(i) / 1536.0
	}

	rec := &EmbeddingRecord{
		DocID:       "docs|article-123|en_us",
		Vector:      vec,
		Model:       "text-embedding-3-small",
		Dimensions:  1536,
		CreatedAt:   1700000000,
		ContentHash: "deadbeef12345678",
	}

	data := marshalEmbeddingRecord(rec)
	got, err := unmarshalEmbeddingRecord(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Dimensions != 1536 {
		t.Errorf("Dimensions: got %d, want 1536", got.Dimensions)
	}
	if len(got.Vector) != 1536 {
		t.Errorf("Vector length: got %d, want 1536", len(got.Vector))
	}

	expectedMin := 1536 * 4
	if len(data) < expectedMin {
		t.Errorf("data too small: %d bytes, expected at least %d", len(data), expectedMin)
	}
}

func TestSplitKey(t *testing.T) {
	tests := []struct {
		key      string
		expected []string
	}{
		{"vec|docs|doc1", []string{"vec", "docs", "doc1"}},
		{"doc|blog|post-1|en_us", []string{"doc", "blog", "post-1", "en_us"}},
		{"simple", []string{"simple"}},
	}

	for _, tt := range tests {
		got := splitKey([]byte(tt.key))
		if len(got) != len(tt.expected) {
			t.Errorf("splitKey(%q): got %d parts, want %d", tt.key, len(got), len(tt.expected))
			continue
		}
		for i := range tt.expected {
			if got[i] != tt.expected[i] {
				t.Errorf("splitKey(%q)[%d]: got %q, want %q", tt.key, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestBuildVecKey(t *testing.T) {
	key := buildVecKey("docs", "doc1")
	if string(key) != "vec|docs|doc1" {
		t.Errorf("buildVecKey: got %q, want %q", string(key), "vec|docs|doc1")
	}
}

func TestVectorStorePutGetDelete(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "mddb-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	db, err := bolt.Open(tmpPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	store := NewVectorStore(db)
	if err := store.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	// Put
	vec := []float32{0.1, 0.2, 0.3}
	err = store.Put("docs", "doc1", vec, "test-model", "abc123")
	if err != nil {
		t.Fatal(err)
	}

	// Get
	rec, err := store.Get("docs", "doc1")
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("expected record, got nil")
	}
	if rec.Model != "test-model" {
		t.Errorf("Model: got %q, want %q", rec.Model, "test-model")
	}
	if rec.ContentHash != "abc123" {
		t.Errorf("ContentHash: got %q, want %q", rec.ContentHash, "abc123")
	}
	if len(rec.Vector) != 3 {
		t.Errorf("Vector length: got %d, want 3", len(rec.Vector))
	}

	// Get non-existent
	rec, err = store.Get("docs", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if rec != nil {
		t.Error("expected nil for non-existent doc")
	}

	// Count
	counts, err := store.CountByCollection()
	if err != nil {
		t.Fatal(err)
	}
	if counts["docs"] != 1 {
		t.Errorf("count: got %d, want 1", counts["docs"])
	}

	// Delete
	err = store.Delete("docs", "doc1")
	if err != nil {
		t.Fatal(err)
	}

	rec, err = store.Get("docs", "doc1")
	if err != nil {
		t.Fatal(err)
	}
	if rec != nil {
		t.Error("expected nil after delete")
	}
}
