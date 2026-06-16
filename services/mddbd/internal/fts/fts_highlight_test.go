package fts

import (
	"strings"
	"testing"
)

func TestDeriveCloseTag(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<mark>", "</mark>"},
		{"<strong>", "</strong>"},
		{`<mark class="hit">`, "</mark>"},
		{"**", "**"},
		{"", ""},
	}
	for _, tc := range cases {
		got := deriveCloseTag(tc.in)
		if got != tc.want {
			t.Errorf("deriveCloseTag(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizedOptions_DefaultsFill(t *testing.T) {
	opts := normalizedOptions(HighlightOptions{})
	if opts.MaxFragments != defaultHighlightMaxFragments {
		t.Errorf("MaxFragments default = %d; want %d", opts.MaxFragments, defaultHighlightMaxFragments)
	}
	if opts.FragmentSize != defaultHighlightFragmentSize {
		t.Errorf("FragmentSize default = %d; want %d", opts.FragmentSize, defaultHighlightFragmentSize)
	}
	if opts.OpenTag != defaultHighlightTag || opts.CloseTag != "</mark>" {
		t.Errorf("tags wrong: open=%q close=%q", opts.OpenTag, opts.CloseTag)
	}
}

func TestIsWordBoundary(t *testing.T) {
	// "the rust language" — substring "rust" starts at offset 4, ends at 8.
	content := "the rust language"
	if !isWordBoundary(content, 4, 8) {
		t.Error("expected word boundary around 'rust'")
	}
	// "rustic" starts at 0, ends at 4 — next char is 'i' so NOT a boundary.
	content2 := "rustic crust"
	if isWordBoundary(content2, 0, 4) {
		t.Error("'rust' inside 'rustic' should not be a word boundary")
	}
}

func TestFindTermHits_CaseInsensitiveWordBoundary(t *testing.T) {
	content := "Rust is a systems language. rustic is different from rust, but RUST matches."
	hits := findTermHits(content, []string{"rust"})
	// Expected matches: "Rust" (0-4), "rust" (53-57), "RUST" (63-67). NOT "rustic".
	if len(hits) != 3 {
		t.Fatalf("expected 3 hits, got %d: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.term != "rust" {
			t.Errorf("term mismatch: %q", h.term)
		}
	}
}

func TestFindTermHits_SkipsNegatedSentinel(t *testing.T) {
	// The sentinel carries a null byte. If the extractor tries to search for
	// it, strings.Index still returns -1, but we explicitly filter it out
	// as a belt-and-braces check against future refactors.
	hits := findTermHits("plain text", []string{negatedMarker, "plain"})
	if len(hits) != 1 || hits[0].term != "plain" {
		t.Errorf("expected only 'plain' hit, got %+v", hits)
	}
}

func TestExtractHighlights_Basic(t *testing.T) {
	content := "Rust is a systems programming language designed for memory safety and concurrency."
	hl := ExtractHighlights(content, []string{"rust"}, HighlightOptions{})
	if len(hl) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(hl))
	}
	if !strings.Contains(hl[0].Fragment, "<mark>Rust</mark>") {
		t.Errorf("expected wrap around Rust, got fragment=%q", hl[0].Fragment)
	}
	// Fragment should be shorter than the full content (trimmed) only when
	// content exceeds FragmentSize. Here content ≈ 80 chars < default 150,
	// so the fragment is the whole thing without leading ellipsis.
	if strings.HasPrefix(hl[0].Fragment, "…") {
		t.Errorf("short content should not have leading ellipsis: %q", hl[0].Fragment)
	}
}

func TestExtractHighlights_LongContentTruncated(t *testing.T) {
	long := strings.Repeat("filler words ", 40) + "needle in the middle " + strings.Repeat("more filler ", 40)
	hl := ExtractHighlights(long, []string{"needle"}, HighlightOptions{FragmentSize: 60})
	if len(hl) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(hl))
	}
	frag := hl[0].Fragment
	if !strings.HasPrefix(frag, "…") || !strings.HasSuffix(frag, "…") {
		t.Errorf("expected ellipsis on both sides, got %q", frag)
	}
	if !strings.Contains(frag, "<mark>needle</mark>") {
		t.Errorf("expected needle marked, got %q", frag)
	}
}

