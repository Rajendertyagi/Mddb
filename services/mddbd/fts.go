package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketFTS      = []byte("fts")
	bucketFTSRev   = []byte("ftsrev")
	bucketFTSF     = []byte("ftsf")
	bucketFTSFMeta = []byte("ftsfmeta")
	bucketFTSFStat = []byte("ftsfstat")
	bucketFTSFRev  = []byte("ftsfrev")
)

// FTSIndex provides full-text search using an inverted index in BoltDB.
type FTSIndex struct {
	db              *bolt.DB
	stopWords       map[string]bool
	binlog          *Binlog
	stemmer         Stemmer
	langRegistry    *LangRegistry
	synonymManager  *SynonymManager
	stopWordManager *StopWordManager
	pmiData         *PMIData
}

// SetStemmer sets the stemmer for term normalization.
func (f *FTSIndex) SetStemmer(s Stemmer) { f.stemmer = s }

// SetLangRegistry sets the language registry for multi-language FTS support.
func (f *FTSIndex) SetLangRegistry(r *LangRegistry) { f.langRegistry = r }

// SetSynonymManager sets the synonym manager for query expansion.
func (f *FTSIndex) SetSynonymManager(sm *SynonymManager) { f.synonymManager = sm }

// SetStopWordManager sets the stop word manager for per-collection custom stop words.
func (f *FTSIndex) SetStopWordManager(swm *StopWordManager) { f.stopWordManager = swm }

// SetBinlog sets the binlog for replication logging.
func (f *FTSIndex) SetBinlog(bl *Binlog) {
	f.binlog = bl
}

// FTSResult represents a full-text search result.
type FTSResult struct {
	DocID        string   `json:"docId"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matchedTerms"`
}

// fieldTermEntry is used in the field-level reverse index for BM25F cleanup.
type fieldTermEntry struct {
	Field string `json:"f"`
	Term  string `json:"t"`
}

// FTSSearchRequest is the HTTP request for full-text search.
type FTSSearchRequest struct {
	Collection      string              `json:"collection"`
	Query           string              `json:"query"`
	Limit           int                 `json:"limit"`
	Algorithm       string              `json:"algorithm"`              // "tfidf", "bm25", "bm25f", "pmisparse"
	Fuzzy           int                 `json:"fuzzy"`                  // typo tolerance: 0 (off), 1 (1 edit), 2 (2 edits)
	DisableStem     bool                `json:"disableStem"`            // temporarily disable stemming for this query
	DisableSynonyms bool                `json:"disableSynonyms"`        // temporarily disable synonyms for this query
	FieldWeights    map[string]float64  `json:"fieldWeights,omitempty"` // BM25F field weights
	FilterMeta      map[string][]string `json:"filterMeta,omitempty"`   // metadata pre-filter (in-graph filtering)
	// Advanced search modes
	Mode      string             `json:"mode,omitempty"`      // "simple" (default), "boolean", "phrase", "wildcard", "proximity", "auto"
	Distance  int                `json:"distance,omitempty"`  // proximity distance (words) for mode=proximity
	RangeMeta     []RangeFilter      `json:"rangeMeta,omitempty"`     // range filters on metadata/timestamps
	Lang          string             `json:"lang,omitempty"`          // language for query tokenization (e.g. "en", "pl", "de")
	Boost         map[string]float64 `json:"boost,omitempty"`         // per-query boost: "metaKey:metaValue" → multiplier
	Highlight     bool               `json:"highlight,omitempty"`     // when true, each result carries a `highlights` array (v2.9.13+)
	HighlightTag  string             `json:"highlightTag,omitempty"`  // override wrap tag, e.g. "<strong>" (default "<mark>")
	MaxHighlights int                `json:"maxHighlights,omitempty"` // cap per-result fragments (default 3)
	FragmentSize  int                `json:"fragmentSize,omitempty"`  // approx chars per fragment (default 150)
}

// FTSSearchResponse is the HTTP response for full-text search.
type FTSSearchResponse struct {
	Results        []FTSResultWithDoc   `json:"results"`
	Total          int                  `json:"total"`
	Algorithm      string               `json:"algorithm"`
	Mode           string               `json:"mode"`
	Fuzzy          int                  `json:"fuzzy"`
	Lang           string               `json:"lang,omitempty"`
	StemmingActive bool                 `json:"stemmingActive"`
	SynonymsActive bool                 `json:"synonymsActive"`
	FieldWeights   map[string]float64   `json:"fieldWeights,omitempty"`
	Stats          *SearchStats         `json:"searchStats,omitempty"`
	SpellCorrected *SpellCorrectionInfo `json:"spellCorrected,omitempty"`
}

