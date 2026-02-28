package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	bolt "go.etcd.io/bbolt"
)

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
}

// NewVectorStore creates a new vector store backed by BoltDB.
func NewVectorStore(db *bolt.DB) *VectorStore {
	return &VectorStore{
		db:         db,
		bucketName: []byte("vectors"),
	}
}

// EnsureBucket creates the vectors bucket if it doesn't exist.
func (vs *VectorStore) EnsureBucket() error {
	return vs.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(vs.bucketName)
		return err
	})
}

// Put stores an embedding record for a document.
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

	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return fmt.Errorf("vectors bucket not found")
		}
		return b.Put(key, data)
	})
}

// Get retrieves an embedding record for a document.
func (vs *VectorStore) Get(collection, docID string) (*EmbeddingRecord, error) {
	key := buildVecKey(collection, docID)
	var rec *EmbeddingRecord

	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		v := b.Get(key)
		if v == nil {
			return nil
		}
		var err error
		rec, err = unmarshalEmbeddingRecord(v)
		return err
	})

	return rec, err
}

// Delete removes an embedding record for a document.
func (vs *VectorStore) Delete(collection, docID string) error {
	key := buildVecKey(collection, docID)
	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		return b.Delete(key)
	})
}

// LoadCollection loads all embedding records for a collection.
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
			records[rec.DocID] = rec
		}
		return nil
	})

	return records, err
}

// CountByCollection counts embeddings per collection.
func (vs *VectorStore) CountByCollection() (map[string]int, error) {
	counts := make(map[string]int)
	err := vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(vs.bucketName)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			// key format: vec|collection|docID
			parts := splitKey(k)
			if len(parts) >= 2 {
				counts[parts[1]]++
			}
		}
		return nil
	})
	return counts, err
}

// buildVecKey builds key: vec|collection|docID
func buildVecKey(collection, docID string) []byte {
	return []byte("vec|" + collection + "|" + docID)
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
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(modelBytes)))
	offset += 4
	copy(buf[offset:], modelBytes)
	offset += len(modelBytes)

	// dimensions
	binary.LittleEndian.PutUint32(buf[offset:], uint32(rec.Dimensions))
	offset += 4

	// vectors
	for _, v := range rec.Vector {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(v))
		offset += 4
	}

	// created_at
	binary.LittleEndian.PutUint64(buf[offset:], uint64(rec.CreatedAt))
	offset += 8

	// content hash
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(hashBytes)))
	offset += 4
	copy(buf[offset:], hashBytes)
	offset += len(hashBytes)

	// docID
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(docIDBytes)))
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
	rec.CreatedAt = int64(binary.LittleEndian.Uint64(data[offset:]))
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
