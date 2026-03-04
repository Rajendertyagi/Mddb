package main

import (
	"strings"
	"testing"
)

func TestChunkText_ShortText(t *testing.T) {
	text := "This is a short text."
	chunks := ChunkText(text, 1500)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Fatalf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunkText_EmptyText(t *testing.T) {
	chunks := ChunkText("", 1500)
	if chunks != nil {
		t.Fatalf("expected nil, got %v", chunks)
	}
}

func TestChunkText_WhitespaceOnly(t *testing.T) {
	chunks := ChunkText("   \n\n   ", 1500)
	if chunks != nil {
		t.Fatalf("expected nil, got %v", chunks)
	}
}

func TestChunkText_ExactLimit(t *testing.T) {
	text := strings.Repeat("a", 1500)
	chunks := ChunkText(text, 1500)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestChunkText_MultiParagraph(t *testing.T) {
	para1 := strings.Repeat("Hello world. ", 50)  // ~650 chars
	para2 := strings.Repeat("Foo bar baz. ", 50)  // ~650 chars
	para3 := strings.Repeat("Test content. ", 50) // ~700 chars
	text := para1 + "\n\n" + para2 + "\n\n" + para3

	chunks := ChunkText(text, 1400)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// All chunks should be within limit
	for i, chunk := range chunks {
		if len(chunk) > 1400 {
			t.Errorf("chunk %d exceeds limit: %d chars", i, len(chunk))
		}
		if len(chunk) == 0 {
			t.Errorf("chunk %d is empty", i)
		}
	}

	// Reconstruct should contain all content (no data loss)
	combined := strings.Join(chunks, "\n\n")
	for _, word := range []string{"Hello", "Foo", "Test"} {
		if !strings.Contains(combined, word) {
			t.Errorf("combined chunks missing %q", word)
		}
	}
}

func TestChunkText_SingleHugeParagraph(t *testing.T) {
	// Single paragraph that's too long — should split on sentences
	text := ""
	for i := 0; i < 100; i++ {
		text += "This is sentence number. "
	}
	text = strings.TrimSpace(text) // ~2500 chars

	chunks := ChunkText(text, 500)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk) > 500 {
			t.Errorf("chunk %d exceeds limit: %d chars (max 500)", i, len(chunk))
		}
	}
}

func TestChunkText_HardSplit(t *testing.T) {
	// A single "word" that exceeds the limit — must hard split
	text := strings.Repeat("x", 3000)
	chunks := ChunkText(text, 1000)

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk) > 1000 {
			t.Errorf("chunk %d exceeds limit: %d chars", i, len(chunk))
		}
	}

	// Total length should match
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	if total != 3000 {
		t.Errorf("expected total 3000 chars, got %d", total)
	}
}

func TestChunkText_DefaultMaxChars(t *testing.T) {
	text := strings.Repeat("a", 2000)
	chunks := ChunkText(text, 0) // should default to 1500
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks with default limit, got %d", len(chunks))
	}
}

func TestChunkText_PreserveParagraphBoundaries(t *testing.T) {
	// Two paragraphs that fit together
	text := "First paragraph.\n\nSecond paragraph."
	chunks := ChunkText(text, 1500)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (both paragraphs fit), got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "First") || !strings.Contains(chunks[0], "Second") {
		t.Errorf("chunk should contain both paragraphs")
	}
}

func TestChunkText_ManySmallParagraphs(t *testing.T) {
	var paras []string
	for i := 0; i < 20; i++ {
		paras = append(paras, "Short paragraph.")
	}
	text := strings.Join(paras, "\n\n")

	chunks := ChunkText(text, 200)

	// Should merge small paragraphs into chunks
	for i, chunk := range chunks {
		if len(chunk) > 200 {
			t.Errorf("chunk %d exceeds limit: %d chars", i, len(chunk))
		}
	}

	// All paragraphs should be present
	combined := strings.Join(chunks, "\n\n")
	count := strings.Count(combined, "Short paragraph.")
	if count != 20 {
		t.Errorf("expected 20 paragraphs, found %d", count)
	}
}

func TestBaseDocID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"doc1", "doc1"},
		{"doc1#0", "doc1"},
		{"doc1#5", "doc1"},
		{"my-doc#123", "my-doc"},
		{"no-hash", "no-hash"},
	}
	for _, tt := range tests {
		got := baseDocID(tt.input)
		if got != tt.expected {
			t.Errorf("baseDocID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDeduplicateChunkResults(t *testing.T) {
	results := []VectorResult{
		{DocID: "doc1#0", Score: 0.9},
		{DocID: "doc1#1", Score: 0.7},
		{DocID: "doc2#0", Score: 0.85},
		{DocID: "doc2#1", Score: 0.95},
		{DocID: "doc3", Score: 0.6}, // legacy non-chunked
	}

	deduped := DeduplicateChunkResults(results)

	if len(deduped) != 3 {
		t.Fatalf("expected 3 results, got %d", len(deduped))
	}

	// Should be sorted by score descending
	if deduped[0].DocID != "doc2" || deduped[0].Score != 0.95 {
		t.Errorf("expected doc2 with score 0.95 first, got %+v", deduped[0])
	}
	if deduped[1].DocID != "doc1" || deduped[1].Score != 0.9 {
		t.Errorf("expected doc1 with score 0.9 second, got %+v", deduped[1])
	}
	if deduped[2].DocID != "doc3" || deduped[2].Score != 0.6 {
		t.Errorf("expected doc3 with score 0.6 third, got %+v", deduped[2])
	}
}

func TestDeduplicateChunkResults_Empty(t *testing.T) {
	deduped := DeduplicateChunkResults(nil)
	if len(deduped) != 0 {
		t.Fatalf("expected 0 results, got %d", len(deduped))
	}
}