// FTSResultWithDoc includes the full document in the result.
type FTSResultWithDoc struct {
	Document     Doc         `json:"document"`
	Score        float64     `json:"score"`
	MatchedTerms []string    `json:"matchedTerms"`
	Highlights   []Highlight `json:"highlights,omitempty"` // populated when request.highlight=true
}

// NewFTSIndex creates a new full-text search index.
func NewFTSIndex(db *bolt.DB) *FTSIndex {
	return &FTSIndex{
		db:        db,
		stopWords: defaultStopWords,
	}
}

// EnsureBuckets creates the FTS buckets if they don't exist.
func (f *FTSIndex) EnsureBuckets() error {
	return f.db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketFTS, bucketFTSRev,
			bucketFTSF, bucketFTSFMeta, bucketFTSFStat, bucketFTSFRev,
			bucketFTSPos,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
}

// Tokenize splits text into a frequency map of normalized terms.
// If a stemmer is configured, terms are stemmed after stop word filtering.
func (f *FTSIndex) Tokenize(text string) map[string]int {
	terms := make(map[string]int)
	text = strings.ToLower(text)

	var word strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
		} else {
			if word.Len() >= 2 {
				w := word.String()
				if !f.stopWords[w] {
					if f.stemmer != nil {
						w = f.stemmer.Stem(w)
					}
					terms[w]++
				}
			}
			word.Reset()
		}
	}
	if word.Len() >= 2 {
		w := word.String()
		if !f.stopWords[w] {
			if f.stemmer != nil {
				w = f.stemmer.Stem(w)
			}
			terms[w]++
		}
	}
	return terms
}

// TokenizeQuery tokenizes query text and expands with synonyms.
func (f *FTSIndex) TokenizeQuery(collection, text string) map[string]int {
	terms := f.Tokenize(text)
	if f.synonymManager == nil {
		return terms
	}
	// Expand with synonyms
	expanded := make(map[string]int, len(terms)*2)
	for term, count := range terms {
		expanded[term] = count
		synonyms := f.synonymManager.Expand(collection, []string{term})
		for _, syn := range synonyms {
			if syn == term {
				continue
			}
			// Stem the synonym too
			stemmed := syn
			if f.stemmer != nil {
				stemmed = f.stemmer.Stem(syn)
			}
			if _, exists := expanded[stemmed]; !exists {
				expanded[stemmed] = 1
			}
		}
	}
	return expanded
}

// resolveLang returns the stemmer and stop words for the given language code.
// If no lang registry or language is configured, falls back to defaults.
func (f *FTSIndex) resolveLang(lang string) (Stemmer, map[string]bool) {
	if f.langRegistry != nil && lang != "" {
		cfg := f.langRegistry.Resolve(lang)
		if cfg != nil {
			return cfg.Stemmer, cfg.StopWords
		}
	}
	return f.stemmer, f.stopWords
}

// TokenizeLang tokenizes text using the stemmer and stop words for the given language.
func (f *FTSIndex) TokenizeLang(text, lang string) map[string]int {
	stemmer, stopWords := f.resolveLang(lang)
	terms := make(map[string]int)
	text = strings.ToLower(text)

	var word strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
		} else {
			if word.Len() >= 2 {
				w := word.String()
				if !stopWords[w] {
					if stemmer != nil {
						w = stemmer.Stem(w)
					}
					terms[w]++
				}
			}
			word.Reset()
		}
	}
	if word.Len() >= 2 {
		w := word.String()
		if !stopWords[w] {
			if stemmer != nil {
				w = stemmer.Stem(w)
			}
			terms[w]++
		}
	}
	return terms
}

// TokenizeQueryLang tokenizes query text with synonym expansion, language-aware.
func (f *FTSIndex) TokenizeQueryLang(collection, text, lang string) map[string]int {
	terms := f.TokenizeLang(text, lang)
	if f.synonymManager == nil {
		return terms
	}
	stemmer, _ := f.resolveLang(lang)
	expanded := make(map[string]int, len(terms)*2)
	for term, count := range terms {
		expanded[term] = count
		synonyms := f.synonymManager.Expand(collection, []string{term})
		for _, syn := range synonyms {
			if syn == term {
				continue
			}
			stemmed := syn
			if stemmer != nil {
				stemmed = stemmer.Stem(syn)
			}
			if _, exists := expanded[stemmed]; !exists {
				expanded[stemmed] = 1
			}
		}
	}
	return expanded
}

