package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var bucketSynonyms = []byte("synonyms")

// SynonymManager manages synonym dictionaries per collection.
type SynonymManager struct {
	db     *bolt.DB
	mu     sync.RWMutex
	cache  map[string]map[string][]string // collection -> term -> synonyms
	binlog *Binlog
}

// SynonymRequest is the HTTP request for synonym CRUD.
type SynonymRequest struct {
	Collection string   `json:"collection"`
	Term       string   `json:"term"`
	Synonyms   []string `json:"synonyms"`
}

// SynonymEntry represents a single synonym mapping for API responses.
type SynonymEntry struct {
	Term     string   `json:"term"`
	Synonyms []string `json:"synonyms"`
}

// SynonymListResponse is the HTTP response for listing synonyms.
type SynonymListResponse struct {
	Collection string         `json:"collection"`
	Entries    []SynonymEntry `json:"entries"`
	Total      int            `json:"total"`
}

// NewSynonymManager creates a new synonym manager.
func NewSynonymManager(db *bolt.DB) *SynonymManager {
	return &SynonymManager{
		db:    db,
		cache: make(map[string]map[string][]string),
	}
}

// SetBinlog sets the binlog for replication logging.
func (sm *SynonymManager) SetBinlog(bl *Binlog) {
	sm.binlog = bl
}

// EnsureBucket creates the synonyms bucket if it doesn't exist.
func (sm *SynonymManager) EnsureBucket() error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketSynonyms)
		return err
	})
}

// LoadAll loads all synonym entries from BoltDB into the in-memory cache.
func (sm *SynonymManager) LoadAll() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.cache = make(map[string]map[string][]string)

	return sm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSynonyms)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			key := string(k)
			parts := strings.SplitN(key, "|", 3)
			if len(parts) != 3 || parts[0] != "syn" {
				return nil
			}
			collection := parts[1]
			term := parts[2]

			var synonyms []string
			if err := json.Unmarshal(v, &synonyms); err != nil {
				return nil // skip malformed entries
			}

			if sm.cache[collection] == nil {
				sm.cache[collection] = make(map[string][]string)
			}
			sm.cache[collection][term] = synonyms
			return nil
		})
	})
}

// Set adds or updates synonyms for a term in a collection.
func (sm *SynonymManager) Set(collection, term string, synonyms []string) error {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" || collection == "" {
		return fmt.Errorf("collection and term are required")
	}

	// Normalize synonyms
	normalized := make([]string, 0, len(synonyms))
	for _, s := range synonyms {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && s != term {
			normalized = append(normalized, s)
		}
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	key := synKey(collection, term)
	var bo BinlogOps
	err = sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSynonyms)
		bo.Put("synonyms", key, data)
		return b.Put(key, data)
	})
	if err != nil {
		return err
	}
	bo.FlushTo(sm.binlog)

	sm.mu.Lock()
	if sm.cache[collection] == nil {
		sm.cache[collection] = make(map[string][]string)
	}
	sm.cache[collection][term] = normalized
	sm.mu.Unlock()

	return nil
}

// Get returns synonyms for a term in a collection.
func (sm *SynonymManager) Get(collection, term string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if coll, ok := sm.cache[collection]; ok {
		return coll[strings.ToLower(term)]
	}
	return nil
}

// Delete removes synonyms for a term in a collection.
func (sm *SynonymManager) Delete(collection, term string) error {
	term = strings.ToLower(strings.TrimSpace(term))
	key := synKey(collection, term)
	var bo BinlogOps
	err := sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSynonyms)
		bo.Delete("synonyms", key)
		return b.Delete(key)
	})
	if err != nil {
		return err
	}
	bo.FlushTo(sm.binlog)

	sm.mu.Lock()
	if coll, ok := sm.cache[collection]; ok {
		delete(coll, term)
	}
	sm.mu.Unlock()

	return nil
}

// List returns all synonym entries for a collection.
func (sm *SynonymManager) List(collection string) map[string][]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string][]string)
	if coll, ok := sm.cache[collection]; ok {
		for k, v := range coll {
			cp := make([]string, len(v))
			copy(cp, v)
			result[k] = cp
		}
	}
	return result
}

