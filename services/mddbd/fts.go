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
	bucketFTS    = []byte("fts")
	bucketFTSRev = []byte("ftsrev")
)

// FTSIndex provides full-text search using an inverted index in BoltDB.
type FTSIndex struct {
	db        *bolt.DB
	stopWords map[string]bool
	binlog    *Binlog
}

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

// FTSSearchRequest is the HTTP request for full-text search.
type FTSSearchRequest struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
	Algorithm  string `json:"algorithm"` // "tfidf" (default) or "bm25"
}

// FTSSearchResponse is the HTTP response for full-text search.
type FTSSearchResponse struct {
	Results   []FTSResultWithDoc `json:"results"`
	Total     int                `json:"total"`
	Algorithm string             `json:"algorithm"`
}

// FTSResultWithDoc includes the full document in the result.
type FTSResultWithDoc struct {
	Document     Doc      `json:"document"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matchedTerms"`
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
		if _, err := tx.CreateBucketIfNotExists(bucketFTS); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketFTSRev)
		return err
	})
}

// Tokenize splits text into a frequency map of normalized terms.
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
					terms[w]++
				}
			}
			word.Reset()
		}
	}
	if word.Len() >= 2 {
		w := word.String()
		if !f.stopWords[w] {
			terms[w]++
		}
	}
	return terms
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
			binary.LittleEndian.PutUint32(buf[:], uint32(count))
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
			docLength += uint32(count)
		}
		return f.IndexBM25Meta(tx, collection, docID, docLength)
	})
	if err == nil {
		bo.FlushTo(f.binlog)
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
		bo.Delete("ftsrev", revKey)
		return bRev.Delete(revKey)
	})
	if err == nil {
		bo.FlushTo(f.binlog)
	}
	return err
}

// Search performs a full-text search and returns matching document IDs with scores.
func (f *FTSIndex) Search(collection, query string, limit int) ([]FTSResult, error) {
	queryTerms := f.Tokenize(query)
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

// --- HTTP handler ---

func (s *Server) handleFTS(w http.ResponseWriter, r *http.Request) {
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

	algo := req.Algorithm
	if algo == "" {
		algo = "tfidf"
	}

	var results []FTSResult
	var err error
	switch algo {
	case "bm25":
		results, err = s.FTSIndex.SearchBM25(req.Collection, req.Query, req.Limit)
	case "tfidf":
		results, err = s.FTSIndex.Search(req.Collection, req.Query, req.Limit)
	default:
		bad(w, fmt.Errorf("unknown algorithm: %s, available: tfidf, bm25", algo))
		return
	}
	if err != nil {
		bad(w, err)
		return
	}

	// Load full documents
	var resp FTSSearchResponse
	resp.Algorithm = algo
	resp.Results = make([]FTSResultWithDoc, 0, len(results))
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		for _, res := range results {
			v := bDocs.Get(kDoc(req.Collection, res.DocID))
			if v == nil {
				continue
			}
			var doc Doc
			if err := json.Unmarshal(v, &doc); err != nil {
				continue
			}
			// Skip expired docs
			if doc.ExpiresAt > 0 && doc.ExpiresAt < currentUnix() {
				continue
			}
			resp.Results = append(resp.Results, FTSResultWithDoc{
				Document:     doc,
				Score:        res.Score,
				MatchedTerms: res.MatchedTerms,
			})
		}
		return nil
	})
	resp.Total = len(resp.Results)

	ok(w, resp)
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