// IndexWithLang adds or updates the FTS index for a document using language-specific tokenization.
func (f *FTSIndex) IndexWithLang(collection, docID, content, lang string) error {
	terms := f.TokenizeLang(content, lang)
	if len(terms) == 0 {
		return nil
	}

	var bo BinlogOps
	err := f.db.Update(func(tx *bolt.Tx) error {
		bFTS := tx.Bucket(bucketFTS)
		bRev := tx.Bucket(bucketFTSRev)

		// Remove old entries via reverse index
		revKey := ftsRevKey(collection, docID)
		if old := bRev.Get(revKey); old != nil {
			oldTerms := strings.Split(string(old), ",")
			for _, term := range oldTerms {
				if term != "" {
					k := ftsKey(collection, term, docID)
					_ = bFTS.Delete(k)
					bo.Delete("fts", k)
				}
			}
		}

		// Store new entries
		termList := make([]string, 0, len(terms))
		for term, count := range terms {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], uint32(count)) // #nosec G115 -- value always positive and bounded
			k := ftsKey(collection, term, docID)
			if err := bFTS.Put(k, buf[:]); err != nil {
				return err
			}
			bo.Put("fts", k, buf[:])
			termList = append(termList, term)
		}

		// Store reverse index
		revVal := []byte(strings.Join(termList, ","))
		bo.Put("ftsrev", revKey, revVal)
		if err := bRev.Put(revKey, revVal); err != nil {
			return err
		}

		// Store BM25 metadata
		var docLength uint32
		for _, count := range terms {
			docLength += uint32(count) // #nosec G115 -- value always positive and bounded
		}
		return f.IndexBM25Meta(tx, collection, docID, docLength)
	})
	if err == nil {
		bo.FlushTo(f.binlog)
		f.InvalidatePMI(collection)
	}
	return err
}

// IndexFieldsWithLang indexes a document's fields using language-specific tokenization.
func (f *FTSIndex) IndexFieldsWithLang(collection, docID string, fields map[string]string, lang string) error {
	fieldTokens := make(map[string]map[string]int, len(fields))
	for field, text := range fields {
		tokens := f.TokenizeLang(text, lang)
		if len(tokens) > 0 {
			fieldTokens[field] = tokens
		}
	}
	if len(fieldTokens) == 0 {
		return nil
	}

	return f.db.Update(func(tx *bolt.Tx) error {
		if err := f.removeFieldData(tx, collection, docID); err != nil {
			return err
		}

		bF := tx.Bucket(bucketFTSF)
		bMeta := tx.Bucket(bucketFTSFMeta)
		bStat := tx.Bucket(bucketFTSFStat)
		bRev := tx.Bucket(bucketFTSFRev)
		if bF == nil || bMeta == nil || bStat == nil || bRev == nil {
			return nil
		}

		var allEntries []fieldTermEntry
		for field, tokens := range fieldTokens {
			var docLength uint32
			for term, count := range tokens {
				var buf [4]byte
				binary.LittleEndian.PutUint32(buf[:], uint32(count)) // #nosec G115 -- value always positive and bounded
				if err := bF.Put(ftsfKey(collection, field, term, docID), buf[:]); err != nil {
					return err
				}
				allEntries = append(allEntries, fieldTermEntry{Field: field, Term: term})
				docLength += uint32(count) // #nosec G115 -- value always positive and bounded
			}

			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], docLength)
			if err := bMeta.Put(ftsfMetaKey(collection, field, docID), buf[:]); err != nil {
				return err
			}

			sk := ftsfStatKey(collection, field)
			stats := collectionStats{}
			if sraw := bStat.Get(sk); sraw != nil {
				stats = decodeCollectionStats(sraw)
			}
			stats.TotalDocs++
			stats.TotalTerms += uint64(docLength)
			if err := bStat.Put(sk, encodeCollectionStats(stats)); err != nil {
				return err
			}
		}

		revData, err := json.Marshal(allEntries)
		if err != nil {
			return err
		}
		return bRev.Put(ftsfRevKey(collection, docID), revData)
	})
}

