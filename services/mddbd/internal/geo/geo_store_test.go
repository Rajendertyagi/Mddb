package geo

import (
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func openGeoTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "geo_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	})
	return db
}

func TestGeoStoreRoundTrip(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	if err := gs.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	if err := gs.Put("venues", "doc1", 52.52, 13.405); err != nil {
		t.Fatal(err)
	}

	lat, lng, ok, err := gs.Get("venues", "doc1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected doc1 to exist")
	}
	if lat != 52.52 || lng != 13.405 {
		t.Errorf("got (%v, %v), want (52.52, 13.405)", lat, lng)
	}
}

func TestGeoStoreDelete(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	_ = gs.Put("c", "a", 1, 2)

	if err := gs.Delete("c", "a"); err != nil {
		t.Fatal(err)
	}
	_, _, ok, _ := gs.Get("c", "a")
	if ok {
		t.Error("deleted doc should not be found")
	}
}

func TestGeoStoreDeleteCollection(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	_ = gs.Put("c1", "a", 1, 2)
	_ = gs.Put("c1", "b", 3, 4)
	_ = gs.Put("c2", "a", 5, 6)

	if err := gs.DeleteCollection("c1"); err != nil {
		t.Fatal(err)
	}
	// c1 gone.
	if _, _, ok, _ := gs.Get("c1", "a"); ok {
		t.Error("c1/a should be gone")
	}
	if _, _, ok, _ := gs.Get("c1", "b"); ok {
		t.Error("c1/b should be gone")
	}
	// c2 untouched.
	if _, _, ok, _ := gs.Get("c2", "a"); !ok {
		t.Error("c2/a should survive")
	}
}

func TestGeoStoreRebuild(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	for i, p := range [][3]float64{{52.52, 13.405, 1}, {48.857, 2.352, 2}, {51.507, -0.128, 3}} {
		id := string(rune('a' + i))
		if err := gs.Put("venues", id, p[0], p[1]); err != nil {
			t.Fatal(err)
		}
	}

	idx := NewGeoIndex()
	n, err := gs.Rebuild(idx, "")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rebuilt %d, want 3", n)
	}
	if idx.Len("venues") != 3 {
		t.Errorf("index Len = %d, want 3", idx.Len("venues"))
	}

	// Rebuild specific collection only clears and repopulates that one.
	gs2 := NewGeoStore(db)
	_ = gs2.Put("other", "x", 0, 0)
	_, err = gs2.Rebuild(idx, "venues")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Len("venues") != 3 {
		t.Errorf("per-collection rebuild lost venues: %d", idx.Len("venues"))
	}
}

func TestGeoStoreKeyRoundTrip(t *testing.T) {
	key := buildGeoKey("venues", "doc#1")
	coll, docID, ok := parseGeoKey(key)
	if !ok {
		t.Fatal("parseGeoKey failed")
	}
	if coll != "venues" || docID != "doc#1" {
		t.Errorf("got (%q, %q), want (venues, doc#1)", coll, docID)
	}
}

func TestGeoStoreEncodeDecode(t *testing.T) {
	tests := []struct{ lat, lng float64 }{
		{52.52, 13.405},
		{-90, -180},
		{90, 180},
		{0, 0},
	}
	for _, tt := range tests {
		v := encodeGeoValue(tt.lat, tt.lng)
		if len(v) != 16 {
			t.Errorf("encoded length %d, want 16", len(v))
		}
		lat, lng, ok := decodeGeoValue(v)
		if !ok || lat != tt.lat || lng != tt.lng {
			t.Errorf("roundtrip failed: got (%v, %v, %v)", lat, lng, ok)
		}
	}
	// Bad length.
	if _, _, ok := decodeGeoValue([]byte{1, 2, 3}); ok {
		t.Error("short value should fail to decode")
	}
}

func TestGeoStoreParseGeoKeyInvalid(t *testing.T) {
	if _, _, ok := parseGeoKey([]byte("not-a-geo-key")); ok {
		t.Error("non-prefixed key should fail")
	}
	if _, _, ok := parseGeoKey([]byte("geo|no-sep")); ok {
		t.Error("missing second | should fail")
	}
}

// SetBinlog must be a no-op when called with nil and still let Put/Delete
// succeed. Covered here because main_test.go doesn't exercise this branch.
func TestGeoStoreSetBinlogNil(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	gs.SetBinlog(nil)
	if err := gs.Put("c", "a", 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := gs.Delete("c", "a"); err != nil {
		t.Fatal(err)
	}
}

// RebuildHash scans the "geo" bucket and feeds every persisted point
// into a GeoHashIndex. Mirrors TestGeoStoreRebuild but for the alternative
// index type.
func TestGeoStoreRebuildHash(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	_ = gs.Put("v", "a", 52.52, 13.405)
	_ = gs.Put("v", "b", 48.857, 2.352)
	_ = gs.Put("w", "c", 51.507, -0.128)

	idx := NewGeoHashIndex()
	n, err := gs.RebuildHash(idx, "")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("RebuildHash all: n=%d, want 3", n)
	}
	if idx.Len("v") != 2 {
		t.Errorf("v len=%d, want 2", idx.Len("v"))
	}
	if idx.Len("w") != 1 {
		t.Errorf("w len=%d, want 1", idx.Len("w"))
	}

	// Per-collection rebuild clears and repopulates only that collection.
	idx2 := NewGeoHashIndex()
	idx2.Add("v", "stale", 0, 0)
	idx2.Add("w", "c", 51.507, -0.128) // should survive
	n2, err := gs.RebuildHash(idx2, "v")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 2 {
		t.Errorf("RebuildHash v: n=%d, want 2", n2)
	}
	if idx2.Len("v") != 2 {
		t.Errorf("after per-collection rebuild v len=%d, want 2", idx2.Len("v"))
	}
	if idx2.Len("w") != 1 {
		t.Errorf("per-collection rebuild clobbered w: len=%d, want 1", idx2.Len("w"))
	}
}

// RebuildHash with nil index must error rather than panic.
func TestGeoStoreRebuildHashNilIndex(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	if _, err := gs.RebuildHash(nil, ""); err == nil {
		t.Error("expected error for nil index")
	}
}

// Delete on a missing key and DeleteCollection on an empty collection
// should both be no-ops, not errors.
func TestGeoStoreDeleteMissing(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	if err := gs.Delete("nope", "nope"); err != nil {
		t.Errorf("Delete missing should not error, got %v", err)
	}
	if err := gs.DeleteCollection("empty"); err != nil {
		t.Errorf("DeleteCollection empty should not error, got %v", err)
	}
}

// Rebuild with nil index must error.
func TestGeoStoreRebuildNilIndex(t *testing.T) {
	db := openGeoTestDB(t)
	gs := NewGeoStore(db)
	_ = gs.EnsureBucket()
	if _, err := gs.Rebuild(nil, ""); err == nil {
		t.Error("expected error for nil index")
	}
}
