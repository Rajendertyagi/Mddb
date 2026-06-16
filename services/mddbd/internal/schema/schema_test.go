package schema

import (
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestSchemaManager(t *testing.T) (*SchemaManager, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "schema_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	sm := NewSchemaManager(db)
	if err := sm.EnsureBucket(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return sm, cleanup
}

func TestSchemaManagerSetGetDelete(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"required":["author"],"properties":{"author":{"type":"string"}}}`

	// Set
	if err := sm.Set("blog", schema); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	raw, found := sm.Get("blog")
	if !found {
		t.Fatal("expected schema to be found")
	}
	if raw != schema {
		t.Errorf("expected %q, got %q", schema, raw)
	}

	// List
	list := sm.List()
	if len(list) != 1 {
		t.Errorf("expected 1 schema, got %d", len(list))
	}

	// Delete
	if err := sm.Delete("blog"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, found = sm.Get("blog")
	if found {
		t.Error("expected schema to be deleted")
	}
}

func TestSchemaValidateNoSchema(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	// No schema set = validation passes (opt-in)
	err := sm.Validate("blog", map[string][]string{"anything": {"goes"}})
	if err != nil {
		t.Errorf("expected nil error for collection without schema, got: %v", err)
	}
}

func TestSchemaValidateRequired(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"required":["author","status"]}`
	if err := sm.Set("blog", schema); err != nil {
		t.Fatal(err)
	}

	// Missing both
	err := sm.Validate("blog", map[string][]string{})
	if err == nil {
		t.Error("expected error for missing required fields")
	}

	// Missing one
	err = sm.Validate("blog", map[string][]string{"author": {"John"}})
	if err == nil {
		t.Error("expected error for missing status")
	}

	// All present
	err = sm.Validate("blog", map[string][]string{"author": {"John"}, "status": {"draft"}})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSchemaValidateEnum(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"properties":{"status":{"enum":["draft","published","archived"]}}}`
	if err := sm.Set("blog", schema); err != nil {
		t.Fatal(err)
	}

	// Valid
	err := sm.Validate("blog", map[string][]string{"status": {"draft"}})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid
	err = sm.Validate("blog", map[string][]string{"status": {"deleted"}})
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestSchemaValidatePattern(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"properties":{"email":{"pattern":"^[a-z]+@[a-z]+\\.[a-z]+$"}}}`
	if err := sm.Set("users", schema); err != nil {
		t.Fatal(err)
	}

	// Valid
	err := sm.Validate("users", map[string][]string{"email": {"john@example.com"}})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid
	err = sm.Validate("users", map[string][]string{"email": {"not-an-email"}})
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestSchemaValidateTypes(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"properties":{"priority":{"type":"integer"},"score":{"type":"number"},"active":{"type":"boolean"}}}`
	if err := sm.Set("items", schema); err != nil {
		t.Fatal(err)
	}

	// Valid
	err := sm.Validate("items", map[string][]string{
		"priority": {"5"},
		"score":    {"3.14"},
		"active":   {"true"},
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Invalid integer
	err = sm.Validate("items", map[string][]string{"priority": {"abc"}})
	if err == nil {
		t.Error("expected error for non-integer")
	}

	// Invalid number
	err = sm.Validate("items", map[string][]string{"score": {"xyz"}})
	if err == nil {
		t.Error("expected error for non-number")
	}

	// Invalid boolean
	err = sm.Validate("items", map[string][]string{"active": {"yes"}})
	if err == nil {
		t.Error("expected error for non-boolean")
	}
}

func TestSchemaValidateMinMaxItems(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	schema := `{"properties":{"tags":{"minItems":1,"maxItems":3}}}`
	if err := sm.Set("blog", schema); err != nil {
		t.Fatal(err)
	}

	// Too few
	err := sm.Validate("blog", map[string][]string{"tags": {}})
	if err == nil {
		t.Error("expected error for too few items")
	}

	// Too many
	err = sm.Validate("blog", map[string][]string{"tags": {"a", "b", "c", "d"}})
	if err == nil {
		t.Error("expected error for too many items")
	}

	// Valid
	err = sm.Validate("blog", map[string][]string{"tags": {"a", "b"}})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSchemaValidateInvalidSchema(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	// Invalid JSON
	err := sm.Set("blog", "not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Invalid pattern
	err = sm.Set("blog", `{"properties":{"x":{"pattern":"[invalid"}}}`)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}

	// Invalid type
	err = sm.Set("blog", `{"properties":{"x":{"type":"array"}}}`)
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestSchemaLoadAll(t *testing.T) {
	f, err := os.CreateTemp("", "schema_load_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create and populate
	sm := NewSchemaManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := sm.Set("blog", `{"required":["title"]}`); err != nil {
		t.Fatal(err)
	}
	if err := sm.Set("docs", `{"required":["version"]}`); err != nil {
		t.Fatal(err)
	}

	// Create new manager and load from DB
	sm2 := NewSchemaManager(db)
	if err := sm2.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	list := sm2.List()
	if len(list) != 2 {
		t.Errorf("expected 2 schemas loaded, got %d", len(list))
	}

	// Validate works after reload
	err = sm2.Validate("blog", map[string][]string{})
	if err == nil {
		t.Error("expected validation error after reload")
	}

	_ = db.Close()
}

func BenchmarkSchemaValidate(b *testing.B) {
	f, err := os.CreateTemp("", "schema_bench_*.db")
	if err != nil {
		b.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	sm := NewSchemaManager(db)
	if err := sm.EnsureBucket(); err != nil {
		b.Fatal(err)
	}
	if err := sm.Set("blog", `{"required":["author","status"],"properties":{"status":{"enum":["draft","published"]},"priority":{"type":"integer"}}}`); err != nil {
		b.Fatal(err)
	}

	meta := map[string][]string{
		"author":   {"John"},
		"status":   {"draft"},
		"priority": {"5"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.Validate("blog", meta)
	}
}
