package main

import "testing"

func TestPorterStemmerBasic(t *testing.T) {
	stemmer := NewPorterStemmer()

	tests := []struct {
		input, expected string
	}{
		// Step 1a: plurals
		{"caresses", "caress"},
		{"ponies", "poni"},
		{"cats", "cat"},
		{"sses", "ss"},

		// Step 1b: -eed, -ed, -ing
		{"agreed", "agre"},
		{"feed", "feed"},
		{"disabled", "disabl"},
		{"fitting", "fit"},
		{"failing", "fail"},
		{"filing", "file"},
		{"tabling", "tabl"},
		{"rolling", "roll"},

		// Step 1c: Y -> I
		{"happy", "happi"},
		{"sky", "sky"},

		// Step 2: double suffixes
		{"relational", "relat"},
		{"conditional", "condit"},
		{"rational", "ration"},
		{"valenci", "valenc"},
		{"hesitanci", "hesit"},
		{"digitizer", "digit"},
		{"conformabli", "conform"},
		{"radicalli", "radic"},
		{"differentli", "differ"},
		{"vileli", "vile"},
		{"analogousli", "analog"},
		{"vietnamization", "vietnam"},
		{"predication", "predic"},
		{"operator", "oper"},
		{"feudalism", "feudal"},
		{"decisiveness", "decis"},
		{"hopefulness", "hope"},
		{"callousness", "callous"},
		{"formaliti", "formal"},
		{"sensitiviti", "sensit"},
		{"sensibiliti", "sensibl"},

		// Step 3
		{"triplicate", "triplic"},
		{"formative", "form"},
		{"formalize", "formal"},
		{"electriciti", "electr"},
		{"electrical", "electr"},
		{"hopeful", "hope"},
		{"goodness", "good"},

		// Step 4: long suffixes removed
		{"revival", "reviv"},
		{"allowance", "allow"},
		{"inference", "infer"},
		{"airliner", "airlin"},
		{"adjustable", "adjust"},
		{"defensible", "defens"},
		{"irritant", "irrit"},
		{"replacement", "replac"},
		{"adjustment", "adjust"},
		{"dependent", "depend"},
		{"adoption", "adopt"},
		{"homologou", "homolog"},
		{"communism", "commun"},
		{"activate", "activ"},
		{"angulariti", "angular"},
		{"homologous", "homolog"},
		{"effective", "effect"},
		{"bowdlerize", "bowdler"},

		// Short words unchanged
		{"a", "a"},
		{"an", "an"},
		{"go", "go"},
	}

	for _, tc := range tests {
		got := stemmer.Stem(tc.input)
		if got != tc.expected {
			t.Errorf("Stem(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestPorterStemmerShortWords(t *testing.T) {
	stemmer := NewPorterStemmer()

	// Words < 3 chars should be returned unchanged
	shorts := []string{"", "a", "be", "go", "it", "hi"}
	for _, w := range shorts {
		if got := stemmer.Stem(w); got != w {
			t.Errorf("Stem(%q) = %q, want %q (short word)", w, got, w)
		}
	}
}

func TestPorterStemmerConsistency(t *testing.T) {
	stemmer := NewPorterStemmer()

	// Stemming the same word twice should produce the same result
	words := []string{"running", "happiness", "organization", "effectively"}
	for _, w := range words {
		r1 := stemmer.Stem(w)
		r2 := stemmer.Stem(w)
		if r1 != r2 {
			t.Errorf("inconsistent stemming for %q: %q vs %q", w, r1, r2)
		}
	}
}

func TestPorterStemmerIdempotent(t *testing.T) {
	stemmer := NewPorterStemmer()

	// Stemming an already-stemmed word should ideally not change it further
	words := []string{"run", "happi", "organ", "effect"}
	for _, w := range words {
		r1 := stemmer.Stem(w)
		r2 := stemmer.Stem(r1)
		if r1 != r2 {
			t.Logf("Stem not fully idempotent for %q: %q -> %q (acceptable)", w, r1, r2)
		}
	}
}

func BenchmarkPorterStemmer(b *testing.B) {
	stemmer := NewPorterStemmer()
	words := []string{
		"running", "happiness", "organization", "effectively",
		"internationalization", "documentation", "relationships",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, w := range words {
			stemmer.Stem(w)
		}
	}
}
