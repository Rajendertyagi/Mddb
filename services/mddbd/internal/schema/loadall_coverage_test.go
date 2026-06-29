package schema

import (
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestSchemaLoadAllNilBucketAndCorrupt covers LoadAll's nil-bucket short-circuit
// (no EnsureBucket) and the corrupt-entry parse-error branch.
func TestSchemaLoadAllNilBucketAndCorrupt(t *testing.T) {
	db := newSchemaTestDB(t)
	sm := NewSchemaManager(db)

	// No bucket yet -> LoadAll returns nil without scanning.
	if err := sm.LoadAll(); err != nil {
		t.Errorf("LoadAll without bucket = %v, want nil", err)
	}

	// Corrupt schema bytes -> parseSchema fails and LoadAll surfaces the error.
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSchemas).Put([]byte("schema|c"), []byte("{not valid json"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := sm.LoadAll(); err == nil {
		t.Error("LoadAll should surface a parse error for a corrupt schema entry")
	}
}
