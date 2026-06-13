package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// CacheEntry represents a cached document
type CacheEntry struct {
	Data      []byte
	ExpiresAt int64
}

// DocumentCache is a simple LRU cache for hot documents
type DocumentCache struct {
	cache     map[string]*CacheEntry
	mu        sync.RWMutex
	maxSize   int
	ttl       int64 // seconds
	hits      uint64
	misses    uint64
	stopCh    chan struct{} // closed by Close() to stop the cleanup goroutine
	closeOnce sync.Once
}

// NewDocumentCache creates a new document cache
func NewDocumentCache(maxSize int, ttlSeconds int64) *DocumentCache {
	if maxSize <= 0 {
		maxSize = 1000 // Default 1000 documents
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300 // Default 5 minutes
	}

	cache := &DocumentCache{
		cache:   make(map[string]*CacheEntry, maxSize),
		maxSize: maxSize,
		ttl:     ttlSeconds,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Close stops the background cleanup goroutine. Safe to call multiple times.
// Used so a replication restore can recycle the cache without leaking a
// goroutine per restore (GO-004 / GO-006).
func (dc *DocumentCache) Close() {
	dc.closeOnce.Do(func() {
		if dc.stopCh != nil {
			close(dc.stopCh)
		}
	})
}

// Get retrieves a document from cache
func (dc *DocumentCache) Get(key string) ([]byte, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	entry, exists := dc.cache[key]
	if !exists {
		atomic.AddUint64(&dc.misses, 1)
		return nil, false
	}

	// Check if expired
	if time.Now().Unix() > entry.ExpiresAt {
		atomic.AddUint64(&dc.misses, 1)
		return nil, false
	}

	atomic.AddUint64(&dc.hits, 1)
	return entry.Data, true
}

// Set stores a document in cache
func (dc *DocumentCache) Set(key string, data []byte) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Evict if cache is full (simple FIFO, not true LRU)
	if len(dc.cache) >= dc.maxSize {
		// Remove first entry (simple eviction)
		for k := range dc.cache {
			delete(dc.cache, k)
			break
		}
	}

	dc.cache[key] = &CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Unix() + dc.ttl,
	}
}

// Delete removes a document from cache
func (dc *DocumentCache) Delete(key string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	delete(dc.cache, key)
}

// Clear removes all entries from cache
func (dc *DocumentCache) Clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.cache = make(map[string]*CacheEntry, dc.maxSize)
}

// Stats returns cache statistics
func (dc *DocumentCache) Stats() (hits, misses uint64, size int) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.hits, dc.misses, len(dc.cache)
}

// cleanup periodically removes expired entries until Close() stops it.
func (dc *DocumentCache) cleanup() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dc.stopCh:
			return
		case <-ticker.C:
			dc.mu.Lock()
			now := time.Now().Unix()
			for key, entry := range dc.cache {
				if now > entry.ExpiresAt {
					delete(dc.cache, key)
				}
			}
			dc.mu.Unlock()
		}
	}
}

// BuildCacheKey builds a cache key for a document
func BuildCacheKey(collection, key, lang string) string {
	return collection + "|" + key + "|" + lang
}
