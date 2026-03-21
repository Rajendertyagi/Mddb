package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ChunkEmbedding holds a single chunk's embedding vector.
type ChunkEmbedding struct {
	ChunkIndex int
	Vector     []float32
}

// EmbeddingRecord stores a document's embedding vector alongside metadata.
type EmbeddingRecord struct {
	DocID       string    `json:"docId"`
	Vector      []float32 `json:"vector"`
	Model       string    `json:"model"`
	Dimensions  int       `json:"dimensions"`
	CreatedAt   int64     `json:"createdAt"`
	ContentHash string    `json:"contentHash"`
}

// VectorStore handles persistence of embedding vectors in BoltDB.
type VectorStore struct {
	db         *bolt.DB
	bucketName []byte
	binlog     *Binlog
}

// NewVectorStore creates a new vector store backed by BoltDB.
func NewVectorStore(db *bolt.DB) *VectorStore {
	return &VectorStore{
		db:         db,
		bucketName: []byte("vectors"),
	}
}

// SetBinlog sets the binlog for replication logging.
func (vs *VectorStore) SetBinlog(bl *Binlog) {
	vs.binlog = bl
}

// EnsureBucket creates the vectors bucket if it doesn't exist.
func (vs *VectorStore) EnsureBucket() error {
	return vs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(vs.bucketName)
		return err
	})
}

// Put stores a single embedding record for a document (backward-compatible).
func (vs *VectorStore) Put(collection, docID string, vector []float32, model string, contentHash string) error {
	key := buildVecKey(collection, docID)
	data := marshalEmbeddingRecord(&EmbeddingRecord{
		DocID:       docID,
		Vector:      vector,
		Model:       model,
		Dimensions:  len(vector),
		CreatedAt:   time.Now().Unix(),
		ContentHash: contentHash,
	})

	err := vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return fmt.Errorf("vectors bucket not found")
		}
		return b.Put(key, data)
	})
	if err == nil && vs.binlog != nil {
		_ = vs.binlog.Append(&BinlogEntry{Type: BinlogPut, BucketName: "vectors", Key: copyBytes(key), Value: copyBytes(data)})
	}
	return err
}

// PutChunks stores multiple chunk embeddings for a document.
// Keys: vec|collection|docID#0, vec|collection|docID#1, etc.
func (vs *VectorStore) PutChunks(collection, docID string, chunks []ChunkEmbedding, model string, contentHash string) error {
	now := time.Now().Unix()

	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return fmt.Errorf("vectors bucket not found")
		}

		for _, chunk := range chunks {
			chunkKey := buildChunkKey(collection, docID, chunk.ChunkIndex)
			data := marshalEmbeddingRecord(&EmbeddingRecord{
				DocID:       docID,
				Vector:      chunk.Vector,
				Model:       model,
				Dimensions:  len(chunk.Vector),
				CreatedAt:   now,
				ContentHash: contentHash,
			})

			if err := b.Put(chunkKey, data); err != nil {
				return err
			}

			if vs.binlog != nil {
				_ = vs.binlog.Append(&BinlogEntry{Type: BinlogPut, BucketName: "vectors", Key: copyBytes(chunkKey), Value: copyBytes(data)})
			}
		}

		return nil
	})
}

// CleanStaleChunks removes chunk keys beyond the current chunk count from BoltDB and the in-memory index.
func (vs *VectorStore) CleanStaleChunks(collection, docID string, currentChunkCount int, index *VectorIndex) {
	prefix := []byte("vec|" + collection + "|" + docID + "#")

	var bo BinlogOps
	_ = vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			suffix := string(k[len(prefix):])
			idx, err := strconv.Atoi(suffix)
			if err != nil {
				continue
			}
			if idx >= currentChunkCount {
				_ = b.Delete(k)
				bo.Delete("vectors", k)
				if index != nil {
					chunkKey := fmt.Sprintf("%s#%d", docID, idx)
					index.Remove(collection, chunkKey)
				}
			}
		}
		return nil
	})
	bo.FlushTo(vs.binlog)

	// Also clean the old non-chunked key if chunks > 1
	if currentChunkCount > 1 {
		oldKey := buildVecKey(collection, docID)
		var bo2 BinlogOps
		_ = vs.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(vs.bucketName)
			if b == nil {
				return nil
			}
			if v := b.Get(oldKey); v != nil {
				_ = b.Delete(oldKey)
				bo2.Delete("vectors", oldKey)
				if index != nil {
					index.Remove(collection, docID)
				}
			}
			return nil
		})
		bo2.FlushTo(vs.binlog)
	}
}