// Index adds or updates the FTS index for a document.
func (f *FTSIndex) Index(collection, docID, content string) error {
	terms := f.Tokenize(content)
	if len(terms) == 0 {
		return nil
	}

	var bo BinlogOps
	err := f.db.Update(func(tx *bolt.Tx) error {
		bFTS := tx.Bucket(bucketFTS)
		bRev := tx.Bucket(bucketFTSRev)

		// Remove old entries via reverse index
		revKey := ftsRevKey(collection, docID)
		if old := bRev.Get(revKey); old != nil {
			oldTerms := strings.Split(string(old), ",")
			for _, term := range oldTerms {
				if term != "" {
					k := ftsKey(collection, term, docID)
					_ = bFTS.Delete(k)
					bo.Delete("fts", k)
				}
			}
		}

		// Store new entries
		termList := make([]string, 0, len(terms))
		for term, count := range terms {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], uint32(count)) // #nosec G115 -- value always positive and bounded
			k := ftsKey(collection, term, docID)
			if err := bFTS.Put(k, buf[:]); err != nil {
				return err
			}
			bo.Put("fts", k, buf[:])
			termList = append(termList, term)
		}

		// Store reverse index
		revVal := []byte(strings.Join(termList, ","))
		bo.Put("ftsrev", revKey, revVal)
		if err := bRev.Put(revKey, revVal); err != nil {
			return err
		}

		// Store BM25 metadata (document length = sum of term frequencies)
		var docLength uint32
		for _, count := range terms {
			docLength += uint32(count) // #nosec G115 -- value always positive and bounded
		}
		return f.IndexBM25Meta(tx, collection, docID, docLength)
	})
	if err == nil {
		bo.FlushTo(f.binlog)
		f.InvalidatePMI(collection)
	}
	return err
}

// Remove deletes all FTS entries for a document.
func (f *FTSIndex) Remove(collection, docID string) error {
	var bo BinlogOps
	err := f.db.Update(func(tx *bolt.Tx) error {
		bFTS := tx.Bucket(bucketFTS)
		bRev := tx.Bucket(bucketFTSRev)

		revKey := ftsRevKey(collection, docID)
		if old := bRev.Get(revKey); old != nil {
			oldTerms := strings.Split(string(old), ",")
			for _, term := range oldTerms {
				if term != "" {
					k := ftsKey(collection, term, docID)
					_ = bFTS.Delete(k)
					bo.Delete("fts", k)
				}
			}
		}
		// Clean up BM25 metadata
		if err := f.RemoveBM25Meta(tx, collection, docID); err != nil {
			return err
		}
		// Clean up field-level FTS data (BM25F)
		if err := f.removeFieldData(tx, collection, docID); err != nil {
			return err
		}
		// Clean up positional index
		f.removePositionsInTx(tx, collection, docID)
		bo.Delete("ftsrev", revKey)
		return bRev.Delete(revKey)
	})
	if err == nil {
		bo.FlushTo(f.binlog)
		f.InvalidatePMI(collection)
	}
	return err
}

