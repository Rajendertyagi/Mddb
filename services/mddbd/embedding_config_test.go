package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// newTestServerForEmbeddingConfig creates a minimal Server for embedding config tests.
func newTestServerForEmbeddingConfig(t *testing.T) (*Server, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("bolt.Open: %v", err)
	}

	s := &Server{
		DB:   db,
		Path: dbPath,
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		t.Fatalf("ensureBuckets: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(filepath.Dir(dbPath))
	}
	return s, cleanup
}

func TestEmbeddingConfig_JSONMarshal(t *testing.T) {
	config := EmbeddingConfig{
		ID:         "cfg-1",
		Name:       "OpenAI Ada",
		Provider:   "openai",
		Model:      "text-embedding-ada-002",
		Dimensions: 1536,
		APIKey:     "sk-test-key",
		APIURL:     "https://api.openai.com/v1",
		IsDefault:  true,
		CreatedAt:  1700000000,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded EmbeddingConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != config.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, config.ID)
	}
	if decoded.Name != config.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, config.Name)
	}
	if decoded.Provider != config.Provider {
		t.Errorf("Provider = %q, want %q", decoded.Provider, config.Provider)
	}
	if decoded.Model != config.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, config.Model)
	}
	if decoded.Dimensions != config.Dimensions {
		t.Errorf("Dimensions = %d, want %d", decoded.Dimensions, config.Dimensions)
	}
	if decoded.APIKey != config.APIKey {
		t.Errorf("APIKey = %q, want %q", decoded.APIKey, config.APIKey)
	}
	if decoded.IsDefault != config.IsDefault {
		t.Errorf("IsDefault = %v, want %v", decoded.IsDefault, config.IsDefault)
	}
	if decoded.CreatedAt != config.CreatedAt {
		t.Errorf("CreatedAt = %d, want %d", decoded.CreatedAt, config.CreatedAt)
	}
}

func TestEmbeddingConfig_JSONFields(t *testing.T) {
	config := EmbeddingConfig{
		ID:   "test",
		Name: "Test Config",
	}
	data, _ := json.Marshal(config)

	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)

	// Verify JSON field names match expected tags
	expectedFields := []string{"id", "name", "provider", "model", "dimensions", "apiKey", "apiUrl", "isDefault", "createdAt"}
	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("missing JSON field %q", f)
		}
	}
}

func TestSaveAndGetEmbeddingConfig(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:         "cfg-1",
		Name:       "Test Config",
		Provider:   "openai",
		Model:      "text-embedding-ada-002",
		Dimensions: 1536,
		APIKey:     "sk-test",
		IsDefault:  false,
		CreatedAt:  1700000000,
	}

	if err := s.SaveEmbeddingConfig(config); err != nil {
		t.Fatalf("SaveEmbeddingConfig: %v", err)
	}

	got, err := s.GetEmbeddingConfig("cfg-1")
	if err != nil {
		t.Fatalf("GetEmbeddingConfig: %v", err)
	}
	if got.ID != "cfg-1" {
		t.Errorf("ID = %q, want %q", got.ID, "cfg-1")
	}
	if got.Name != "Test Config" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Config")
	}
	if got.Dimensions != 1536 {
		t.Errorf("Dimensions = %d, want 1536", got.Dimensions)
	}
}