// Get retrieves the first embedding record for a document (chunk 0 or legacy single record).
func (vs *VectorStore) Get(collection, docID string) (*EmbeddingRecord, error) {
	chunkKey := buildChunkKey(collection, docID, 0)
	var rec *EmbeddingRecord

	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		v := b.Get(chunkKey)
		if v != nil {
			var err error
			rec, err = unmarshalEmbeddingRecord(v)
			return err
		}
		// Fallback to legacy non-chunked key
		v = b.Get(buildVecKey(collection, docID))
		if v == nil {
			return nil
		}
		var err error
		rec, err = unmarshalEmbeddingRecord(v)
		return err
	})

	return rec, err
}

// Delete removes all embedding records for a document (all chunks + legacy key).
func (vs *VectorStore) Delete(collection, docID string) error {
	legacyKey := buildVecKey(collection, docID)
	prefix := []byte("vec|" + collection + "|" + docID + "#")

	var bo BinlogOps
	err := vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		if b.Get(legacyKey) != nil {
			_ = b.Delete(legacyKey)
			bo.Delete("vectors", legacyKey)
		}

		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
			bo.Delete("vectors", k)
		}
		return nil
	})
	if err == nil {
		bo.FlushTo(vs.binlog)
	}
	return err
}

// LoadCollection loads all embedding records for a collection.
// Returns records keyed by their full suffix (docID or docID#N).
func (vs *VectorStore) LoadCollection(collection string) (map[string]*EmbeddingRecord, error) {
	prefix := []byte("vec|" + collection + "|")
	records := make(map[string]*EmbeddingRecord)

	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			rec, err := unmarshalEmbeddingRecord(v)
			if err != nil {
				continue
			}
			suffix := string(k[len(prefix):])
			records[suffix] = rec
		}
		return nil
	})

	return records, err
}

// CountByCollection counts embeddings per collection (counting unique docIDs, not chunks).
// Handles keys like vec|coll|docID and vec|coll|docID#N where docID may contain | characters.
func (vs *VectorStore) CountByCollection() (map[string]int, error) {
	counts := make(map[string]int)
	seen := make(map[string]bool)

	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			ks := string(k)
			// Key format: vec|collection|docID or vec|collection|docID#N
			// Find first | after "vec|"
			if len(ks) < 5 || ks[:4] != "vec|" {
				continue
			}
			rest := ks[4:] // "collection|..."
			pipeIdx := strings.IndexByte(rest, '|')
			if pipeIdx < 0 {
				continue
			}
			coll := rest[:pipeIdx]
			docIDPart := rest[pipeIdx+1:]
			// Strip chunk suffix (#N)
			if hashIdx := strings.LastIndexByte(docIDPart, '#'); hashIdx >= 0 {
				// Only strip if what follows # is a number
				suffix := docIDPart[hashIdx+1:]
				if _, err := strconv.Atoi(suffix); err == nil {
					docIDPart = docIDPart[:hashIdx]
				}
			}
			dedupKey := coll + "\x00" + docIDPart
			if !seen[dedupKey] {
				seen[dedupKey] = true
				counts[coll]++
			}
		}
		return nil
	})
	return counts, err
}

// CountChunksByCollection counts total chunk embeddings per collection (including multi-chunk docs).
func (vs *VectorStore) CountChunksByCollection() (map[string]int, error) {
	counts := make(map[string]int)

	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			ks := string(k)
			if len(ks) < 5 || ks[:4] != "vec|" {
				continue
			}
			rest := ks[4:]
			pipeIdx := strings.IndexByte(rest, '|')
			if pipeIdx < 0 {
				continue
			}
			counts[rest[:pipeIdx]]++
		}
		return nil
	})
	return counts, err
}

