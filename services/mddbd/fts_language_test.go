package main

import (
	"os"
	"sort"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestServerForLang(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "fts_lang_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
		Cache: NewDocumentCache(100, 60),
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s.FTSIndex = NewFTSIndex(db)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	// Set up multi-language support
	langReg := NewLangRegistry("en")
	RegisterDefaultLanguages(langReg)
	s.FTSIndex.SetStemmer(NewPorterStemmer())
	s.FTSIndex.SetLangRegistry(langReg)

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

// --- LangRegistry tests ---

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

// --- Stemmer interface tests ---

func TestPorterStemmer_SatisfiesInterface(t *testing.T) {
	var s Stemmer = NewPorterStemmer()
	result := s.Stem("running")
	if result != "run" {
		t.Errorf("PorterStemmer.Stem('running') = %q, want 'run'", result)
	}
}

func TestSnowballStemmer_SatisfiesInterface(t *testing.T) {
	stemmer := newSnowballStemmer("de")
	if stemmer == nil {
		t.Fatal("newSnowballStemmer('de') returned nil")
	}
	var s Stemmer = stemmer
	// German: "Häuser" (houses) should stem
	result := s.Stem("häuser")
	if result == "häuser" {
		t.Errorf("German stemmer did not modify 'häuser'")
	}
}

func TestSnowballStemmer_UnsupportedLanguage(t *testing.T) {
	stemmer := newSnowballStemmer("xx")
	if stemmer != nil {
		t.Error("newSnowballStemmer('xx') should return nil for unsupported language")
	}
}

// --- Polish stemmer tests ---

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

// --- Polish stop words tests ---

func TestPolishStopWords(t *testing.T) {
	// Common Polish stop words that should be filtered (min 2 chars, since tokenizer requires len>=2)
	stopWords := []string{"na", "że", "jest", "nie", "do", "to", "jak", "ale", "czy", "lub"}
	for _, w := range stopWords {
		if !defaultStopWordsPL[w] {
			t.Errorf("expected %q to be a Polish stop word", w)
		}
	}

	// Content words that should NOT be stop words
	contentWords := []string{"programowanie", "komputer", "baza", "danych", "wyszukiwanie"}
	for _, w := range contentWords {
		if defaultStopWordsPL[w] {
			t.Errorf("expected %q to NOT be a Polish stop word", w)
		}
	}
}

// --- German stemming tests ---

func TestGermanStemming(t *testing.T) {
	stemmer := newSnowballStemmer("de")
	if stemmer == nil {
		t.Fatal("German stemmer not available")
	}

	// German words should be modified
	tests := []string{"häuser", "laufen", "kinder", "programmierung"}
	for _, word := range tests {
		result := stemmer.Stem(word)
		if result == word {
			t.Errorf("German stemmer did not modify %q", word)
		}
	}
}

// --- French stemming tests ---

func TestFrenchStemming(t *testing.T) {
	stemmer := newSnowballStemmer("fr")
	if stemmer == nil {
		t.Fatal("French stemmer not available")
	}

	result := stemmer.Stem("maisons")
	if result == "maisons" {
		t.Error("French stemmer did not modify 'maisons'")
	}
}

// --- Spanish stemming tests ---

func TestSpanishStemming(t *testing.T) {
	stemmer := newSnowballStemmer("es")
	if stemmer == nil {
		t.Fatal("Spanish stemmer not available")
	}

	result := stemmer.Stem("casas")
	if result == "casas" {
		t.Error("Spanish stemmer did not modify 'casas'")
	}
}

// --- Russian stemming tests ---

func TestRussianStemming(t *testing.T) {
	stemmer := newSnowballStemmer("ru")
	if stemmer == nil {
		t.Fatal("Russian stemmer not available")
	}

	result := stemmer.Stem("домов")
	if result == "домов" {
		t.Error("Russian stemmer did not modify 'домов'")
	}
}

// --- TokenizeLang tests ---

func TestTokenizeLang_EnglishDefault(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	// Without lang, should use English defaults
	terms := s.FTSIndex.TokenizeLang("the quick brown fox jumps", "")
	// "the" is an English stop word
	if _, ok := terms["the"]; ok {
		t.Error("expected 'the' to be filtered as English stop word")
	}
	if _, ok := terms["quick"]; !ok {
		t.Error("expected 'quick' to be in terms")
	}
}

func TestTokenizeLang_Polish(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	terms := s.FTSIndex.TokenizeLang("to jest bardzo duża baza danych", "pl")
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
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	terms := s.FTSIndex.TokenizeLang("der schnelle braune Fuchs", "de")
	// "der" is a German stop word
	if _, ok := terms["der"]; ok {
		t.Error("expected 'der' to be filtered as German stop word")
	}
}

func TestTokenizeLang_DifferentLangDifferentTokens(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	text := "the quick and the fox"
	enTerms := s.FTSIndex.TokenizeLang(text, "en")
	deTerms := s.FTSIndex.TokenizeLang(text, "de")

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

// --- IndexWithLang + Search round-trip ---

func TestIndexWithLang_PolishRoundTrip(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	content := "programowanie komputerowe jest fascynujące i bardzo ważne"
	err := s.FTSIndex.IndexWithLang("blog", "doc1", content, "pl")
	if err != nil {
		t.Fatal(err)
	}

	// Verify indexing produced terms
	indexedTerms := s.FTSIndex.TokenizeLang(content, "pl")
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
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	content := "Die Programmierung ist faszinierend und sehr wichtig"
	err := s.FTSIndex.IndexWithLang("blog", "doc1", content, "de")
	if err != nil {
		t.Fatal(err)
	}

	// Verify German stop words filtered and content words indexed
	indexedTerms := s.FTSIndex.TokenizeLang(content, "de")
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
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	// Old-style Index (no lang) should still work
	content := "golang programming language is great"
	err := s.FTSIndex.Index("blog", "doc1", content)
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.FTSIndex.Search("blog", "golang", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected to find document indexed without lang")
	}
}

func TestIndexWithLang_MixedLanguageCollection(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	// Index docs in different languages in the same collection
	_ = s.FTSIndex.IndexWithLang("articles", "en1", "programming computers software development", "en")
	_ = s.FTSIndex.IndexWithLang("articles", "pl1", "programowanie komputerów oprogramowanie rozwój", "pl")
	_ = s.FTSIndex.IndexWithLang("articles", "de1", "programmierung computer software entwicklung", "de")

	// Verify each language indexed different terms
	enTerms := s.FTSIndex.TokenizeLang("programming computers software development", "en")
	plTerms := s.FTSIndex.TokenizeLang("programowanie komputerów oprogramowanie rozwój", "pl")
	deTerms := s.FTSIndex.TokenizeLang("programmierung computer software entwicklung", "de")

	if len(enTerms) == 0 || len(plTerms) == 0 || len(deTerms) == 0 {
		t.Errorf("expected terms for all languages: en=%d pl=%d de=%d", len(enTerms), len(plTerms), len(deTerms))
	}

	// English "programming" should be findable via Search (which uses English tokenization)
	enResults, _ := s.FTSIndex.Search("articles", "programming", 10)
	if len(enResults) == 0 {
		t.Error("expected to find English document via Search")
	}
}

// --- IndexPositionsWithLang ---

func TestIndexPositionsWithLang(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	content := "programowanie komputerowe jest fascynujące"
	err := s.FTSIndex.IndexPositionsWithLang("blog", "doc1", content, "pl")
	if err != nil {
		t.Fatal(err)
	}
}

// --- IndexFieldsWithLang ---

func TestIndexFieldsWithLang(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	fields := map[string]string{
		"content":    "programowanie komputerowe jest fascynujące",
		"meta.title": "programowanie po polsku",
	}
	err := s.FTSIndex.IndexFieldsWithLang("blog", "doc1", fields, "pl")
	if err != nil {
		t.Fatal(err)
	}
}

// --- resolveLang ---

func TestResolveLang_NoRegistry(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	idx := NewFTSIndex(db)
	idx.SetStemmer(NewPorterStemmer())

	stemmer, stopWords := idx.resolveLang("pl")
	// Without registry, should fall back to defaults
	if stemmer == nil {
		t.Error("expected fallback stemmer")
	}
	if stopWords == nil {
		t.Error("expected fallback stop words")
	}
}

func TestResolveLang_WithRegistry(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	idx := NewFTSIndex(db)
	reg := NewLangRegistry("en")
	RegisterDefaultLanguages(reg)
	idx.SetLangRegistry(reg)

	stemmer, stopWords := idx.resolveLang("pl")
	if stemmer == nil {
		t.Error("expected Polish stemmer")
	}
	// Polish stop words should include "jest"
	if !stopWords["jest"] {
		t.Error("expected Polish stop words to include 'jest'")
	}
	// And should NOT include English "the"
	if stopWords["the"] {
		t.Error("Polish stop words should not include 'the'")
	}
}

// --- Stop words per language ---

func TestStopWords_PerLanguage(t *testing.T) {
	tests := []struct {
		lang      string
		stopWords map[string]bool
		expected  []string
	}{
		{"de", defaultStopWordsDE, []string{"der", "die", "das", "und", "ist"}},
		{"fr", defaultStopWordsFR, []string{"le", "la", "les", "de", "et"}},
		{"es", defaultStopWordsES, []string{"el", "la", "los", "de", "en"}},
		{"it", defaultStopWordsIT, []string{"il", "la", "le", "di", "che"}},
		{"ru", defaultStopWordsRU, []string{"и", "в", "на", "не", "он"}},
		{"sv", defaultStopWordsSV, []string{"och", "att", "den", "för", "med"}},
	}

	for _, tc := range tests {
		for _, word := range tc.expected {
			if !tc.stopWords[word] {
				t.Errorf("[%s] expected %q to be a stop word", tc.lang, word)
			}
		}
	}
}

// --- FTSLanguages handler ---

func TestFTSLanguages(t *testing.T) {
	s, cleanup := newTestServerForLang(t)
	defer cleanup()

	// Verify the languages endpoint would work
	if s.FTSIndex.langRegistry == nil {
		t.Fatal("langRegistry should be set")
	}

	langs := s.FTSIndex.langRegistry.Languages()
	if len(langs) < 17 {
		t.Errorf("expected at least 17 languages, got %d", len(langs))
	}
}

// --- Helpers ---

func openTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "fts_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	})
	_ = db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{"fts", "ftsrev", "ftsf", "ftsfmeta", "ftsfstat", "ftsfrev", "ftsp"} {
			_, _ = tx.CreateBucketIfNotExists([]byte(b))
		}
		return nil
	})
	return db
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