func TestGetEmbeddingConfig_NotFound(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_, err := s.GetEmbeddingConfig("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestListEmbeddingConfigs(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	configs := []*EmbeddingConfig{
		{ID: "cfg-1", Name: "Config 1", Provider: "openai", Model: "ada", Dimensions: 1536},
		{ID: "cfg-2", Name: "Config 2", Provider: "ollama", Model: "nomic", Dimensions: 768},
	}

	for _, c := range configs {
		if err := s.SaveEmbeddingConfig(c); err != nil {
			t.Fatalf("SaveEmbeddingConfig: %v", err)
		}
	}

	list, err := s.ListEmbeddingConfigs()
	if err != nil {
		t.Fatalf("ListEmbeddingConfigs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
}

func TestListEmbeddingConfigs_Empty(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	list, err := s.ListEmbeddingConfigs()
	if err != nil {
		t.Fatalf("ListEmbeddingConfigs: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("list len = %d, want 0", len(list))
	}
}

func TestGetDefaultEmbeddingConfig(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	c1 := &EmbeddingConfig{ID: "cfg-1", Name: "Non-default", Provider: "openai", Model: "ada", Dimensions: 1536, IsDefault: false}
	c2 := &EmbeddingConfig{ID: "cfg-2", Name: "Default", Provider: "ollama", Model: "nomic", Dimensions: 768, IsDefault: true}

	_ = s.SaveEmbeddingConfig(c1)
	_ = s.SaveEmbeddingConfig(c2)

	def, err := s.GetDefaultEmbeddingConfig()
	if err != nil {
		t.Fatalf("GetDefaultEmbeddingConfig: %v", err)
	}
	if def.ID != "cfg-2" {
		t.Errorf("default config ID = %q, want %q", def.ID, "cfg-2")
	}
	if !def.IsDefault {
		t.Error("default config IsDefault = false")
	}
}

func TestGetDefaultEmbeddingConfig_NoDefault(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	c := &EmbeddingConfig{ID: "cfg-1", Name: "No default", Provider: "openai", Model: "ada", Dimensions: 1536, IsDefault: false}
	_ = s.SaveEmbeddingConfig(c)

	_, err := s.GetDefaultEmbeddingConfig()
	if err == nil {
		t.Error("expected error when no default config exists")
	}
}

func TestSaveEmbeddingConfig_SetDefault_UnsetsOthers(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	c1 := &EmbeddingConfig{ID: "cfg-1", Name: "First Default", Provider: "openai", Model: "ada", Dimensions: 1536, IsDefault: true}
	c2 := &EmbeddingConfig{ID: "cfg-2", Name: "Second Default", Provider: "ollama", Model: "nomic", Dimensions: 768, IsDefault: true}

	_ = s.SaveEmbeddingConfig(c1)
	_ = s.SaveEmbeddingConfig(c2)

	// c1 should no longer be default
	got1, _ := s.GetEmbeddingConfig("cfg-1")
	if got1.IsDefault {
		t.Error("cfg-1 should no longer be default after cfg-2 was set as default")
	}

	got2, _ := s.GetEmbeddingConfig("cfg-2")
	if !got2.IsDefault {
		t.Error("cfg-2 should be the default")
	}
}

func TestDeleteEmbeddingConfig(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	c := &EmbeddingConfig{ID: "cfg-del", Name: "To Delete", Provider: "openai", Model: "ada", Dimensions: 1536}
	_ = s.SaveEmbeddingConfig(c)

	err := s.DeleteEmbeddingConfig("cfg-del")
	if err != nil {
		t.Fatalf("DeleteEmbeddingConfig: %v", err)
	}

	_, err = s.GetEmbeddingConfig("cfg-del")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestDeleteEmbeddingConfig_NonExistent(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	// Deleting a non-existent key should not error in BoltDB
	err := s.DeleteEmbeddingConfig("nonexistent")
	if err != nil {
		t.Errorf("DeleteEmbeddingConfig: %v (expected nil for non-existent)", err)
	}
}

func TestSaveEmbeddingConfig_UpdateExisting(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	c := &EmbeddingConfig{ID: "cfg-upd", Name: "Original", Provider: "openai", Model: "ada", Dimensions: 1536}
	_ = s.SaveEmbeddingConfig(c)

	c.Name = "Updated"
	c.Dimensions = 3072
	_ = s.SaveEmbeddingConfig(c)

	got, _ := s.GetEmbeddingConfig("cfg-upd")
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated")
	}
	if got.Dimensions != 3072 {
		t.Errorf("Dimensions = %d, want 3072", got.Dimensions)
	}
}

func TestInitializeEmbeddingFromConfig_NilConfig(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	// Should not panic with nil config
	s.InitializeEmbeddingFromConfig(nil)
}

func TestInitializeEmbeddingFromConfig_UnknownProvider(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:       "cfg-unknown",
		Provider: "unknown-provider",
		Model:    "test",
	}
	// Should not panic with unknown provider
	s.InitializeEmbeddingFromConfig(config)

	if s.Embedding != nil {
		t.Error("Embedding should be nil for unknown provider")
	}
}
