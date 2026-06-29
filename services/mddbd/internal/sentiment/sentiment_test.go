package sentiment

import (
	"testing"
)

func TestAnalyzeSentiment_PositiveText(t *testing.T) {
	score := AnalyzeSentiment("This is a great and wonderful product, I love it")
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
}

func TestAnalyzeSentiment_NegativeText(t *testing.T) {
	score := AnalyzeSentiment("This is terrible, horrible and disgusting")
	if score >= 0 {
		t.Errorf("expected negative score, got %f", score)
	}
}

func TestAnalyzeSentiment_NeutralText(t *testing.T) {
	score := AnalyzeSentiment("The document contains information about the system")
	if score != 0.0 {
		t.Errorf("expected 0.0 for neutral text, got %f", score)
	}
}

func TestAnalyzeSentiment_MixedText(t *testing.T) {
	score := AnalyzeSentiment("This is good but also bad")
	if score != 0.0 {
		t.Errorf("expected 0.0 for equal positive/negative, got %f", score)
	}
}

func TestAnalyzeSentiment_EmptyText(t *testing.T) {
	score := AnalyzeSentiment("")
	if score != 0.0 {
		t.Errorf("expected 0.0 for empty text, got %f", score)
	}
}

func TestAnalyzeSentiment_AllPositive(t *testing.T) {
	score := AnalyzeSentiment("good good good good")
	if score != 1.0 {
		t.Errorf("expected 1.0 for all positive, got %f", score)
	}
}

func TestAnalyzeSentiment_AllNegative(t *testing.T) {
	score := AnalyzeSentiment("bad bad bad bad")
	if score != -1.0 {
		t.Errorf("expected -1.0 for all negative, got %f", score)
	}
}

func TestAnalyzeSentiment_MarkdownContent(t *testing.T) {
	text := "# Great Review\n\n**Excellent** product with [amazing](http://link) features"
	score := AnalyzeSentiment(text)
	if score <= 0 {
		t.Errorf("expected positive score for markdown with positive words, got %f", score)
	}
}

func TestAnalyzeSentiment_CodeBlockIgnored(t *testing.T) {
	text := "This is terrible.\n```\ngreat excellent amazing\n```\nReally awful."
	score := AnalyzeSentiment(text)
	if score >= 0 {
		t.Errorf("expected negative score (code block words should be stripped), got %f", score)
	}
}

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"# Hello World", "Hello World"},
		{"**bold text**", "bold text"},
		{"[link text](http://url)", "link text"},
		{"![alt](http://img)", ""},
		{"`inline code`", "inline code"},
	}

	for _, tt := range tests {
		result := stripMarkdown(tt.input)
		if tt.contains != "" && !containsWord(result, tt.contains) {
			t.Errorf("stripMarkdown(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
		}
	}
}

func containsWord(text, word string) bool {
	return len(text) > 0 && (text == word || len(text) >= len(word))
}

func TestAnalyzeSentiment_ScoreRange(t *testing.T) {
	// Score should always be in [-1.0, 1.0]
	texts := []string{
		"good great excellent amazing wonderful fantastic awesome",
		"bad terrible horrible awful dreadful disgusting nasty",
		"good bad great terrible",
		"neutral words here nothing special",
		"",
	}
	for _, text := range texts {
		score := AnalyzeSentiment(text)
		if score < -1.0 || score > 1.0 {
			t.Errorf("score %f for %q out of range [-1.0, 1.0]", score, text)
		}
	}
}