func TestExtractHighlights_MultipleTermsInSameCluster(t *testing.T) {
	content := "The rust async runtime delivers zero-cost abstractions."
	hl := ExtractHighlights(content, []string{"rust", "async"}, HighlightOptions{})
	if len(hl) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(hl))
	}
	frag := hl[0].Fragment
	if !strings.Contains(frag, "<mark>rust</mark>") || !strings.Contains(frag, "<mark>async</mark>") {
		t.Errorf("expected both marks, got %q", frag)
	}
	if len(hl[0].MatchedTerms) != 2 {
		t.Errorf("expected 2 matched terms in cluster, got %v", hl[0].MatchedTerms)
	}
}

func TestExtractHighlights_MaxFragmentsCap(t *testing.T) {
	// Ten "needle" mentions far apart — want fragment count capped to 2.
	parts := make([]string, 10)
	for i := range parts {
		parts[i] = strings.Repeat("filler ", 50) + "needle"
	}
	content := strings.Join(parts, " ") + " " + strings.Repeat("tail ", 20)
	hl := ExtractHighlights(content, []string{"needle"}, HighlightOptions{MaxFragments: 2, FragmentSize: 40})
	if len(hl) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(hl))
	}
}

func TestExtractHighlights_CustomTag(t *testing.T) {
	hl := ExtractHighlights("find the cat here", []string{"cat"}, HighlightOptions{OpenTag: "<strong>"})
	if len(hl) != 1 {
		t.Fatal("expected 1 highlight")
	}
	if !strings.Contains(hl[0].Fragment, "<strong>cat</strong>") {
		t.Errorf("expected custom tag, got %q", hl[0].Fragment)
	}
}

func TestExtractHighlights_NoMatches(t *testing.T) {
	hl := ExtractHighlights("nothing to see here", []string{"missing"}, HighlightOptions{})
	if hl != nil {
		t.Errorf("expected nil, got %v", hl)
	}
}

func TestExtractHighlights_EmptyInputs(t *testing.T) {
	if hl := ExtractHighlights("", []string{"x"}, HighlightOptions{}); hl != nil {
		t.Errorf("empty content → nil; got %v", hl)
	}
	if hl := ExtractHighlights("some text", nil, HighlightOptions{}); hl != nil {
		t.Errorf("nil terms → nil; got %v", hl)
	}
	if hl := ExtractHighlights("some text", []string{""}, HighlightOptions{}); hl != nil {
		t.Errorf("empty term → nil; got %v", hl)
	}
}

func TestExtractHighlights_DocumentOrderOutput(t *testing.T) {
	// Two widely-separated clusters — extractor ranks them by hit count,
	// but the final output order should be by offset so UIs can display
	// fragments in reading flow.
	content := "first sentence mentions needle once." +
		strings.Repeat(" filler", 40) +
		" second sentence has needle and needle twice."
	hl := ExtractHighlights(content, []string{"needle"}, HighlightOptions{MaxFragments: 3, FragmentSize: 30})
	if len(hl) < 2 {
		t.Fatalf("expected ≥2 fragments, got %d", len(hl))
	}
	for i := 1; i < len(hl); i++ {
		if hl[i-1].StartOffset >= hl[i].StartOffset {
			t.Errorf("fragments not in document order: %d → %d", hl[i-1].StartOffset, hl[i].StartOffset)
		}
	}
}

func TestExtractHighlights_OffsetsPointIntoOriginal(t *testing.T) {
	content := "The quick brown fox jumps over the lazy dog."
	hl := ExtractHighlights(content, []string{"fox"}, HighlightOptions{})
	if len(hl) != 1 {
		t.Fatal("expected 1 highlight")
	}
	slice := content[hl[0].StartOffset:hl[0].EndOffset]
	if !strings.Contains(slice, "fox") {
		t.Errorf("offsets didn't slice over the match: got %q", slice)
	}
}
