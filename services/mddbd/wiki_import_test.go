package main

import (
	"testing"
)

// --- wikiTitleToKey ---

func TestWikiTitleToKey_Basic(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"simple", "Poland", "poland"},
		{"spaces", "United States", "united-states"},
		{"parentheses", "Mercury (planet)", "mercury-planet"},
		{"slashes", "AC/DC", "ac-dc"},
		{"quotes", "Rock 'n' Roll", "rock-n-roll"},
		{"commas dots", "Dr. Smith, Jr.", "dr-smith-jr"},
		{"multiple special", "A/B (C, D)", "a-b-c-d"},
		{"trailing spaces", "  Hello  ", "hello"},
		{"double dashes collapsed", "Foo  Bar", "foo-bar"}, // double spaces → double dashes → collapsed
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wikiTitleToKey(tt.title)
			if got != tt.want {
				t.Errorf("wikiTitleToKey(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestWikiTitleToKey_Empty(t *testing.T) {
	got := wikiTitleToKey("")
	if got != "" {
		t.Errorf("empty title should return empty key, got %q", got)
	}
}

func TestWikiTitleToKey_Unicode(t *testing.T) {
	got := wikiTitleToKey("Kraków")
	if got != "kraków" {
		t.Errorf("unicode should be preserved (lowered), got %q", got)
	}
}

// --- charsetReader ---

func TestCharsetReader_UTF8(t *testing.T) {
	r, err := charsetReader("UTF-8", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// For UTF-8 with nil input, charsetReader should pass through and return nil.
	if r != nil {
		t.Errorf("expected nil reader for UTF-8 pass-through, got %T", r)
	}
}

func TestCharsetReader_Unknown(t *testing.T) {
	_, err := charsetReader("ISO-8859-1", nil)
	if err != nil {
		t.Fatalf("should not error for unknown charset (pass through), got: %v", err)
	}
}
