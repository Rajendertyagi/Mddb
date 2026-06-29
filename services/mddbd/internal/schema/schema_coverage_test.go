package schema

import (
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"mddb/internal/binlog"
)

func newSchemaTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "schema_cov_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); _ = os.Remove(f.Name()) })
	return db
}

func TestSchemaManagerReloadAndCRUD(t *testing.T) {
	db := newSchemaTestDB(t)
	sm := NewSchemaManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	// Binlog set so Set/Delete exercise their append branches.
	bl, err := binlog.NewBinlog("", binlog.BinlogConfig{Path: filepath.Join(t.TempDir(), "s.binlog")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bl.Close() }()
	sm.SetBinlog(bl)

	if err := sm.Set("col", `{"required":["title"]}`); err != nil {
		t.Fatal(err)
	}
	// Invalid schema JSON -> error branch.
	if err := sm.Set("col", "{not json"); err == nil {
		t.Error("expected error for invalid schema JSON")
	}

	// Reload rebuilds the in-memory cache from BoltDB.
	if err := sm.Reload(db); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, ok := sm.Get("col"); !ok {
		t.Error("expected 'col' schema present after Reload")
	}

	// Delete existing + missing.
	if err := sm.Delete("col"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := sm.Delete("nonexistent"); err != nil {
		t.Fatalf("Delete(missing): %v", err)
	}

	// LoadAll on a fresh manager over the same DB.
	sm2 := NewSchemaManager(db)
	if err := sm2.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}
