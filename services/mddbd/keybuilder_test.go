package main

import (
	"fmt"
	"testing"
)

func TestBuildDocKey_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildDocKey("blog", "post1"))
	want := "doc|blog|post1"
	if got != want {
		t.Errorf("BuildDocKey: got %q, want %q", got, want)
	}
}

func TestBuildDocKey_EmptyCollection(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildDocKey("", "post1"))
	want := "doc||post1"
	if got != want {
		t.Errorf("BuildDocKey with empty collection: got %q, want %q", got, want)
	}
}

func TestBuildDocKey_EmptyID(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildDocKey("blog", ""))
	want := "doc|blog|"
	if got != want {
		t.Errorf("BuildDocKey with empty ID: got %q, want %q", got, want)
	}
}

func TestBuildDocKey_LongValues(t *testing.T) {
	kb := &KeyBuilder{}
	coll := "mycollection"
	id := "some-long-document-identifier-12345"
	got := string(kb.BuildDocKey(coll, id))
	want := "doc|" + coll + "|" + id
	if got != want {
		t.Errorf("BuildDocKey with long values: got %q, want %q", got, want)
	}
}

func TestBuildByKey_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildByKey("blog", "homepage", "en_GB"))
	want := "bykey|blog|homepage|en_GB"
	if got != want {
		t.Errorf("BuildByKey: got %q, want %q", got, want)
	}
}

func TestBuildByKey_EmptyLang(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildByKey("blog", "homepage", ""))
	want := "bykey|blog|homepage|"
	if got != want {
		t.Errorf("BuildByKey with empty lang: got %q, want %q", got, want)
	}
}

func TestBuildByKey_AllEmpty(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildByKey("", "", ""))
	want := "bykey|||"
	if got != want {
		t.Errorf("BuildByKey all empty: got %q, want %q", got, want)
	}
}

func TestBuildRevPrefix_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildRevPrefix("blog", "abc"))
	want := "rev|blog|abc|"
	if got != want {
		t.Errorf("BuildRevPrefix: got %q, want %q", got, want)
	}
}

func TestBuildRevPrefix_Empty(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildRevPrefix("", ""))
	want := "rev|||"
	if got != want {
		t.Errorf("BuildRevPrefix all empty: got %q, want %q", got, want)
	}
}

func TestBuildRevKey_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildRevKey("blog", "post1", 1700000000))
	want := "rev|blog|post1|00000000001700000000"
	if got != want {
		t.Errorf("BuildRevKey: got %q, want %q", got, want)
	}
}

func TestBuildRevKey_ZeroTimestamp(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildRevKey("c", "d", 0))
	want := "rev|c|d|00000000000000000000"
	if got != want {
		t.Errorf("BuildRevKey zero ts: got %q, want %q", got, want)
	}
}

func TestBuildRevKey_LargeTimestamp(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildRevKey("c", "d", 99999999999999))
	want := "rev|c|d|00000099999999999999"
	if got != want {
		t.Errorf("BuildRevKey large ts: got %q, want %q", got, want)
	}
}

func TestBuildMetaKeyPrefix_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildMetaKeyPrefix("blog", "tag", "go"))
	want := "meta|blog|tag|go|"
	if got != want {
		t.Errorf("BuildMetaKeyPrefix: got %q, want %q", got, want)
	}
}

func TestBuildMetaKeyPrefix_Empty(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildMetaKeyPrefix("", "", ""))
	want := "meta||||"
	if got != want {
		t.Errorf("BuildMetaKeyPrefix all empty: got %q, want %q", got, want)
	}
}

func TestBuildMetaKey_Basic(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildMetaKey("blog", "tag", "go", "doc1"))
	want := "meta|blog|tag|go|doc1"
	if got != want {
		t.Errorf("BuildMetaKey: got %q, want %q", got, want)
	}
}

func TestBuildMetaKey_AllEmpty(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildMetaKey("", "", "", ""))
	want := "meta||||"
	if got != want {
		t.Errorf("BuildMetaKey all empty: got %q, want %q", got, want)
	}
}

func TestBuildMetaKey_LongValues(t *testing.T) {
	kb := &KeyBuilder{}
	got := string(kb.BuildMetaKey("production", "category", "technology-articles", "document-id-xyz"))
	want := "meta|production|category|technology-articles|document-id-xyz"
	if got != want {
		t.Errorf("BuildMetaKey long: got %q, want %q", got, want)
	}
}

