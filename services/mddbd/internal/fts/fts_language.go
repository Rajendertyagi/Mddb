package fts

import (
	"strings"

	"github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/arabic"
	"github.com/blevesearch/snowballstem/danish"
	"github.com/blevesearch/snowballstem/dutch"
	"github.com/blevesearch/snowballstem/finnish"
	"github.com/blevesearch/snowballstem/french"
	"github.com/blevesearch/snowballstem/german"
	"github.com/blevesearch/snowballstem/hungarian"
	"github.com/blevesearch/snowballstem/irish"
	"github.com/blevesearch/snowballstem/italian"
	"github.com/blevesearch/snowballstem/norwegian"
	"github.com/blevesearch/snowballstem/portuguese"
	"github.com/blevesearch/snowballstem/romanian"
	"github.com/blevesearch/snowballstem/russian"
	"github.com/blevesearch/snowballstem/spanish"
	"github.com/blevesearch/snowballstem/swedish"
	"github.com/blevesearch/snowballstem/tamil"
	"github.com/blevesearch/snowballstem/turkish"
)

// Stemmer abstracts word stemming for any language.
type Stemmer interface {
	Stem(word string) string
}

// LanguageConfig holds stemmer and stop words for a single language.
type LanguageConfig struct {
	Code      string
	Name      string
	Stemmer   Stemmer
	StopWords map[string]bool
}

// LangRegistry maps language codes to their configurations.
type LangRegistry struct {
	langs       map[string]*LanguageConfig
	defaultLang string
}

// NewLangRegistry creates a new language registry with the given default language code.
func NewLangRegistry(defaultLang string) *LangRegistry {
	if defaultLang == "" {
		defaultLang = "en"
	}
	return &LangRegistry{
		langs:       make(map[string]*LanguageConfig),
		defaultLang: defaultLang,
	}
}

// Register adds a language configuration to the registry.
func (r *LangRegistry) Register(cfg *LanguageConfig) {
	r.langs[cfg.Code] = cfg
}

// Resolve returns the language config for the given language code.
// It normalizes codes like "en_GB" or "pl-PL" to their primary subtag ("en", "pl").
// Falls back to the default language if not found.
func (r *LangRegistry) Resolve(lang string) *LanguageConfig {
	if lang == "" {
		return r.langs[r.defaultLang]
	}

	// Try exact match first
	if cfg, ok := r.langs[lang]; ok {
		return cfg
	}

	// Normalize: extract primary subtag before _ or -
	primary := lang
	if idx := strings.IndexAny(lang, "_-"); idx > 0 {
		primary = lang[:idx]
	}
	primary = strings.ToLower(primary)

	if cfg, ok := r.langs[primary]; ok {
		return cfg
	}

	// Fall back to default
	return r.langs[r.defaultLang]
}

// DefaultLang returns the default language code.
func (r *LangRegistry) DefaultLang() string {
	return r.defaultLang
}

// Languages returns all registered language codes.
func (r *LangRegistry) Languages() []string {
	codes := make([]string, 0, len(r.langs))
	for code := range r.langs {
		codes = append(codes, code)
	}
	return codes
}

// SnowballStemmer wraps the blevesearch/snowballstem library for a specific language.
type SnowballStemmer struct {
	stemFunc func(env *snowballstem.Env) bool
}

// Stem returns the stemmed form of a word using the Snowball algorithm.
func (s *SnowballStemmer) Stem(word string) string {
	env := snowballstem.NewEnv(word)
	s.stemFunc(env)
	return env.Current()
}

// newSnowballStemmer creates a SnowballStemmer for the given language code.
// Returns nil if the language is not supported by the Snowball library.
func newSnowballStemmer(langCode string) *SnowballStemmer {
	stemFuncs := map[string]func(*snowballstem.Env) bool{
		"ar": arabic.Stem,
		"da": danish.Stem,
		"nl": dutch.Stem,
		"fi": finnish.Stem,
		"fr": french.Stem,
		"de": german.Stem,
		"hu": hungarian.Stem,
		"ga": irish.Stem,
		"it": italian.Stem,
		"no": norwegian.Stem,
		"pt": portuguese.Stem,
		"ro": romanian.Stem,
		"ru": russian.Stem,
		"es": spanish.Stem,
		"sv": swedish.Stem,
		"ta": tamil.Stem,
		"tr": turkish.Stem,
	}

	fn, ok := stemFuncs[langCode]
	if !ok {
		return nil
	}
	return &SnowballStemmer{stemFunc: fn}
}

// RegisterDefaultLanguages registers all supported languages with their stemmers and stop words.
func RegisterDefaultLanguages(reg *LangRegistry) {
	// English — uses PorterStemmer (already existing)
	reg.Register(&LanguageConfig{
		Code:      "en",
		Name:      "English",
		Stemmer:   NewPorterStemmer(),
		StopWords: defaultStopWords,
	})

	// Polish — uses custom Polish stemmer (no Snowball support yet)
	reg.Register(&LanguageConfig{
		Code:      "pl",
		Name:      "Polish",
		Stemmer:   NewPolishStemmer(),
		StopWords: defaultStopWordsPL,
	})

	// Snowball-supported languages
	type langDef struct {
		code      string
		name      string
		stopWords map[string]bool
	}
	snowballLangs := []langDef{
		{"de", "German", defaultStopWordsDE},
		{"fr", "French", defaultStopWordsFR},
		{"es", "Spanish", defaultStopWordsES},
		{"it", "Italian", defaultStopWordsIT},
		{"pt", "Portuguese", defaultStopWordsPT},
		{"nl", "Dutch", defaultStopWordsNL},
		{"ru", "Russian", defaultStopWordsRU},
		{"sv", "Swedish", defaultStopWordsSV},
		{"no", "Norwegian", defaultStopWordsNO},
		{"da", "Danish", defaultStopWordsDA},
		{"fi", "Finnish", defaultStopWordsFI},
		{"hu", "Hungarian", defaultStopWordsHU},
		{"ro", "Romanian", defaultStopWordsRO},
		{"tr", "Turkish", defaultStopWordsTR},
		{"ar", "Arabic", defaultStopWordsAR},
	}

	for _, l := range snowballLangs {
		stemmer := newSnowballStemmer(l.code)
		if stemmer != nil {
			reg.Register(&LanguageConfig{
				Code:      l.code,
				Name:      l.name,
				Stemmer:   stemmer,
				StopWords: l.stopWords,
			})
		}
	}
}
