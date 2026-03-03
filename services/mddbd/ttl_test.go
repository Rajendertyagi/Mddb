package main

import (
	"encoding/binary"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestTTLManager(t *testing.T) (*TTLManager, *bolt.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "ttl_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	// Create required docs bucket for cleanup tests
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("docs"))
		return err
	})
	if err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	server := &Server{DB: db}
	mgr := NewTTLManager(db, server)
	if err := mgr.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return mgr, db, cleanup
}

func TestNewTTLManager(t *testing.T) {
	mgr, _, cleanup := newTestTTLManager(t)
	defer cleanup()

	if mgr.db == nil {
		t.Error("expected non-nil db")
	}
	if mgr.server == nil {
		t.Error("expected non-nil server")
	}
	if mgr.stopCh == nil {
		t.Error("expected non-nil stopCh")
	}
}

func TestTTLEnsureBuckets(t *testing.T) {
	f, err := os.CreateTemp("", "ttl_buckets_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mgr := NewTTLManager(db, &Server{DB: db})

	if err := mgr.EnsureBuckets(); err != nil {
		t.Fatalf("EnsureBuckets failed: %v", err)
	}

	// Idempotent
	if err := mgr.EnsureBuckets(); err != nil {
		t.Fatalf("second EnsureBuckets failed: %v", err)
	}

	// Verify buckets
	err = db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(bucketTTL) == nil {
			t.Error("ttl bucket not created")
		}
		if tx.Bucket(bucketTTLRev) == nil {
			t.Error("ttlrev bucket not created")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLSetAndVerify(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	expiresAt := time.Now().Unix() + 3600 // 1 hour from now
	if err := mgr.Set("blog", "doc1", expiresAt); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify forward key exists
	err := db.View(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		fwdKey := ttlKey(expiresAt, "blog", "doc1")
		if bTTL.Get(fwdKey) == nil {
			t.Error("expected forward TTL key to exist")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify reverse key exists
	err = db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		revKey := ttlRevKey("blog", "doc1")
		v := bRev.Get(revKey)
		if v == nil {
			t.Error("expected reverse TTL key to exist")
		}
		if len(v) != 8 {
			t.Errorf("expected 8-byte value, got %d", len(v))
		}
		stored := int64(binary.BigEndian.Uint64(v))
		if stored != expiresAt {
			t.Errorf("expected expiresAt %d, got %d", expiresAt, stored)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLSetUpdatesTTL(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	oldExpiry := time.Now().Unix() + 3600
	newExpiry := time.Now().Unix() + 7200

	// Set initial TTL
	if err := mgr.Set("blog", "doc1", oldExpiry); err != nil {
		t.Fatalf("Set oldExpiry: %v", err)
	}

	// Update TTL
	if err := mgr.Set("blog", "doc1", newExpiry); err != nil {
		t.Fatalf("Set newExpiry: %v", err)
	}

	// Old forward key should be removed
	err := db.View(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		oldKey := ttlKey(oldExpiry, "blog", "doc1")
		if bTTL.Get(oldKey) != nil {
			t.Error("old forward TTL key should be removed")
		}

		newKey := ttlKey(newExpiry, "blog", "doc1")
		if bTTL.Get(newKey) == nil {
			t.Error("new forward TTL key should exist")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reverse key should point to new expiry
	err = db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		v := bRev.Get(ttlRevKey("blog", "doc1"))
		stored := int64(binary.BigEndian.Uint64(v))
		if stored != newExpiry {
			t.Errorf("reverse key should point to new expiry %d, got %d", newExpiry, stored)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLSetZeroRemovesTTL(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	expiresAt := time.Now().Unix() + 3600
	if err := mgr.Set("blog", "doc1", expiresAt); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Set with 0 should remove TTL
	if err := mgr.Set("blog", "doc1", 0); err != nil {
		t.Fatalf("Set with 0 failed: %v", err)
	}

	// Forward key should be removed
	err := db.View(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		fwdKey := ttlKey(expiresAt, "blog", "doc1")
		if bTTL.Get(fwdKey) != nil {
			t.Error("forward TTL key should be removed when expiresAt=0")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reverse key should be removed
	err = db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("blog", "doc1")) != nil {
			t.Error("reverse TTL key should be removed when expiresAt=0")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLSetNegativeRemovesTTL(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	expiresAt := time.Now().Unix() + 3600
	if err := mgr.Set("blog", "doc1", expiresAt); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Negative expiresAt should also remove TTL
	if err := mgr.Set("blog", "doc1", -1); err != nil {
		t.Fatalf("Set negative: %v", err)
	}

	err := db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("blog", "doc1")) != nil {
			t.Error("reverse key should be removed for negative expiresAt")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLRemove(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	expiresAt := time.Now().Unix() + 3600
	if err := mgr.Set("blog", "doc1", expiresAt); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := mgr.Remove("blog", "doc1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Both keys should be removed
	err := db.View(func(tx *bolt.Tx) error {
		bTTL := tx.Bucket(bucketTTL)
		fwdKey := ttlKey(expiresAt, "blog", "doc1")
		if bTTL.Get(fwdKey) != nil {
			t.Error("forward key should be removed")
		}

		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("blog", "doc1")) != nil {
			t.Error("reverse key should be removed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLRemoveNonexistent(t *testing.T) {
	mgr, _, cleanup := newTestTTLManager(t)
	defer cleanup()

	// Removing a non-existent TTL should not error
	if err := mgr.Remove("blog", "nonexistent"); err != nil {
		t.Fatalf("Remove of nonexistent TTL failed: %v", err)
	}
}

func TestTTLRemoveMultiple(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	exp1 := time.Now().Unix() + 3600
	exp2 := time.Now().Unix() + 7200

	if err := mgr.Set("blog", "doc1", exp1); err != nil {
		t.Fatalf("Set doc1: %v", err)
	}
	if err := mgr.Set("blog", "doc2", exp2); err != nil {
		t.Fatalf("Set doc2: %v", err)
	}

	// Remove only doc1
	if err := mgr.Remove("blog", "doc1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// doc1 should be gone, doc2 should remain
	err := db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("blog", "doc1")) != nil {
			t.Error("doc1 reverse key should be removed")
		}
		if bRev.Get(ttlRevKey("blog", "doc2")) == nil {
			t.Error("doc2 reverse key should still exist")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLKeyFormat(t *testing.T) {
	key := ttlKey(1234567890, "blog", "doc1")
	expected := "00000000001234567890|blog|doc1"
	if string(key) != expected {
		t.Errorf("expected %q, got %q", expected, string(key))
	}
}

func TestTTLKeyZeroPadding(t *testing.T) {
	key := ttlKey(1, "col", "id")
	s := string(key)
	// Should be zero-padded to 20 digits
	if len(s) < 20 {
		t.Errorf("expected zero-padded key, got %q", s)
	}
	expected := "00000000000000000001|col|id"
	if s != expected {
		t.Errorf("expected %q, got %q", expected, s)
	}
}

func TestTTLRevKeyFormat(t *testing.T) {
	key := ttlRevKey("blog", "doc1")
	expected := "blog|doc1"
	if string(key) != expected {
		t.Errorf("expected %q, got %q", expected, string(key))
	}
}

func TestTTLSetBinlog(t *testing.T) {
	mgr, _, cleanup := newTestTTLManager(t)
	defer cleanup()

	mgr.SetBinlog(nil)
	if mgr.binlog != nil {
		t.Error("expected nil binlog")
	}
}

func TestTTLStartAndStop(t *testing.T) {
	mgr, _, cleanup := newTestTTLManager(t)
	defer cleanup()

	// StartCleanup should not panic
	mgr.StartCleanup(100 * time.Millisecond)

	// Give it a moment to run
	time.Sleep(250 * time.Millisecond)

	// Stop should not panic
	mgr.Stop()
}

func TestTTLSetMultipleCollections(t *testing.T) {
	mgr, db, cleanup := newTestTTLManager(t)
	defer cleanup()

	exp := time.Now().Unix() + 3600

	if err := mgr.Set("blog", "doc1", exp); err != nil {
		t.Fatalf("Set blog/doc1: %v", err)
	}
	if err := mgr.Set("docs", "doc1", exp); err != nil {
		t.Fatalf("Set docs/doc1: %v", err)
	}
	if err := mgr.Set("blog", "doc2", exp+100); err != nil {
		t.Fatalf("Set blog/doc2: %v", err)
	}

	// Verify all three entries exist
	err := db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("blog", "doc1")) == nil {
			t.Error("blog|doc1 should exist")
		}
		if bRev.Get(ttlRevKey("docs", "doc1")) == nil {
			t.Error("docs|doc1 should exist")
		}
		if bRev.Get(ttlRevKey("blog", "doc2")) == nil {
			t.Error("blog|doc2 should exist")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTTLSetSameDocDifferentCollection(t *testing.T) {
	mgr, _, cleanup := newTestTTLManager(t)
	defer cleanup()

	exp1 := time.Now().Unix() + 3600
	exp2 := time.Now().Unix() + 7200

	// Same docID in different collections
	if err := mgr.Set("blog", "doc1", exp1); err != nil {
		t.Fatalf("Set blog/doc1: %v", err)
	}
	if err := mgr.Set("docs", "doc1", exp2); err != nil {
		t.Fatalf("Set docs/doc1: %v", err)
	}

	// Remove from one should not affect the other
	if err := mgr.Remove("blog", "doc1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// docs|doc1 should still exist
	if err := mgr.db.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(bucketTTLRev)
		if bRev.Get(ttlRevKey("docs", "doc1")) == nil {
			t.Error("docs|doc1 should still exist after removing blog|doc1")
		}
		return nil
	}); err != nil {
		t.Fatalf("db.View: %v", err)
	}
}

func TestTTLKeyOrdering(t *testing.T) {
	// Keys should be ordered by timestamp for cursor scan
	k1 := ttlKey(100, "col", "doc1")
	k2 := ttlKey(200, "col", "doc2")
	k3 := ttlKey(300, "col", "doc3")

	if string(k1) >= string(k2) {
		t.Error("key with lower timestamp should sort before higher")
	}
	if string(k2) >= string(k3) {
		t.Error("key ordering is incorrect")
	}
}
