package main

// PorterStemmer provides English word stemming using the Porter Stemming Algorithm.
// Reference: https://tartarus.org/martin/PorterStemmer/
type PorterStemmer struct{}

// NewPorterStemmer creates a new Porter Stemmer.
func NewPorterStemmer() *PorterStemmer {
	return &PorterStemmer{}
}

// Stem returns the stemmed form of a word.
// Words shorter than 3 characters are returned unchanged.
func (s *PorterStemmer) Stem(word string) string {
	if len(word) < 3 {
		return word
	}
	w := []byte(word)
	w = step1a(w)
	w = step1b(w)
	w = step1c(w)
	w = step2(w)
	w = step3(w)
	w = step4(w)
	w = step5a(w)
	w = step5b(w)
	return string(w)
}

// isConsonant returns true if the character at position i is a consonant.
func isConsonant(w []byte, i int) bool {
	switch w[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if i == 0 {
			return true
		}
		return !isConsonant(w, i-1)
	}
	return true
}

// measure returns the consonant sequence count (m) of a stem.
// [C](VC){m}[V] where C = consonant sequence, V = vowel sequence.
func measure(w []byte) int {
	n := len(w)
	if n == 0 {
		return 0
	}
	m := 0
	i := 0
	// Skip initial consonants
	for i < n && isConsonant(w, i) {
		i++
	}
	for i < n {
		// Skip vowels
		for i < n && !isConsonant(w, i) {
			i++
		}
		if i >= n {
			break
		}
		// Skip consonants
		for i < n && isConsonant(w, i) {
			i++
		}
		m++
	}
	return m
}

// hasVowel returns true if the stem contains a vowel.
func hasVowel(w []byte) bool {
	for i := range w {
		if !isConsonant(w, i) {
			return true
		}
	}
	return false
}

// endsWithDouble returns true if the stem ends with a double consonant.
func endsWithDouble(w []byte) bool {
	n := len(w)
	if n < 2 {
		return false
	}
	return w[n-1] == w[n-2] && isConsonant(w, n-1)
}

// endsCVC returns true if the stem ends consonant-vowel-consonant
// where the final consonant is not w, x, or y.
func endsCVC(w []byte) bool {
	n := len(w)
	if n < 3 {
		return false
	}
	if !isConsonant(w, n-1) || isConsonant(w, n-2) || !isConsonant(w, n-3) {
		return false
	}
	ch := w[n-1]
	return ch != 'w' && ch != 'x' && ch != 'y'
}

func hasSuffix(w []byte, suffix string) bool {
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

func removeSuffix(w []byte, n int) []byte {
	return w[:len(w)-n]
}



// Step 1a: plural forms
func step1a(w []byte) []byte {
	if hasSuffix(w, "sses") {
		return removeSuffix(w, 2) // SSES -> SS
	}
	if hasSuffix(w, "ies") {
		return removeSuffix(w, 2) // IES -> I
	}
	if hasSuffix(w, "ss") {
		return w // SS -> SS
	}
	if hasSuffix(w, "s") {
		return removeSuffix(w, 1) // S -> (remove)
	}
	return w
}

// Step 1b: -eed, -ed, -ing
func step1b(w []byte) []byte {
	if hasSuffix(w, "eed") {
		stem := removeSuffix(w, 3)
		if measure(stem) > 0 {
			return append(stem, "ee"...) // (m>0) EED -> EE
		}
		return w
	}

	changed := false
	if hasSuffix(w, "ed") {
		stem := removeSuffix(w, 2)
		if hasVowel(stem) {
			w = stem
			changed = true
		}
	} else if hasSuffix(w, "ing") {
		stem := removeSuffix(w, 3)
		if hasVowel(stem) {
			w = stem
			changed = true
		}
	}

	if changed {
		if hasSuffix(w, "at") || hasSuffix(w, "bl") || hasSuffix(w, "iz") {
			w = append(w, 'e')
		} else if endsWithDouble(w) {
			ch := w[len(w)-1]
			if ch != 'l' && ch != 's' && ch != 'z' {
				w = removeSuffix(w, 1)
			}
		} else if measure(w) == 1 && endsCVC(w) {
			w = append(w, 'e')
		}
	}
	return w
}

// Step 1c: (*v*) Y -> I
func step1c(w []byte) []byte {
	if hasSuffix(w, "y") {
		stem := removeSuffix(w, 1)
		if hasVowel(stem) {
			return append(stem, 'i')
		}
	}
	return w
}

// Step 2: map double suffixes to single
func step2(w []byte) []byte {
	type rule struct {
		suffix      string
		replacement string
	}
	rules := []rule{
		{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"},
		{"anci", "ance"}, {"izer", "ize"}, {"abli", "able"},
		{"alli", "al"}, {"entli", "ent"}, {"eli", "e"},
		{"ousli", "ous"}, {"ization", "ize"}, {"ation", "ate"},
		{"ator", "ate"}, {"alism", "al"}, {"iveness", "ive"},
		{"fulness", "ful"}, {"ousness", "ous"}, {"aliti", "al"},
		{"iviti", "ive"}, {"biliti", "ble"}, {"logi", "log"},
	}

	for _, r := range rules {
		if hasSuffix(w, r.suffix) {
			stem := removeSuffix(w, len(r.suffix))
			if measure(stem) > 0 {
				return append(stem, r.replacement...)
			}
			return w
		}
	}
	return w
}

// Step 3: map suffixes
func step3(w []byte) []byte {
	type rule struct {
		suffix      string
		replacement string
	}
	rules := []rule{
		{"icate", "ic"}, {"ative", ""}, {"alize", "al"},
		{"iciti", "ic"}, {"ical", "ic"}, {"ful", ""},
		{"ness", ""},
	}

	for _, r := range rules {
		if hasSuffix(w, r.suffix) {
			stem := removeSuffix(w, len(r.suffix))
			if measure(stem) > 0 {
				return append(stem, r.replacement...)
			}
			return w
		}
	}
	return w
}

// Step 4: remove suffixes (m > 1)
func step4(w []byte) []byte {
	suffixes := []string{
		"al", "ance", "ence", "er", "ic", "able", "ible", "ant",
		"ement", "ment", "ent", "ion", "ou", "ism", "ate", "iti",
		"ous", "ive", "ize",
	}

	for _, suffix := range suffixes {
		if hasSuffix(w, suffix) {
			stem := removeSuffix(w, len(suffix))
			if suffix == "ion" {
				// (m>1) and (S or T) ION
				if len(stem) > 0 && (stem[len(stem)-1] == 's' || stem[len(stem)-1] == 't') {
					if measure(stem) > 1 {
						return stem
					}
				}
			} else {
				if measure(stem) > 1 {
					return stem
				}
			}
			return w
		}
	}
	return w
}

// Step 5a: remove trailing e
func step5a(w []byte) []byte {
	if hasSuffix(w, "e") {
		stem := removeSuffix(w, 1)
		m := measure(stem)
		if m > 1 {
			return stem
		}
		if m == 1 && !endsCVC(stem) {
			return stem
		}
	}
	return w
}

// Step 5b: (m > 1 and *d and *L) -> single letter
func step5b(w []byte) []byte {
	if measure(w) > 1 && endsWithDouble(w) && w[len(w)-1] == 'l' {
		return removeSuffix(w, 1)
	}
	return w
}