func TestKeyBuilder_Reuse(t *testing.T) {
	// KeyBuilder should be safe to reuse; each call overwrites the buffer
	kb := &KeyBuilder{}

	first := string(kb.BuildDocKey("a", "b"))
	if first != "doc|a|b" {
		t.Errorf("first call: got %q, want %q", first, "doc|a|b")
	}

	second := string(kb.BuildDocKey("x", "y"))
	if second != "doc|x|y" {
		t.Errorf("second call: got %q, want %q", second, "doc|x|y")
	}

	// Use a different method to ensure buffer is shared correctly
	third := string(kb.BuildByKey("c", "d", "e"))
	if third != "bykey|c|d|e" {
		t.Errorf("third call: got %q, want %q", third, "bykey|c|d|e")
	}
}

func TestKeyBuilder_Reset(t *testing.T) {
	kb := &KeyBuilder{}
	kb.BuildDocKey("test", "id")
	kb.Reset()
	// After reset, building a key should still work
	got := string(kb.BuildDocKey("new", "id"))
	if got != "doc|new|id" {
		t.Errorf("after Reset: got %q, want %q", got, "doc|new|id")
	}
}

func TestKeyBuilder_BufferDoesNotLeak(t *testing.T) {
	// Verify that returned slices point into the buffer but are correctly sized
	kb := &KeyBuilder{}

	// Build a long key
	longKey := kb.BuildDocKey("longcollection", "longdocumentidentifier")
	longLen := len(longKey)

	// Build a short key
	shortKey := kb.BuildDocKey("a", "b")
	shortLen := len(shortKey)

	if shortLen >= longLen {
		t.Errorf("short key (%d) should be shorter than long key (%d)", shortLen, longLen)
	}

	// The short key should be exactly "doc|a|b"
	if string(shortKey) != "doc|a|b" {
		t.Errorf("short key corrupted: got %q", string(shortKey))
	}
}

func TestBuildDocKey_SpecialCharacters(t *testing.T) {
	kb := &KeyBuilder{}
	// Keys might contain hyphens, underscores, dots
	got := string(kb.BuildDocKey("my-coll", "doc_id.v1"))
	want := "doc|my-coll|doc_id.v1"
	if got != want {
		t.Errorf("BuildDocKey special chars: got %q, want %q", got, want)
	}
}

func TestBuildRevKey_ConsecutiveTimestamps(t *testing.T) {
	kb := &KeyBuilder{}
	// Verify lexicographic ordering of timestamps
	k1 := string(kb.BuildRevKey("c", "d", 1))
	k2 := string(kb.BuildRevKey("c", "d", 2))
	k3 := string(kb.BuildRevKey("c", "d", 10))

	if k1 >= k2 {
		t.Errorf("timestamp ordering: %q should be < %q", k1, k2)
	}
	if k2 >= k3 {
		t.Errorf("timestamp ordering: %q should be < %q", k2, k3)
	}
}

// --- Benchmark ---

func BenchmarkBuildDocKey(b *testing.B) {
	kb := &KeyBuilder{}
	for i := 0; i < b.N; i++ {
		kb.BuildDocKey("blog", "post1")
	}
}

func BenchmarkBuildMetaKey(b *testing.B) {
	kb := &KeyBuilder{}
	for i := 0; i < b.N; i++ {
		kb.BuildMetaKey("blog", "tag", "go", "doc123")
	}
}

func BenchmarkBuildRevKey(b *testing.B) {
	kb := &KeyBuilder{}
	for i := 0; i < b.N; i++ {
		kb.BuildRevKey("blog", "post1", 1700000000)
	}
}

// --- Table-driven tests ---

func TestBuildDocKey_TableDriven(t *testing.T) {
	tests := []struct {
		coll, id string
		want     string
	}{
		{"blog", "post1", "doc|blog|post1"},
		{"docs", "readme", "doc|docs|readme"},
		{"", "", "doc||"},
		{"a", "b", "doc|a|b"},
		{"collection-name", "id_123", "doc|collection-name|id_123"},
	}

	kb := &KeyBuilder{}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.coll, tt.id), func(t *testing.T) {
			got := string(kb.BuildDocKey(tt.coll, tt.id))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildByKey_TableDriven(t *testing.T) {
	tests := []struct {
		coll, key, lang string
		want            string
	}{
		{"blog", "homepage", "en", "bykey|blog|homepage|en"},
		{"docs", "readme", "de", "bykey|docs|readme|de"},
		{"", "", "", "bykey|||"},
		{"c", "k", "l", "bykey|c|k|l"},
	}

	kb := &KeyBuilder{}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s/%s", tt.coll, tt.key, tt.lang), func(t *testing.T) {
			got := string(kb.BuildByKey(tt.coll, tt.key, tt.lang))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
