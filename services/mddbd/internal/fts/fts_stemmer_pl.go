package fts

import "strings"

// PolishStemmer implements Polish word stemming.
// Based on the Snowball Polish stemming algorithm by Dmitry Shachnev.
// Reference: https://snowballstem.org/algorithms/polish/stemmer.html
type PolishStemmer struct{}

// NewPolishStemmer creates a new Polish stemmer.
func NewPolishStemmer() *PolishStemmer {
	return &PolishStemmer{}
}

// Stem returns the stemmed form of a Polish word.
func (s *PolishStemmer) Stem(word string) string {
	if len(word) < 3 {
		return word
	}

	w := []rune(strings.ToLower(word))
	r1 := findR1Polish(w)

	// Step 1: Remove noun suffixes
	w = plStepNoun(w, r1)

	// Step 2: Remove diminutive suffixes
	w = plStepDiminutive(w, r1)

	// Step 3: Remove adjective suffixes
	w = plStepAdjective(w, r1)

	// Step 4: Remove verb suffixes
	w = plStepVerb(w, r1)

	// Step 5: Remove adverb suffixes
	w = plStepAdverb(w, r1)

	// Step 6: Normalize diacritics at end
	w = plNormalize(w)

	return string(w)
}

// findR1Polish finds the region after the first non-vowel following a vowel.
func findR1Polish(w []rune) int {
	for i := 1; i < len(w); i++ {
		if isPolishVowel(w[i-1]) && !isPolishVowel(w[i]) {
			return i + 1
		}
	}
	return len(w)
}

func isPolishVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y',
		'ą', 'ę', 'ó':
		return true
	}
	return false
}

// plStepNoun removes noun suffixes.
func plStepNoun(w []rune, r1 int) []rune {
	type rule struct {
		suffix      string
		replacement string
	}

	// Ordered by suffix length (longest first)
	rules := []rule{
		// 6-char
		{"czkami", ""}, {"eczku", ""}, {"iczka", ""}, {"iczek", ""},
		{"eczka", ""}, {"eczek", ""},
		// 5-char
		{"ności", ""}, {"ości", ""}, {"kami", ""}, {"kiem", ""},
		// 4-char
		{"acja", ""}, {"acje", ""}, {"acji", ""}, {"ość", ""},
		{"ność", ""},
		// 3-char
		{"ach", ""}, {"ami", ""}, {"owi", ""}, {"owi", ""},
		{"iem", ""}, {"iem", ""}, {"ów", ""},
		{"om", ""}, {"em", ""}, {"ze", ""},
		// 2-char
		{"ów", ""}, {"om", ""}, {"ie", ""}, {"ek", ""},
		{"ce", ""}, {"ki", ""}, {"ka", ""}, {"ek", ""},
		{"ko", ""}, {"ku", ""},
	}

	for _, r := range rules {
		suffix := []rune(r.suffix)
		if len(w) >= len(suffix)+2 && len(w)-len(suffix) >= r1 {
			if hasRuneSuffix(w, suffix) {
				return append(w[:len(w)-len(suffix)], []rune(r.replacement)...)
			}
		}
	}
	return w
}

// plStepDiminutive removes diminutive suffixes.
func plStepDiminutive(w []rune, r1 int) []rune {
	suffixes := []string{
		"eczek", "iczek", "iszek", "aszek", "uszek",
		"eńka", "eńko",
		"czek", "czka",
		"szek", "szka",
	}

	for _, s := range suffixes {
		suffix := []rune(s)
		if len(w) >= len(suffix)+2 && len(w)-len(suffix) >= r1 {
			if hasRuneSuffix(w, suffix) {
				return w[:len(w)-len(suffix)]
			}
		}
	}
	return w
}

// plStepAdjective removes adjective suffixes.
func plStepAdjective(w []rune, r1 int) []rune {
	suffixes := []string{
		"owego", "owej", "owym", "owych", "owe", "owy", "owa",
		"iego", "iej", "imi", "ich",
		"nym", "nej", "nych", "na", "ne", "ny",
		"ego", "emu",
	}

	for _, s := range suffixes {
		suffix := []rune(s)
		if len(w) >= len(suffix)+2 && len(w)-len(suffix) >= r1 {
			if hasRuneSuffix(w, suffix) {
				return w[:len(w)-len(suffix)]
			}
		}
	}
	return w
}

// plStepVerb removes verb suffixes.
func plStepVerb(w []rune, r1 int) []rune {
	suffixes := []string{
		"owalibyście", "owalibyśmy",
		"ywalibyście", "ywalibyśmy",
		"owaliście", "owaliśmy",
		"ywaliście", "ywaliśmy",
		"ować", "ywać", "iwać", "awać",
		"owali", "owała", "ywali", "ywała",
		"ując", "ując",
		"ował", "ywał", "iwał", "awał",
		"ujesz", "ujemy", "ujecie",
		"ując", "uje", "ują",
		"ały", "ała", "ali", "ał",
		"cie", "asz", "amy",
		"ić", "yć", "ać", "ęć",
		"ił", "ył", "ał", "ęł",
		"aj", "uj",
	}

	for _, s := range suffixes {
		suffix := []rune(s)
		if len(w) >= len(suffix)+2 && len(w)-len(suffix) >= r1 {
			if hasRuneSuffix(w, suffix) {
				return w[:len(w)-len(suffix)]
			}
		}
	}
	return w
}

// plStepAdverb removes adverb suffixes.
func plStepAdverb(w []rune, r1 int) []rune {
	suffixes := []string{"owo", "wie", "nie", "rze"}

	for _, s := range suffixes {
		suffix := []rune(s)
		if len(w) >= len(suffix)+2 && len(w)-len(suffix) >= r1 {
			if hasRuneSuffix(w, suffix) {
				return w[:len(w)-len(suffix)]
			}
		}
	}
	return w
}

// plNormalize normalizes Polish diacritical marks at the end of stems.
func plNormalize(w []rune) []rune {
	if len(w) == 0 {
		return w
	}
	replacements := map[rune]rune{
		'ć': 'c', 'ń': 'n', 'ś': 's', 'ź': 'z', 'ż': 'z',
	}
	last := w[len(w)-1]
	if r, ok := replacements[last]; ok {
		w[len(w)-1] = r
	}
	return w
}

// hasRuneSuffix checks if w ends with the given rune suffix.
func hasRuneSuffix(w, suffix []rune) bool {
	if len(w) < len(suffix) {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		if w[len(w)-len(suffix)+i] != suffix[i] {
			return false
		}
	}
	return true
}