// Search performs a full-text search and returns matching document IDs with scores.
func (f *FTSIndex) Search(collection, query string, limit int) ([]FTSResult, error) {
	queryTerms := f.TokenizeQuery(collection, query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	type docScore struct {
		id           string
		totalTF      float64
		matchedTerms []string
	}

	scores := make(map[string]*docScore)

	err := f.db.View(func(tx *bolt.Tx) error {
		bFTS := tx.Bucket(bucketFTS)
		if bFTS == nil {
			return nil
		}

		for term := range queryTerms {
			prefix := ftsKey(collection, term, "")
			c := bFTS.Cursor()
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				// Extract docID from key
				docID := string(k[len(prefix):])
				if docID == "" {
					continue
				}

				tf := float64(1)
				if len(v) >= 4 {
					tf = float64(binary.LittleEndian.Uint32(v))
				}
				// Use log(1+tf) to dampen high frequency terms
				logTF := math.Log1p(tf)

				ds, ok := scores[docID]
				if !ok {
					ds = &docScore{id: docID}
					scores[docID] = ds
				}
				ds.totalTF += logTF
				ds.matchedTerms = append(ds.matchedTerms, term)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Score = (matched terms / total query terms) * average log TF
	queryTermCount := float64(len(queryTerms))
	results := make([]FTSResult, 0, len(scores))
	for _, ds := range scores {
		matchRatio := float64(len(ds.matchedTerms)) / queryTermCount
		avgTF := ds.totalTF / float64(len(ds.matchedTerms))
		score := matchRatio * (0.5 + 0.5*math.Min(avgTF/5.0, 1.0)) // normalize

		results = append(results, FTSResult{
			DocID:        ds.id,
			Score:        score,
			MatchedTerms: unique(ds.matchedTerms),
		})
	}

	// Sort by score desc
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ftsKey builds the FTS bucket key.
func ftsKey(collection, term, docID string) []byte {
	return []byte(fmt.Sprintf("fts|%s|%s|%s", collection, term, docID))
}

// ftsRevKey builds the FTS reverse lookup key.
func ftsRevKey(collection, docID string) []byte {
	return []byte(fmt.Sprintf("ftsrev|%s|%s", collection, docID))
}

// --- Field-level FTS key builders (BM25F) ---

func ftsfKey(collection, field, term, docID string) []byte {
	return []byte(fmt.Sprintf("ftsf|%s|%s|%s|%s", collection, field, term, docID))
}

func ftsfMetaKey(collection, field, docID string) []byte {
	return []byte(fmt.Sprintf("ftsfmeta|%s|%s|%s", collection, field, docID))
}

func ftsfStatKey(collection, field string) []byte {
	return []byte(fmt.Sprintf("ftsfstat|%s|%s", collection, field))
}

func ftsfRevKey(collection, docID string) []byte {
	return []byte(fmt.Sprintf("ftsfrev|%s|%s", collection, docID))
}

// removeFieldData removes all field-level FTS data for a document within a transaction.
func (f *FTSIndex) removeFieldData(tx *bolt.Tx, collection, docID string) error {
	bF := tx.Bucket(bucketFTSF)
	bRev := tx.Bucket(bucketFTSFRev)
	if bF == nil || bRev == nil {
		return nil
	}

	revKey := ftsfRevKey(collection, docID)
	old := bRev.Get(revKey)
	if old == nil {
		return nil
	}

	var entries []fieldTermEntry
	if err := json.Unmarshal(old, &entries); err != nil {
		return bRev.Delete(revKey)
	}

	// Collect unique fields and delete term entries
	fields := make(map[string]bool)
	for _, e := range entries {
		_ = bF.Delete(ftsfKey(collection, e.Field, e.Term, docID))
		fields[e.Field] = true
	}

	// Update per-field stats and delete metadata
	bMeta := tx.Bucket(bucketFTSFMeta)
	bStat := tx.Bucket(bucketFTSFStat)
	if bMeta != nil && bStat != nil {
		for field := range fields {
			mk := ftsfMetaKey(collection, field, docID)
			if raw := bMeta.Get(mk); len(raw) >= 4 {
				oldLen := binary.LittleEndian.Uint32(raw)
				sk := ftsfStatKey(collection, field)
				stats := collectionStats{}
				if sraw := bStat.Get(sk); sraw != nil {
					stats = decodeCollectionStats(sraw)
				}
				if stats.TotalDocs > 0 {
					stats.TotalDocs--
				}
				if stats.TotalTerms >= uint64(oldLen) {
					stats.TotalTerms -= uint64(oldLen)
				} else {
					stats.TotalTerms = 0
				}
				_ = bStat.Put(sk, encodeCollectionStats(stats))
			}
			_ = bMeta.Delete(mk)
		}
	}

	return bRev.Delete(revKey)
}

// IndexFields indexes a document's fields separately for BM25F scoring.
// Each field (e.g. "content", "meta.title") is tokenized and stored independently.
func (f *FTSIndex) IndexFields(collection, docID string, fields map[string]string) error {
	fieldTokens := make(map[string]map[string]int, len(fields))
	for field, text := range fields {
		tokens := f.Tokenize(text)
		if len(tokens) > 0 {
			fieldTokens[field] = tokens
		}
	}
	if len(fieldTokens) == 0 {
		return nil
	}

	return f.db.Update(func(tx *bolt.Tx) error {
		// Remove old field data first (handles re-indexing)
		if err := f.removeFieldData(tx, collection, docID); err != nil {
			return err
		}

		bF := tx.Bucket(bucketFTSF)
		bMeta := tx.Bucket(bucketFTSFMeta)
		bStat := tx.Bucket(bucketFTSFStat)
		bRev := tx.Bucket(bucketFTSFRev)
		if bF == nil || bMeta == nil || bStat == nil || bRev == nil {
			return nil
		}

		var allEntries []fieldTermEntry
		for field, tokens := range fieldTokens {
			var docLength uint32
			for term, count := range tokens {
				var buf [4]byte
				binary.LittleEndian.PutUint32(buf[:], uint32(count)) // #nosec G115 -- value always positive and bounded
				if err := bF.Put(ftsfKey(collection, field, term, docID), buf[:]); err != nil {
					return err
				}
				allEntries = append(allEntries, fieldTermEntry{Field: field, Term: term})
				docLength += uint32(count) // #nosec G115 -- value always positive and bounded
			}

			// Store per-field doc length
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], docLength)
			if err := bMeta.Put(ftsfMetaKey(collection, field, docID), buf[:]); err != nil {
				return err
			}

			// Update per-field stats
			sk := ftsfStatKey(collection, field)
			stats := collectionStats{}
			if sraw := bStat.Get(sk); sraw != nil {
				stats = decodeCollectionStats(sraw)
			}
			stats.TotalDocs++
			stats.TotalTerms += uint64(docLength)
			if err := bStat.Put(sk, encodeCollectionStats(stats)); err != nil {
				return err
			}
		}

		// Store reverse index for cleanup
		revData, err := json.Marshal(allEntries)
		if err != nil {
			return err
		}
		return bRev.Put(ftsfRevKey(collection, docID), revData)
	})
}

// --- HTTP handler ---

func (s *Server) handleFTS(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req FTSSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Query == "" {
		bad(w, fmt.Errorf("missing required fields: collection, query"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}

	if s.FTSIndex == nil {
		bad(w, fmt.Errorf("full-text search not initialized"))
		return
	}

	// Spell-correct the query when enabled for this collection
	var spellCorrected *SpellCorrectionInfo
	if s.SpellManager != nil && s.SpellManager.Ready() && s.CollectionManager != nil {
		if cfg, ok := s.CollectionManager.Get(req.Collection); ok && cfg.SpellCorrect {
			lang := req.Lang
			if cfg.SpellLang != "" {
				lang = cfg.SpellLang
			}
			corrected := s.SpellManager.Cleanup(req.Collection, lang, req.Query)
			if corrected != req.Query {
				spellCorrected = &SpellCorrectionInfo{Original: req.Query, Corrected: corrected}
				req.Query = corrected
			}
		}
	}

	// Per-query stemming/synonym control (thread-safe: no mutation of shared state)
	origStemmer := s.FTSIndex.stemmer
	origSynonyms := s.FTSIndex.synonymManager
	if req.DisableStem {
		s.FTSIndex.stemmer = nil
	}
	if req.DisableSynonyms {
		s.FTSIndex.synonymManager = nil
	}
	defer func() {
		s.FTSIndex.stemmer = origStemmer
		s.FTSIndex.synonymManager = origSynonyms
	}()

	// Resolve query language
	queryLang := req.Lang

	algo := req.Algorithm
	if algo == "" {
		algo = "tfidf"
	}
	fuzzy := req.Fuzzy
	if fuzzy < 0 {
		fuzzy = 0
	}
	if fuzzy > 2 {
		fuzzy = 2
	}

	// Determine search mode
	mode := req.Mode
	if mode == "" || mode == "auto" {
		parsed := ParseAdvancedQuery(req.Query)
		if parsed.IsAdvanced() {
			if parsed.HasPhrase && !parsed.HasBoolean && !parsed.HasWildcard && !parsed.HasProximity {
				mode = "phrase"
			} else if parsed.HasProximity && !parsed.HasBoolean && !parsed.HasWildcard {
				mode = "proximity"
			} else if parsed.HasWildcard && !parsed.HasBoolean && !parsed.HasPhrase && !parsed.HasProximity {
				mode = "wildcard"
			} else {
				mode = "boolean"
			}
		} else {
			mode = "simple"
		}
	}

	// Pre-filter by metadata (in-graph filtering)
	var allowed map[string]bool
	if len(req.FilterMeta) > 0 {
		allowed = s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowed) == 0 {
			ok(w, FTSSearchResponse{
				Results:   []FTSResultWithDoc{},
				Algorithm: algo,
				Mode:      mode,
				Fuzzy:     fuzzy,
			})
			return
		}
	}

	// Tokenize query (needed for bm25f, reused below) — language-aware
	tokens := s.FTSIndex.TokenizeQueryLang(req.Collection, req.Query, queryLang)

	var results []FTSResult
	var err error

	switch mode {
	case "boolean":
		parsed := ParseAdvancedQuery(req.Query)
		results, err = s.FTSIndex.SearchBoolean(req.Collection, parsed, req.Limit)

	case "phrase":
		// Extract phrase text (strip quotes if present)
		phraseText := strings.Trim(req.Query, "\"")
		results, err = s.FTSIndex.SearchPhrase(req.Collection, phraseText, req.Limit)

	case "proximity":
		parsed := ParseAdvancedQuery(req.Query)
		distance := req.Distance
		if distance <= 0 {
			distance = 5 // default proximity distance
		}
		// If parsed has proximity clause, use its distance
		for _, c := range parsed.Clauses {
			if c.Type == "proximity" {
				distance = c.Distance
				results, err = s.FTSIndex.SearchProximity(req.Collection, c.Value, distance, req.Limit)
				break
			}
		}
		if results == nil && err == nil {
			phraseText := strings.Trim(req.Query, "\"")
			results, err = s.FTSIndex.SearchProximity(req.Collection, phraseText, distance, req.Limit)
		}

	case "wildcard":
		results, err = s.FTSIndex.SearchWildcard(req.Collection, req.Query, req.Limit)

	case "expression":
		var expr QueryExpr
		expr, err = ParseQueryExpression(req.Query)
		if err != nil {
			bad(w, err)
			return
		}
		results, err = s.FTSIndex.EvaluateExpression(req.Collection, expr, req.Limit)

	default: // "simple"
		mode = "simple"
		switch algo {
		case "bm25f":
			if fuzzy > 0 {
				results, err = s.FTSIndex.SearchBM25FFuzzy(req.Collection, tokens, req.Limit, fuzzy, req.FieldWeights)
			} else {
				results, err = s.FTSIndex.SearchBM25F(req.Collection, tokens, req.Limit, req.FieldWeights)
			}
		case "bm25":
			if fuzzy > 0 {
				results, err = s.FTSIndex.SearchBM25Fuzzy(req.Collection, req.Query, req.Limit, fuzzy)
			} else {
				results, err = s.FTSIndex.SearchBM25(req.Collection, req.Query, req.Limit)
			}
		case "tfidf":
			if fuzzy > 0 {
				results, err = s.FTSIndex.SearchFuzzy(req.Collection, req.Query, req.Limit, fuzzy)
			} else {
				results, err = s.FTSIndex.Search(req.Collection, req.Query, req.Limit)
			}
		case "pmisparse":
			if fuzzy > 0 {
				results, err = s.FTSIndex.SearchPMISparseFuzzy(req.Collection, req.Query, req.Limit, fuzzy)
			} else {
				results, err = s.FTSIndex.SearchPMISparse(req.Collection, req.Query, req.Limit)
			}
		default:
			bad(w, fmt.Errorf("unknown algorithm: %s, available: tfidf, bm25, bm25f, pmisparse", algo))
			return
		}
	}
	if err != nil {
		bad(w, err)
		return
	}

	// Track FTS search operation
	if s.Metrics != nil {
		s.Metrics.IncOp("fts_search", mode+"/"+algo)
	}

	// Apply metadata filter to results
	if allowed != nil {
		filtered := results[:0]
		for _, r := range results {
			if allowed[r.DocID] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Apply range filters
	if len(req.RangeMeta) > 0 && len(results) > 0 {
		results, err = s.FilterByRange(req.Collection, results, req.RangeMeta)
		if err != nil {
			bad(w, err)
			return
		}
	}

	// Apply per-query boosts/demotions (metadata-based score multipliers).
	results = s.applyBoostFTS(req.Collection, results, req.Boost)

	// Load full documents
	var resp FTSSearchResponse
	resp.Algorithm = algo
	resp.Mode = mode
	resp.Fuzzy = fuzzy
	resp.Lang = queryLang
	resp.StemmingActive = origStemmer != nil && !req.DisableStem
	resp.SynonymsActive = origSynonyms != nil && !req.DisableSynonyms
	if algo == "bm25f" {
		resp.FieldWeights = req.FieldWeights
	}
	resp.Results = make([]FTSResultWithDoc, 0, len(results))
	hlOpts := HighlightOptions{
		OpenTag:      req.HighlightTag,
		MaxFragments: req.MaxHighlights,
		FragmentSize: req.FragmentSize,
	}
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		for _, res := range results {
			v := bDocs.Get(kDoc(req.Collection, res.DocID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			// Skip expired docs
			if docPtr.ExpiresAt > 0 && docPtr.ExpiresAt < currentUnix() {
				continue
			}
			item := FTSResultWithDoc{
				Document:     *docPtr,
				Score:        res.Score,
				MatchedTerms: res.MatchedTerms,
			}
			if req.Highlight {
				item.Highlights = ExtractHighlights(docPtr.ContentMD, res.MatchedTerms, hlOpts)
			}
			resp.Results = append(resp.Results, item)
		}
		return nil
	})
	resp.Total = len(resp.Results)

	if searchStatsEnabled() {
		terms := make([]string, 0, len(tokens))
		for t := range tokens {
			terms = append(terms, t)
		}
		resp.Stats = &SearchStats{
			DurationMs:  float64(time.Since(start).Microseconds()) / 1000.0,
			QueryTerms:  terms,
			IndexSize:   resp.Total,
			TotalTokens: len(terms),
		}
	}

	resp.SpellCorrected = spellCorrected
	ok(w, resp)
}

// FTSReindexRequest is the HTTP request for FTS reindexing.
type FTSReindexRequest struct {
	Collection string `json:"collection"`
}

// handleFTSReindex re-indexes all documents in a collection using their lang field.
func (s *Server) handleFTSReindex(w http.ResponseWriter, r *http.Request) {
	var req FTSReindexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, fmt.Errorf("missing required field: collection"))
		return
	}
	if s.FTSIndex == nil {
		bad(w, fmt.Errorf("full-text search not initialized"))
		return
	}

	// Collect docs first (read tx), then index outside tx to avoid deadlock
	type reindexDoc struct {
		ID        string
		ContentMD string
		Lang      string
		Meta      map[string][]string
	}
	var docs []reindexDoc
	var skipped int
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}

		prefix := []byte("doc|" + req.Collection + "|")
		c := bDocs.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			docPtr, err := loadDoc(v)
			if err != nil || docPtr.ContentMD == "" {
				skipped++
				continue
			}
			if docPtr.ExpiresAt > 0 && docPtr.ExpiresAt < currentUnix() {
				skipped++
				continue
			}
			docs = append(docs, reindexDoc{
				ID:        docPtr.ID,
				ContentMD: docPtr.ContentMD,
				Lang:      docPtr.Lang,
				Meta:      docPtr.Meta,
			})
		}
		return nil
	})

	// Index outside the read tx to avoid BoltDB nested tx deadlock
	reindexed := 0
	for _, d := range docs {
		_ = s.FTSIndex.IndexWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)
		_ = s.FTSIndex.IndexPositionsWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)

		fields := map[string]string{"content": d.ContentMD}
		for mk, vals := range d.Meta {
			if len(vals) > 0 {
				fields["meta."+mk] = strings.Join(vals, " ")
			}
		}
		_ = s.FTSIndex.IndexFieldsWithLang(req.Collection, d.ID, fields, d.Lang)
		reindexed++
	}

	ok(w, map[string]interface{}{
		"status":    "ok",
		"reindexed": reindexed,
		"skipped":   skipped,
	})
}