// buildVecKey builds key: vec|collection|docID (legacy, non-chunked)
func buildVecKey(collection, docID string) []byte {
	return []byte("vec|" + collection + "|" + docID)
}

// buildChunkKey builds key: vec|collection|docID#N
func buildChunkKey(collection, docID string, chunkIndex int) []byte {
	return []byte(fmt.Sprintf("vec|%s|%s#%d", collection, docID, chunkIndex))
}

// ContentHash computes SHA256 hash of content for staleness detection.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8]) // first 8 bytes = 16 hex chars
}

// Binary serialization for embedding records (compact, no JSON overhead).
// Format: [4B model_len][model][4B dims][4B*dims float32s][8B created_at][4B hash_len][hash][4B docid_len][docid]
func marshalEmbeddingRecord(rec *EmbeddingRecord) []byte {
	modelBytes := []byte(rec.Model)
	hashBytes := []byte(rec.ContentHash)
	docIDBytes := []byte(rec.DocID)

	size := 4 + len(modelBytes) + // model
		4 + // dimensions
		4*len(rec.Vector) + // vectors
		8 + // created_at
		4 + len(hashBytes) + // content hash
		4 + len(docIDBytes) // docID

	buf := make([]byte, size)
	offset := 0

	// model
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(modelBytes))) // #nosec G115 -- model name length always small
	offset += 4
	copy(buf[offset:], modelBytes)
	offset += len(modelBytes)

	// dimensions
	binary.LittleEndian.PutUint32(buf[offset:], uint32(rec.Dimensions)) // #nosec G115 -- dimensions always positive and bounded
	offset += 4

	// vectors
	for _, v := range rec.Vector {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(v))
		offset += 4
	}

	// created_at
	binary.LittleEndian.PutUint64(buf[offset:], uint64(rec.CreatedAt)) // #nosec G115 -- timestamp always non-negative
	offset += 8

	// content hash
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(hashBytes))) // #nosec G115 -- hash length always small
	offset += 4
	copy(buf[offset:], hashBytes)
	offset += len(hashBytes)

	// docID
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(docIDBytes))) // #nosec G115 -- docID length always small
	offset += 4
	copy(buf[offset:], docIDBytes)

	return buf
}

func unmarshalEmbeddingRecord(data []byte) (*EmbeddingRecord, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("embedding record too short")
	}

	offset := 0
	rec := &EmbeddingRecord{}

	// model
	modelLen := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+modelLen > len(data) {
		return nil, fmt.Errorf("invalid model length")
	}
	rec.Model = string(data[offset : offset+modelLen])
	offset += modelLen

	// dimensions
	rec.Dimensions = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// vectors
	if offset+rec.Dimensions*4 > len(data) {
		return nil, fmt.Errorf("invalid vector data")
	}
	rec.Vector = make([]float32, rec.Dimensions)
	for i := 0; i < rec.Dimensions; i++ {
		rec.Vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
	}

	// created_at
	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid created_at")
	}
	rec.CreatedAt = int64(binary.LittleEndian.Uint64(data[offset:])) // #nosec G115 -- timestamp within int64 range
	offset += 8

	// content hash
	if offset+4 > len(data) {
		return nil, fmt.Errorf("invalid hash length")
	}
	hashLen := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+hashLen > len(data) {
		return nil, fmt.Errorf("invalid hash data")
	}
	rec.ContentHash = string(data[offset : offset+hashLen])
	offset += hashLen

	// docID
	if offset+4 > len(data) {
		return nil, fmt.Errorf("invalid docID length")
	}
	docIDLen := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+docIDLen > len(data) {
		return nil, fmt.Errorf("invalid docID data")
	}
	rec.DocID = string(data[offset : offset+docIDLen])

	return rec, nil
}

// splitKey splits a BoltDB key by '|' separator.
func splitKey(key []byte) []string {
	var parts []string
	start := 0
	for i, b := range key {
		if b == '|' {
			parts = append(parts, string(key[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, string(key[start:]))
	return parts
}
