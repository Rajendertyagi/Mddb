package ttl

import (
	"encoding/json"
	"errors"
	"fmt"
	"mddb/internal/storage"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// dbReaper is a functional Reaper backed by the test's BoltDB: it deserializes
// docs as plain JSON and deletes them from the docs bucket, so cleanup() can be
// exercised end-to-end without the Server god-object.
type dbReaper struct {
	db     *bolt.DB
	delErr error // when set, DeleteDocument fails (to test the error branch)
}

func (r *dbReaper) LoadDoc(v []byte) (*storage.Doc, error) {
	var d storage.Doc
	if err := json.Unmarshal(v, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *dbReaper) GenID(collection, key, lang string) string {
	return collection + "|" + key + "|" + lang
}

func (r *dbReaper) DeleteDocument(collection, key, lang string) error {
	if r.delErr != nil {
		return r.delErr
	}
	docID := r.GenID(collection, key, lang)
	return r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("docs")).Delete(storage.DocKey(collection, docID))
	})
}

func newReapTTL(t *testing.T) (*TTLManager, *dbReaper, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "ttl_reap_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("docs"))
		return err
	}); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	r := &dbReaper{db: db}
	mgr := NewTTLManager(db, r)
	if err := mgr.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return mgr, r, cleanup
}

// putDoc stores a JSON document and returns the docID used as its key.
func putDoc(t *testing.T, db *bolt.DB, r *dbReaper, collection, key, lang string) string {
	t.Helper()
	docID := r.GenID(collection, key, lang)
	doc := storage.Doc{ID: docID, Key: key, Lang: lang, ContentMD: "body"}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("docs")).Put(storage.DocKey(collection, docID), data)
	}); err != nil {
		t.Fatal(err)
	}
	return docID
}

func docExists(db *bolt.DB, collection, docID string) bool {
	exists := false
	_ = db.View(func(tx *bolt.Tx) error {
		exists = tx.Bucket([]byte("docs")).Get(storage.DocKey(collection, docID)) != nil
		return nil
	})
	return exists
}

func TestCleanup_DeletesExpired(t *testing.T) {
	mgr, r, done := newReapTTL(t)
	defer done()

	docID := putDoc(t, mgr.db, r, "blog", "expired", "en")
	if err := mgr.Set("blog", docID, time.Now().Unix()-100); err != nil {
		t.Fatal(err)
	}

	mgr.cleanup()

	if docExists(mgr.db, "blog", docID) {
		t.Error("expired document should have been deleted by cleanup")
	}
	// TTL reverse entry must be gone too.
	_ = mgr.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(bucketTTLRev).Get(ttlRevKey("blog", docID)) != nil {
			t.Error("expired TTL reverse entry should be removed")
		}
		return nil
	})
}

func TestCleanup_KeepsNotExpired(t *testing.T) {
	mgr, r, done := newReapTTL(t)
	defer done()

	docID := putDoc(t, mgr.db, r, "blog", "alive", "en")
	if err := mgr.Set("blog", docID, time.Now().Unix()+3600); err != nil {
		t.Fatal(err)
	}

	mgr.cleanup()

	if !docExists(mgr.db, "blog", docID) {
		t.Error("non-expired document must survive cleanup")
	}
}

func TestCleanup_EmptyBucketNoPanic(t *testing.T) {
	mgr, _, done := newReapTTL(t)
	defer done()
	mgr.cleanup() // nothing to reap
}

func TestCleanup_MalformedKeySkipped(t *testing.T) {
	mgr, _, done := newReapTTL(t)
	defer done()

	// A forward key with only two parts must be skipped, not crash.
	malformed := []byte(fmt.Sprintf("%020d|blogonly", time.Now().Unix()-100))
	if err := mgr.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTTL).Put(malformed, []byte{})
	}); err != nil {
		t.Fatal(err)
	}
	mgr.cleanup()
}

func TestCleanup_DeleteErrorKeepsEntry(t *testing.T) {
	mgr, r, done := newReapTTL(t)
	defer done()

	docID := putDoc(t, mgr.db, r, "blog", "stuck", "en")
	if err := mgr.Set("blog", docID, time.Now().Unix()-100); err != nil {
		t.Fatal(err)
	}
	r.delErr = errors.New("delete failed")

	mgr.cleanup()

	// Delete failed, so the doc and its TTL entry are intentionally kept.
	if !docExists(mgr.db, "blog", docID) {
		t.Error("document should remain when DeleteDocument fails")
	}
	_ = mgr.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(bucketTTLRev).Get(ttlRevKey("blog", docID)) == nil {
			t.Error("TTL entry should remain when delete fails")
		}
		return nil
	})
}