// handleFTSLanguages returns the list of supported FTS languages.
func (s *Server) handleFTSLanguages(w http.ResponseWriter, _ *http.Request) {
	if s.FTSIndex == nil || s.FTSIndex.langRegistry == nil {
		ok(w, map[string]interface{}{"languages": []string{}})
		return
	}

	type langInfo struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}

	var langs []langInfo
	for _, code := range s.FTSIndex.langRegistry.Languages() {
		cfg := s.FTSIndex.langRegistry.Resolve(code)
		name := code
		if cfg != nil {
			name = cfg.Name
		}
		langs = append(langs, langInfo{Code: code, Name: name})
	}

	ok(w, map[string]interface{}{
		"languages":   langs,
		"defaultLang": s.FTSIndex.langRegistry.DefaultLang(),
	})
}

func currentUnix() int64 {
	return time.Now().Unix()
}

// Default English stop words
var defaultStopWords = map[string]bool{
	"the": true, "be": true, "to": true, "of": true, "and": true,
	"in": true, "that": true, "have": true, "it": true, "for": true,
	"not": true, "on": true, "with": true, "he": true, "as": true,
	"you": true, "do": true, "at": true, "this": true, "but": true,
	"his": true, "by": true, "from": true, "they": true, "we": true,
	"say": true, "her": true, "she": true, "or": true, "an": true,
	"will": true, "my": true, "one": true, "all": true, "would": true,
	"there": true, "their": true, "what": true, "so": true, "up": true,
	"out": true, "if": true, "about": true, "who": true, "get": true,
	"which": true, "go": true, "me": true, "when": true, "make": true,
	"can": true, "like": true, "no": true, "just": true, "him": true,
	"know": true, "take": true, "come": true, "could": true, "than": true,
	"look": true, "use": true, "into": true, "some": true, "them": true,
	"see": true, "other": true, "then": true, "now": true, "only": true,
	"its": true, "also": true, "after": true, "way": true, "our": true,
	"how": true, "where": true, "most": true, "been": true, "is": true,
	"was": true, "are": true, "were": true, "had": true, "has": true,
	"did": true, "does": true, "am": true,
}
