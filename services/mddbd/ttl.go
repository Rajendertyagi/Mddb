package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketTTL    = []byte("ttl")
	bucketTTLRev = []byte("ttlrev")
)

// TTLManager handles document time-to-live expiry.
type TTLManager struct {
	db     *bolt.DB
	server *Server
	stopCh chan struct{}
}

// NewTTLManager creates a new TTL manager.
func NewTTLManager(db *bolt.DB, server *Server) *TTLManager {
	return &TTLManager{db: db, server: server, stopCh: make(chan struct{})}
}

// EnsureBuckets creates the TTL buckets if they don't exist.
func (t *TTLManager) EnsureBuckets() error {
	return t.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketTTL); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketTTLRev)
		return err
	})
}

// Set stores a TTL entry for a document. Removes any previous TTL first.
func (t *TTLManager) Set(collection, docID string, expiresAt int64) error {
	return t.db.Update(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		bRev := tx.Bucket(bucketTTLRev)

		revKey := ttlRevKey(collection, docID)

		// Remove old TTL entry if exists
		if old := bRev.Get(revKey); old != nil {
			oldExpiry := int64(binary.BigEndian.Uint64(old))
			_ = bTTL.Delete(ttlKey(oldExpiry, collection, docID))
		}

		if expiresAt <= 0 {
			// TTL removed
			return bRev.Delete(revKey)
		}

		// Store forward key: expiresAt|collection|docID -> empty
		if err := bTTL.Put(ttlKey(expiresAt, collection, docID), []byte{}); err != nil {
			return err
		}

		// Store reverse key: collection|docID -> expiresAt (8 bytes)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(expiresAt))
		return bRev.Put(revKey, buf[:])
	})
}

// Remove deletes TTL entries for a document.
func (t *TTLManager) Remove(collection, docID string) error {
	return t.db.Update(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		bRev := tx.Bucket(bucketTTLRev)

		revKey := ttlRevKey(collection, docID)
		if old := bRev.Get(revKey); old != nil {
			oldExpiry := int64(binary.BigEndian.Uint64(old))
			_ = bTTL.Delete(ttlKey(oldExpiry, collection, docID))
		}
		return bRev.Delete(revKey)
	})
}

// StartCleanup runs a background goroutine that reaps expired documents.
func (t *TTLManager) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-t.stopCh:
				return
			case <-ticker.C:
				t.cleanup()
			}
		}
	}()
}

// Stop signals the cleanup goroutine to stop.
func (t *TTLManager) Stop() {
	close(t.stopCh)
}

func (t *TTLManager) cleanup() {
	now := time.Now().Unix()
	threshold := ttlKey(now, "\xff", "\xff") // scan everything <= now

	// Collect expired entries
	type expiredDoc struct {
		collection, key, lang string
	}
	var expired []expiredDoc

	_ = t.db.View(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		if bTTL == nil {
			return nil
		}
		c := bTTL.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			// Key format: %020d|collection|docID
			if string(k) >= string(threshold) {
				break
			}
			// Parse collection and docID from key
			parts := strings.SplitN(string(k), "|", 3)
			if len(parts) < 3 {
				continue
			}
			coll := parts[1]
			docID := parts[2]

			// Look up key and lang from the document
			bDocs := tx.Bucket([]byte("docs"))
			if v := bDocs.Get(kDoc(coll, docID)); v != nil {
				var doc Doc
				if err := json.Unmarshal(v, &doc); err == nil {
					expired = append(expired, expiredDoc{coll, doc.Key, doc.Lang})
				}
			}
		}
		return nil
	})

	// Delete expired documents
	for _, ed := range expired {
		if err := t.server.deleteDocumentInternal(ed.collection, ed.key, ed.lang); err != nil {
			log.Printf("TTL cleanup: failed to delete %s/%s/%s: %v", ed.collection, ed.key, ed.lang, err)
			continue
		}
		// Also remove TTL entries
		docID := genID(ed.collection, ed.key, ed.lang)
		_ = t.Remove(ed.collection, docID)
		log.Printf("TTL cleanup: expired %s/%s/%s", ed.collection, ed.key, ed.lang)
	}
}

// ttlKey builds the forward TTL bucket key.
func ttlKey(expiresAt int64, collection, docID string) []byte {
	return []byte(fmt.Sprintf("%020d|%s|%s", expiresAt, collection, docID))
}

// ttlRevKey builds the reverse TTL lookup key.
func ttlRevKey(collection, docID string) []byte {
	return []byte(collection + "|" + docID)
}

// --- HTTP handlers ---

// SetTTLRequest represents a request to set/remove TTL on a document.
type SetTTLRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
	TTL        int64  `json:"ttl"` // seconds; 0 = remove TTL
}

func (s *Server) handleSetTTL(w http.ResponseWriter, r *http.Request) {
	var req SetTTLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, fmt.Errorf("missing required fields"))
		return
	}

	docID := genID(req.Collection, req.Key, req.Lang)

	// Update ExpiresAt on the document itself
	now := time.Now().Unix()
	var expiresAt int64
	if req.TTL > 0 {
		expiresAt = now + req.TTL
	}

	// Update document in DB
	var updated Doc
	err := s.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		v := bDocs.Get(kDoc(req.Collection, docID))
		if v == nil {
			return fmt.Errorf("document not found")
		}
		if err := json.Unmarshal(v, &updated); err != nil {
			return err
		}
		updated.ExpiresAt = expiresAt
		buf, _ := json.Marshal(updated)
		return bDocs.Put(kDoc(req.Collection, docID), buf)
	})
	if err != nil {
		bad(w, err)
		return
	}

	// Update TTL bucket
	if s.TTLManager != nil {
		if expiresAt > 0 {
			_ = s.TTLManager.Set(req.Collection, docID, expiresAt)
		} else {
			_ = s.TTLManager.Remove(req.Collection, docID)
		}
	}

	ok(w, updated)
}
