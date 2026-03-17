package main

import (
	"fmt"
	"sync"
)

// StorageBackend abstracts document storage operations so that each collection
// can use a different persistence layer (BoltDB, in-memory, S3, etc.).
type StorageBackend interface {
	// PutDoc stores serialized document bytes for collection/docID.
	PutDoc(collection, docID string, data []byte) error

	// GetDoc retrieves serialized document bytes. Returns nil, nil if not found.
	GetDoc(collection, docID string) ([]byte, error)

	// DeleteDoc removes a document.
	DeleteDoc(collection, docID string) error

	// ListDocs iterates all documents in a collection, calling fn for each.
	// Return non-nil error from fn to stop iteration.
	ListDocs(collection string, fn func(docID string, data []byte) error) error

	// PutByKey stores a docID lookup entry for collection/key/lang.
	PutByKey(collection, key, lang, docID string) error

	// GetByKey resolves (collection, key, lang) → docID. Returns "" if not found.
	GetByKey(collection, key, lang string) (string, error)

	// DeleteByKey removes the key→docID lookup entry.
	DeleteByKey(collection, key, lang string) error

	// Close releases resources held by the backend.
	Close() error

	// Name returns the backend type identifier (e.g. "boltdb", "memory", "s3").
	Name() string
}

// BackendRegistry maps collections to their storage backends.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]StorageBackend
	fallback StorageBackend // default backend (BoltDB)
}

// NewBackendRegistry creates a new registry with the given default backend.
func NewBackendRegistry(fallback StorageBackend) *BackendRegistry {
	return &BackendRegistry{
		backends: make(map[string]StorageBackend),
		fallback: fallback,
	}
}

// Get returns the backend for a collection, falling back to the default.
func (br *BackendRegistry) Get(collection string) StorageBackend {
	br.mu.RLock()
	defer br.mu.RUnlock()
	if b, ok := br.backends[collection]; ok {
		return b
	}
	return br.fallback
}

// Register sets a specific backend for a collection.
func (br *BackendRegistry) Register(collection string, b StorageBackend) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.backends[collection] = b
}

// Deregister removes a collection's backend (reverts to default). Closes the old backend.
func (br *BackendRegistry) Deregister(collection string) {
	br.mu.Lock()
	old, ok := br.backends[collection]
	delete(br.backends, collection)
	br.mu.Unlock()
	if ok {
		_ = old.Close()
	}
}

// Default returns the fallback (default) backend.
func (br *BackendRegistry) Default() StorageBackend {
	return br.fallback
}

// CreateBackend instantiates a StorageBackend from config values.
func CreateBackend(backendType string, cfg *StorageConfigDef) (StorageBackend, error) {
	switch backendType {
	case "", "boltdb":
		return nil, fmt.Errorf("boltdb backend is implicit, do not register it")
	case "memory":
		return NewMemoryBackend(), nil
	case "s3":
		if cfg == nil {
			return nil, fmt.Errorf("s3 backend requires storageConfig")
		}
		return NewS3Backend(cfg)
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", backendType)
	}
}
