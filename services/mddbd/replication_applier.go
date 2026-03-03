package main

import (
	"log"

	bolt "go.etcd.io/bbolt"
)

// ReplicationApplier applies binlog entries from the leader to the local BoltDB and in-memory state.
type ReplicationApplier struct {
	server      *Server
	lastApplied uint64
}

// NewReplicationApplier creates a new replication applier
func NewReplicationApplier(s *Server) *ReplicationApplier {
	return &ReplicationApplier{server: s}
}

// Apply applies a single binlog entry to the local database.
func (ra *ReplicationApplier) Apply(entry *BinlogEntry) error {
	err := ra.server.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(entry.BucketName))
		if err != nil {
			return err
		}

		switch entry.Type {
		case BinlogPut:
			return bucket.Put(entry.Key, entry.Value)
		case BinlogDelete:
			return bucket.Delete(entry.Key)
		case BinlogDeleteBucket:
			return tx.DeleteBucket([]byte(entry.BucketName))
		case BinlogCheckpoint:
			// No-op, just a marker
			return nil
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Update in-memory state based on bucket type
	ra.updateInMemoryState(entry)
	ra.lastApplied = entry.LSN

	return nil
}

// ApplyBatch applies multiple binlog entries in a single BoltDB transaction for efficiency.
func (ra *ReplicationApplier) ApplyBatch(entries []*BinlogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	err := ra.server.DB.Update(func(tx *bolt.Tx) error {
		for _, entry := range entries {
			if entry.Type == BinlogDeleteBucket {
				if err := tx.DeleteBucket([]byte(entry.BucketName)); err != nil {
					log.Printf("Replication applier: failed to delete bucket %s: %v", entry.BucketName, err)
				}
				continue
			}
			if entry.Type == BinlogCheckpoint {
				continue
			}

			bucket, err := tx.CreateBucketIfNotExists([]byte(entry.BucketName))
			if err != nil {
				return err
			}

			switch entry.Type {
			case BinlogPut:
				if err := bucket.Put(entry.Key, entry.Value); err != nil {
					return err
				}
			case BinlogDelete:
				if err := bucket.Delete(entry.Key); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Update in-memory state for each entry
	for _, entry := range entries {
		ra.updateInMemoryState(entry)
	}

	ra.lastApplied = entries[len(entries)-1].LSN
	return nil
}

// LastAppliedLSN returns the last applied LSN
func (ra *ReplicationApplier) LastAppliedLSN() uint64 {
	return ra.lastApplied
}

// updateInMemoryState updates in-memory caches and indices based on the replicated bucket
func (ra *ReplicationApplier) updateInMemoryState(entry *BinlogEntry) {
	switch entry.BucketName {
	case "vectors":
		ra.applyVector(entry)
	case "docs":
		ra.invalidateDocCache(entry)
	case "webhooks":
		// Reload all webhooks from DB (simple approach)
		if ra.server.WebhookManager != nil {
			_ = ra.server.WebhookManager.LoadAll()
		}
	case "schemas":
		// Reload all schemas from DB
		if ra.server.SchemaManager != nil {
			_ = ra.server.SchemaManager.LoadAll()
		}
	}
}

// applyVector updates the in-memory vector index
func (ra *ReplicationApplier) applyVector(entry *BinlogEntry) {
	if ra.server.VectorIndex == nil {
		return
	}

	// Parse key: vec|collection|docID
	parts := splitKey(entry.Key)
	if len(parts) < 3 {
		return
	}
	collection := parts[1]
	docID := parts[2]

	switch entry.Type {
	case BinlogPut:
		rec, err := unmarshalEmbeddingRecord(entry.Value)
		if err != nil {
			log.Printf("Replication applier: failed to unmarshal embedding: %v", err)
			return
		}
		ra.server.VectorIndex.Add(collection, docID, rec.Vector)
	case BinlogDelete:
		ra.server.VectorIndex.Remove(collection, docID)
	}
}

// invalidateDocCache removes the document from caches
func (ra *ReplicationApplier) invalidateDocCache(entry *BinlogEntry) {
	// Parse key: doc|collection|docID
	parts := splitKey(entry.Key)
	if len(parts) < 3 {
		return
	}
	collection := parts[1]
	docID := parts[2]

	cacheKey := collection + "|" + docID
	if ra.server.Cache != nil {
		ra.server.Cache.Delete(cacheKey)
	}
	if ra.server.LockFreeCache != nil {
		ra.server.LockFreeCache.Delete(cacheKey)
	}
}
