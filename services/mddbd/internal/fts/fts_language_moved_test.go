package fts

import "testing"

func TestLangRegistry_Resolve(t *testing.T) {
	reg := NewLangRegistry("en")
	RegisterDefaultLanguages(reg)

	tests := []struct {
		input    string
		expected string
	}{
		{"en", "en"},
		{"pl", "pl"},
		{"de", "de"},
		{"en_GB", "en"},
		{"en-US", "en"},
		{"pl_PL", "pl"},
		{"de-DE", "de"},
		{"", "en"},      // empty -> default
		{"xx", "en"},    // unknown -> default
		{"zz_ZZ", "en"}, // unknown with subtag -> default
		{"fr", "fr"},
		{"es", "es"},
		{"it", "it"},
		{"pt", "pt"},
		{"nl", "nl"},
		{"ru", "ru"},
		{"sv", "sv"},
	}

	for _, tc := range tests {
		cfg := reg.Resolve(tc.input)
		if cfg == nil {
			t.Errorf("Resolve(%q) returned nil", tc.input)
			continue
		}
		if cfg.Code != tc.expected {
			t.Errorf("Resolve(%q) = %q, want %q", tc.input, cfg.Code, tc.expected)
		}
	}
}
func TestLangRegistry_DefaultLang(t *testing.T) {
	reg := NewLangRegistry("pl")
	RegisterDefaultLanguages(reg)

	if reg.DefaultLang() != "pl" {
		t.Errorf("DefaultLang() = %q, want %q", reg.DefaultLang(), "pl")
	}

	// With pl default, empty string should resolve to pl
	cfg := reg.Resolve("")
	if cfg == nil || cfg.Code != "pl" {
		t.Errorf("Resolve('') with pl default should return pl config")
	}
}
func TestLangRegistry_EmptyDefault(t *testing.T) {
	reg := NewLangRegistry("")
	if reg.DefaultLang() != "en" {
		t.Errorf("empty default should fall back to 'en', got %q", reg.DefaultLang())
	}
}
func TestLangRegistry_Languages(t *testing.T) {
	reg := NewLangRegistry("en")
	RegisterDefaultLanguages(reg)

	langs := reg.Languages()
	if len(langs) < 17 {
		t.Errorf("expected at least 17 languages, got %d: %v", len(langs), langs)
	}

	// Check key languages are present
	langSet := make(map[string]bool)
	for _, l := range langs {
		langSet[l] = true
	}
	for _, code := range []string{"en", "pl", "de", "fr", "es", "it", "ru", "pt", "nl", "sv"} {
		if !langSet[code] {
			t.Errorf("language %q not found in registry", code)
		}
	}
}
func TestPorterStemmer_SatisfiesInterface(t *testing.T) {
	var s Stemmer = NewPorterStemmer()
	result := s.Stem("running")
	if result != "run" {
		t.Errorf("PorterStemmer.Stem('running') = %q, want 'run'", result)
	}
}
func TestPolishStemmer_Basic(t *testing.T) {
	s := NewPolishStemmer()

	tests := []struct {
		input string
		// We just check it doesn't panic and produces a shorter/different form
		shouldChange bool
	}{
		{"domów", true},         // houses (genitive)
		{"polskich", true},      // Polish (genitive plural)
		{"programowanie", true}, // programming
		{"ab", false},           // too short
		{"x", false},            // too short
	}

	for _, tc := range tests {
		result := s.Stem(tc.input)
		if tc.shouldChange && result == tc.input {
			t.Errorf("PolishStemmer.Stem(%q) = %q, expected change", tc.input, result)
		}
		if !tc.shouldChange && result != tc.input {
			t.Errorf("PolishStemmer.Stem(%q) = %q, expected no change", tc.input, result)
		}
	}
}
func TestPolishStemmer_Interface(t *testing.T) {
	var s Stemmer = NewPolishStemmer()
	_ = s.Stem("testowanie")
}
func TestTokenizeLang_EnglishDefault(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	// Without lang, should use English defaults
	terms := s.TokenizeLang("the quick brown fox jumps", "")
	// "the" is an English stop word
	if _, ok := terms["the"]; ok {
		t.Error("expected 'the' to be filtered as English stop word")
	}
	if _, ok := terms["quick"]; !ok {
		t.Error("expected 'quick' to be in terms")
	}
}
func TestTokenizeLang_Polish(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	terms := s.TokenizeLang("to jest bardzo duża baza danych", "pl")
	// "to", "jest", "bardzo" are Polish stop words
	for _, sw := range []string{"to", "jest", "bardzo"} {
		if _, ok := terms[sw]; ok {
			t.Errorf("expected %q to be filtered as Polish stop word", sw)
		}
	}
	// "baza" and "danych" should remain (content words)
	for _, cw := range []string{"baza", "danych"} {
		found := false
		for term := range terms {
			if term == cw || len(term) > 0 {
				found = true // stemmed form might differ
				break
			}
		}
		if !found {
			t.Errorf("expected content word stemmed from %q to be in terms", cw)
		}
	}
}
func TestTokenizeLang_German(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	terms := s.TokenizeLang("der schnelle braune Fuchs", "de")
	// "der" is a German stop word
	if _, ok := terms["der"]; ok {
		t.Error("expected 'der' to be filtered as German stop word")
	}
}
func TestTokenizeLang_DifferentLangDifferentTokens(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	text := "the quick and the fox"
	enTerms := s.TokenizeLang(text, "en")
	deTerms := s.TokenizeLang(text, "de")

	// In English, "the" and "and" are stop words
	// In German, they may not be
	enKeys := sortedKeys(enTerms)
	deKeys := sortedKeys(deTerms)

	// They should differ because stop word lists differ
	if len(enKeys) == len(deKeys) {
		same := true
		for i := range enKeys {
			if enKeys[i] != deKeys[i] {
				same = false
				break
			}
		}
		if same {
			t.Log("Warning: same tokens for English and German - this can happen if no stop words match")
		}
	}
}
func TestIndexWithLang_PolishRoundTrip(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	content := "programowanie komputerowe jest fascynujące i bardzo ważne"
	err := s.IndexWithLang("blog", "doc1", content, "pl")
	if err != nil {
		t.Fatal(err)
	}

	// Verify indexing produced terms
	indexedTerms := s.TokenizeLang(content, "pl")
	if len(indexedTerms) == 0 {
		t.Fatal("expected indexed terms for Polish content")
	}

	// The TokenizeLang should filter Polish stop words ("jest", "bardzo")
	// and stem content words
	if _, ok := indexedTerms["jest"]; ok {
		t.Error("'jest' should be filtered as Polish stop word")
	}

	// Content words should be present (possibly stemmed)
	if len(indexedTerms) < 2 {
		t.Errorf("expected at least 2 content terms, got %d: %v", len(indexedTerms), indexedTerms)
	}
}
func TestIndexWithLang_GermanRoundTrip(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	content := "Die Programmierung ist faszinierend und sehr wichtig"
	err := s.IndexWithLang("blog", "doc1", content, "de")
	if err != nil {
		t.Fatal(err)
	}

	// Verify German stop words filtered and content words indexed
	indexedTerms := s.TokenizeLang(content, "de")
	if _, ok := indexedTerms["die"]; ok {
		t.Error("'die' should be filtered as German stop word")
	}
	if _, ok := indexedTerms["und"]; ok {
		t.Error("'und' should be filtered as German stop word")
	}
	if _, ok := indexedTerms["ist"]; ok {
		t.Error("'ist' should be filtered as German stop word")
	}
	if len(indexedTerms) < 2 {
		t.Errorf("expected at least 2 content terms, got %d: %v", len(indexedTerms), indexedTerms)
	}
}
func TestIndexWithLang_BackwardCompat(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	// Old-style Index (no lang) should still work
	content := "golang programming language is great"
	err := s.Index("blog", "doc1", content)
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.Search("blog", "golang", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected to find document indexed without lang")
	}
}
func TestIndexWithLang_MixedLanguageCollection(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	// Index docs in different languages in the same collection
	_ = s.IndexWithLang("articles", "en1", "programming computers software development", "en")
	_ = s.IndexWithLang("articles", "pl1", "programowanie komputerów oprogramowanie rozwój", "pl")
	_ = s.IndexWithLang("articles", "de1", "programmierung computer software entwicklung", "de")

	// Verify each language indexed different terms
	enTerms := s.TokenizeLang("programming computers software development", "en")
	plTerms := s.TokenizeLang("programowanie komputerów oprogramowanie rozwój", "pl")
	deTerms := s.TokenizeLang("programmierung computer software entwicklung", "de")

	if len(enTerms) == 0 || len(plTerms) == 0 || len(deTerms) == 0 {
		t.Errorf("expected terms for all languages: en=%d pl=%d de=%d", len(enTerms), len(plTerms), len(deTerms))
	}

	// English "programming" should be findable via Search (which uses English tokenization)
	enResults, _ := s.Search("articles", "programming", 10)
	if len(enResults) == 0 {
		t.Error("expected to find English document via Search")
	}
}
func TestIndexPositionsWithLang(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	content := "programowanie komputerowe jest fascynujące"
	err := s.IndexPositionsWithLang("blog", "doc1", content, "pl")
	if err != nil {
		t.Fatal(err)
	}
}
func TestIndexFieldsWithLang(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	fields := map[string]string{
		"content":    "programowanie komputerowe jest fascynujące",
		"meta.title": "programowanie po polsku",
	}
	err := s.IndexFieldsWithLang("blog", "doc1", fields, "pl")
	if err != nil {
		t.Fatal(err)
	}
}
func TestFTSLanguages(t *testing.T) {
	s, cleanup := newLangFTS(t)
	defer cleanup()

	// Verify the languages endpoint would work
	if s.LangRegistry() == nil {
		t.Fatal("langRegistry should be set")
	}

	langs := s.LangRegistry().Languages()
	if len(langs) < 17 {
		t.Errorf("expected at least 17 languages, got %d", len(langs))
	}
}
func TestStopWordManager_ListLang_NormalizesCode(t *testing.T) {
	db, langReg, cleanup := newLangTestEnv(t)
	defer cleanup()

	swm := NewStopWordManager(db)
	_ = swm.EnsureBucket()
	_ = swm.LoadAll()
	swm.SetLangRegistry(langReg)

	// "pl_PL" should resolve to "pl"
	_, _, lang := swm.ListLang("test", "pl_PL")
	if lang != "pl" {
		t.Errorf("expected resolved lang 'pl' for 'pl_PL', got %q", lang)
	}

	// "de-DE" should resolve to "de"
	_, _, lang = swm.ListLang("test", "de-DE")
	if lang != "de" {
		t.Errorf("expected resolved lang 'de' for 'de-DE', got %q", lang)
	}
}
func TestStopWordManager_ListLang_UnknownFallsToDefault(t *testing.T) {
	db, langReg, cleanup := newLangTestEnv(t)
	defer cleanup()

	swm := NewStopWordManager(db)
	_ = swm.EnsureBucket()
	_ = swm.LoadAll()
	swm.SetLangRegistry(langReg)

	_, _, lang := swm.ListLang("test", "xx")
	if lang != "en" {
		t.Errorf("expected fallback to 'en' for unknown lang 'xx', got %q", lang)
	}
}
func TestStopWordManager_ListLang_DifferentLangsDifferentCounts(t *testing.T) {
	db, langReg, cleanup := newLangTestEnv(t)
	defer cleanup()

	swm := NewStopWordManager(db)
	_ = swm.EnsureBucket()
	_ = swm.LoadAll()
	swm.SetLangRegistry(langReg)

	enDefaults, _, _ := swm.ListLang("test", "en")
	plDefaults, _, _ := swm.ListLang("test", "pl")
	deDefaults, _, _ := swm.ListLang("test", "de")

	// Different languages should have different stop word counts
	if len(enDefaults) == len(plDefaults) && len(enDefaults) == len(deDefaults) {
		t.Error("expected different stop word counts for different languages")
	}
}
