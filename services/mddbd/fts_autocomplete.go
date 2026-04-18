package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"

	bolt "go.etcd.io/bbolt"
)

// AutocompleteItem is a single suggestion returned to the caller.
// DocCount is the document frequency of the term within the scanned scope;
// consumers can use it to render popularity badges next to suggestions.
type AutocompleteItem struct {
	Term     string `json:"term"`
	Field    string `json:"field,omitempty"`
	DocCount int    `json:"docCount"`
}

// autocompleteMaxPrefixLen bounds how long a client prefix can be before we
// reject it — unbounded prefixes can trigger expensive scans without
// improving suggestion quality.
const autocompleteMaxPrefixLen = 32

// autocompleteMaxScan caps how many inverted-index entries a single call may
// visit. Well-structured corpora never hit this ceiling; it exists to stop a
// pathological prefix like "a" from walking the whole vocabulary.
const autocompleteMaxScan = 10_000

// AutocompleteResponse is returned by the HTTP handler.
type AutocompleteResponse struct {
	Items []AutocompleteItem `json:"items"`
	Total int                `json:"total"`
	Query string             `json:"query"`
	Field string             `json:"field,omitempty"`
}

// normalizeAutocompletePrefix lowercases and strips non-alphanumerics so the
// query shape matches whatever the tokenizer produced at index time. Returns
// an empty string when the prefix contains nothing indexable.
func normalizeAutocompletePrefix(q string) string {
	q = strings.ToLower(q)
	var b strings.Builder
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			// Stop on first separator — the tokenizer treats them as word
			// boundaries, so "mar d" and "mar" should autocomplete identically.
			break
		}
	}
	return b.String()
}

// Autocomplete walks the FTS inverted index for terms starting with the given
// prefix and returns the top-N by document frequency. When field is empty the
// collection-wide index (`fts` bucket) is used; otherwise the field-scoped
// index (`ftsf` bucket) drives the scan so callers can autocomplete titles or
// tags independently of body content.
func (f *FTSIndex) Autocomplete(collection, prefix, field string, topN int) ([]AutocompleteItem, error) {
	if collection == "" {
		return nil, errors.New("missing collection")
	}
	prefix = normalizeAutocompletePrefix(prefix)
	if prefix == "" {
		return nil, nil
	}
	if len(prefix) > autocompleteMaxPrefixLen {
		prefix = prefix[:autocompleteMaxPrefixLen]
	}
	if topN <= 0 {
		topN = 10
	}

	counts := make(map[string]int)

	err := f.db.View(func(tx *bolt.Tx) error {
		if field != "" {
			return autocompleteScanField(tx, collection, field, prefix, counts)
		}
		return autocompleteScanGlobal(tx, collection, prefix, counts)
	})
	if err != nil {
		return nil, err
	}

	items := make([]AutocompleteItem, 0, len(counts))
	for term, count := range counts {
		items = append(items, AutocompleteItem{
			Term:     term,
			Field:    field,
			DocCount: count,
		})
	}
	// Primary sort: highest docCount first. Secondary: alphabetic so equal
	// popularity produces a stable suggestion order.
	sort.Slice(items, func(i, j int) bool {
		if items[i].DocCount != items[j].DocCount {
			return items[i].DocCount > items[j].DocCount
		}
		return items[i].Term < items[j].Term
	})
	if len(items) > topN {
		items = items[:topN]
	}
	return items, nil
}

// autocompleteScanGlobal counts matches against the collection-wide FTS
// bucket. Key format: fts|<coll>|<term>|<docID>.
func autocompleteScanGlobal(tx *bolt.Tx, collection, prefix string, counts map[string]int) error {
	b := tx.Bucket(bucketFTS)
	if b == nil {
		return nil
	}
	scanPrefix := []byte("fts|" + collection + "|" + prefix)
	base := len("fts|" + collection + "|")
	c := b.Cursor()
	scanned := 0
	for k, _ := c.Seek(scanPrefix); k != nil && bytes.HasPrefix(k, scanPrefix); k, _ = c.Next() {
		if scanned++; scanned > autocompleteMaxScan {
			return nil
		}
		rest := string(k[base:])
		idx := strings.IndexByte(rest, '|')
		if idx <= 0 {
			continue
		}
		term := rest[:idx]
		counts[term]++
	}
	return nil
}

// autocompleteScanField counts matches against the field-scoped BM25F bucket.
// Key format: ftsf|<coll>|<field>|<term>|<docID>.
func autocompleteScanField(tx *bolt.Tx, collection, field, prefix string, counts map[string]int) error {
	b := tx.Bucket(bucketFTSF)
	if b == nil {
		return nil
	}
	scanPrefix := []byte("ftsf|" + collection + "|" + field + "|" + prefix)
	base := len("ftsf|" + collection + "|" + field + "|")
	c := b.Cursor()
	scanned := 0
	for k, _ := c.Seek(scanPrefix); k != nil && bytes.HasPrefix(k, scanPrefix); k, _ = c.Next() {
		if scanned++; scanned > autocompleteMaxScan {
			return nil
		}
		rest := string(k[base:])
		idx := strings.IndexByte(rest, '|')
		if idx <= 0 {
			continue
		}
		term := rest[:idx]
		counts[term]++
	}
	return nil
}

// handleAutocomplete serves GET /v1/autocomplete?collection=X&q=mar&field=Y&topN=10.
func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.FTSIndex == nil {
		bad(w, errors.New("full-text search not initialized"))
		return
	}

	collection := r.URL.Query().Get("collection")
	query := r.URL.Query().Get("q")
	field := r.URL.Query().Get("field")
	topN := parseIntParam(r, "topN", 10)

	if collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if query == "" {
		// Empty prefix returns empty result set rather than 400 — saves
		// client-side guards on every keystroke that clears the input.
		ok(w, AutocompleteResponse{Items: []AutocompleteItem{}, Query: query, Field: field})
		return
	}

	items, err := s.FTSIndex.Autocomplete(collection, query, field, topN)
	if err != nil {
		bad(w, err)
		return
	}
	if s.Metrics != nil {
		s.Metrics.IncOp("autocomplete", field)
	}
	ok(w, AutocompleteResponse{
		Items: items,
		Total: len(items),
		Query: query,
		Field: field,
	})
}

// parseIntParam reads a URL query parameter as int with a fallback. Used for
// numeric knobs (topN, limit) where zero/negative means "use default".
func parseIntParam(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

// String formats a short debug summary of an item. Used by logs and tests.
func (a AutocompleteItem) String() string {
	if a.Field != "" {
		return fmt.Sprintf("%s[%s]=%d", a.Term, a.Field, a.DocCount)
	}
	return fmt.Sprintf("%s=%d", a.Term, a.DocCount)
}