// Expand expands a list of terms with their synonyms (bidirectional).
// If "big" has synonym "large", then querying "large" also returns "big".
func (sm *SynonymManager) Expand(collection string, terms []string) []string {
	sm.mu.RLock()
	coll, ok := sm.cache[collection]
	sm.mu.RUnlock()

	if !ok {
		return terms
	}

	seen := make(map[string]bool, len(terms))
	expanded := make([]string, 0, len(terms)*2)

	for _, term := range terms {
		t := strings.ToLower(term)
		if seen[t] {
			continue
		}
		seen[t] = true
		expanded = append(expanded, t)

		// Forward: term -> synonyms
		if syns, ok := coll[t]; ok {
			for _, syn := range syns {
				if !seen[syn] {
					seen[syn] = true
					expanded = append(expanded, syn)
				}
			}
		}

		// Reverse: check if this term is a synonym of another term
		for baseTerm, syns := range coll {
			for _, syn := range syns {
				if syn == t {
					if !seen[baseTerm] {
						seen[baseTerm] = true
						expanded = append(expanded, baseTerm)
					}
					// Also add other synonyms from the same group
					for _, otherSyn := range syns {
						if !seen[otherSyn] {
							seen[otherSyn] = true
							expanded = append(expanded, otherSyn)
						}
					}
					break
				}
			}
		}
	}

	return expanded
}

// LoadDefaults loads built-in default synonym groups for a collection.
func (sm *SynonymManager) LoadDefaults(collection string) error {
	for _, group := range defaultSynonymGroups {
		if len(group) < 2 {
			continue
		}
		// Set the first term as the base with the rest as synonyms
		base := group[0]
		synonyms := group[1:]
		if err := sm.Set(collection, base, synonyms); err != nil {
			return err
		}
	}
	return nil
}

// synKey builds the synonym BoltDB key.
func synKey(collection, term string) []byte {
	return []byte(fmt.Sprintf("syn|%s|%s", collection, term))
}

// Default English synonym groups
var defaultSynonymGroups = [][]string{
	{"big", "large", "huge", "enormous"},
	{"fast", "quick", "rapid", "swift"},
	{"small", "tiny", "little", "minute"},
	{"happy", "glad", "pleased", "joyful"},
	{"start", "begin", "commence", "initiate"},
	{"end", "finish", "complete", "conclude"},
	{"error", "mistake", "fault", "bug"},
	{"create", "make", "build", "construct"},
	{"delete", "remove", "erase", "eliminate"},
	{"update", "modify", "change", "alter"},
}

// --- HTTP handlers ---

func (s *Server) handleSynonyms(w http.ResponseWriter, r *http.Request) {
	if s.SynonymManager == nil {
		bad(w, fmt.Errorf("synonym manager not initialized"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleSynonymsList(w, r)
	case http.MethodPost:
		s.handleSynonymsSet(w, r)
	case http.MethodDelete:
		s.handleSynonymsDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSynonymsList(w http.ResponseWriter, r *http.Request) {
	collection := r.URL.Query().Get("collection")
	if collection == "" {
		bad(w, fmt.Errorf("missing required parameter: collection"))
		return
	}

	entries := s.SynonymManager.List(collection)
	resp := SynonymListResponse{
		Collection: collection,
		Entries:    make([]SynonymEntry, 0, len(entries)),
		Total:      len(entries),
	}
	for term, syns := range entries {
		resp.Entries = append(resp.Entries, SynonymEntry{Term: term, Synonyms: syns})
	}
	ok(w, resp)
}

func (s *Server) handleSynonymsSet(w http.ResponseWriter, r *http.Request) {
	var req SynonymRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Term == "" {
		bad(w, fmt.Errorf("missing required fields: collection, term"))
		return
	}
	if len(req.Synonyms) == 0 {
		bad(w, fmt.Errorf("synonyms list cannot be empty"))
		return
	}

	if err := s.SynonymManager.Set(req.Collection, req.Term, req.Synonyms); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "ok"})
}

func (s *Server) handleSynonymsDelete(w http.ResponseWriter, r *http.Request) {
	var req SynonymRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Term == "" {
		bad(w, fmt.Errorf("missing required fields: collection, term"))
		return
	}

	if err := s.SynonymManager.Delete(req.Collection, req.Term); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "ok"})
}
