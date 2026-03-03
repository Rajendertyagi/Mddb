package main

import (
	"os"
	"testing"
)

// Tests for embedding.go: NewEmbeddingProvider, envDefault, envDefaultInt

func TestNewEmbeddingProvider_Empty(t *testing.T) {
	_ = os.Unsetenv("MDDB_EMBEDDING_PROVIDER")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil for empty provider env")
	}
}

func TestNewEmbeddingProvider_None(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "none")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil for provider=none")
	}
}

func TestNewEmbeddingProvider_Unknown(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "unknown-provider")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil for unknown provider")
	}
}

func TestNewEmbeddingProvider_OpenAI_NoKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "openai")
	_ = os.Unsetenv("MDDB_EMBEDDING_API_KEY")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil when openai key not set")
	}
}

func TestNewEmbeddingProvider_OpenAI_WithKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "openai")
	t.Setenv("MDDB_EMBEDDING_API_KEY", "sk-test-key")
	_ = os.Unsetenv("MDDB_EMBEDDING_API_URL")
	_ = os.Unsetenv("MDDB_EMBEDDING_MODEL")
	_ = os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")
	p := NewEmbeddingProvider()
	if p == nil {
		t.Fatal("expected non-nil provider for openai with key")
	}
	if p.Model() != "text-embedding-3-small" {
		t.Errorf("Model = %q, want default", p.Model())
	}
	if p.Dimensions() != 1536 {
		t.Errorf("Dimensions = %d, want 1536", p.Dimensions())
	}
}

func TestNewEmbeddingProvider_OpenAI_CustomSettings(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "openai")
	t.Setenv("MDDB_EMBEDDING_API_KEY", "sk-test-key")
	t.Setenv("MDDB_EMBEDDING_API_URL", "https://custom.api.com/v1")
	t.Setenv("MDDB_EMBEDDING_MODEL", "custom-model")
	t.Setenv("MDDB_EMBEDDING_DIMENSIONS", "3072")
	p := NewEmbeddingProvider()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Model() != "custom-model" {
		t.Errorf("Model = %q, want custom-model", p.Model())
	}
	if p.Dimensions() != 3072 {
		t.Errorf("Dimensions = %d, want 3072", p.Dimensions())
	}
}

func TestNewEmbeddingProvider_Ollama(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "ollama")
	os.Unsetenv("MDDB_EMBEDDING_API_URL")
	os.Unsetenv("MDDB_EMBEDDING_MODEL")
	os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")
	p := NewEmbeddingProvider()
	if p == nil {
		t.Fatal("expected non-nil provider for ollama")
	}
	if p.Model() != "nomic-embed-text" {
		t.Errorf("Model = %q, want default", p.Model())
	}
	if p.Dimensions() != 768 {
		t.Errorf("Dimensions = %d, want 768", p.Dimensions())
	}
}

func TestNewEmbeddingProvider_Voyage_NoKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "voyage")
	os.Unsetenv("MDDB_EMBEDDING_API_KEY")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil when voyage key not set")
	}
}

func TestNewEmbeddingProvider_Voyage_WithKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "voyage")
	t.Setenv("MDDB_EMBEDDING_API_KEY", "vo-test")
	os.Unsetenv("MDDB_EMBEDDING_API_URL")
	os.Unsetenv("MDDB_EMBEDDING_MODEL")
	os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")
	p := NewEmbeddingProvider()
	if p == nil {
		t.Fatal("expected non-nil provider for voyage with key")
	}
	if p.Model() != "voyage-3" {
		t.Errorf("Model = %q, want voyage-3", p.Model())
	}
	if p.Dimensions() != 1024 {
		t.Errorf("Dimensions = %d, want 1024", p.Dimensions())
	}
}

func TestNewEmbeddingProvider_Cohere_NoKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "cohere")
	os.Unsetenv("MDDB_EMBEDDING_API_KEY")
	p := NewEmbeddingProvider()
	if p != nil {
		t.Error("expected nil when cohere key not set")
	}
}

func TestNewEmbeddingProvider_Cohere_WithKey(t *testing.T) {
	t.Setenv("MDDB_EMBEDDING_PROVIDER", "cohere")
	t.Setenv("MDDB_EMBEDDING_API_KEY", "co-test")
	os.Unsetenv("MDDB_EMBEDDING_API_URL")
	os.Unsetenv("MDDB_EMBEDDING_MODEL")
	os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")
	p := NewEmbeddingProvider()
	if p == nil {
		t.Fatal("expected non-nil provider for cohere with key")
	}
	if p.Model() != "embed-english-v3.0" {
		t.Errorf("Model = %q, want embed-english-v3.0", p.Model())
	}
	if p.Dimensions() != 1024 {
		t.Errorf("Dimensions = %d, want 1024", p.Dimensions())
	}
}

// ---- envDefault and envDefaultInt ----

func TestEnvDefault_Set(t *testing.T) {
	t.Setenv("MDDB_TEST_KEY", "custom-value")
	v := envDefault("MDDB_TEST_KEY", "default-value")
	if v != "custom-value" {
		t.Errorf("got %q, want custom-value", v)
	}
}

func TestEnvDefault_Unset(t *testing.T) {
	os.Unsetenv("MDDB_TEST_KEY_UNSET")
	v := envDefault("MDDB_TEST_KEY_UNSET", "default-value")
	if v != "default-value" {
		t.Errorf("got %q, want default-value", v)
	}
}

func TestEnvDefaultInt_Set(t *testing.T) {
	t.Setenv("MDDB_TEST_INT", "42")
	v := envDefaultInt("MDDB_TEST_INT", 100)
	if v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

func TestEnvDefaultInt_Unset(t *testing.T) {
	os.Unsetenv("MDDB_TEST_INT_UNSET")
	v := envDefaultInt("MDDB_TEST_INT_UNSET", 100)
	if v != 100 {
		t.Errorf("got %d, want 100", v)
	}
}

func TestEnvDefaultInt_Invalid(t *testing.T) {
	t.Setenv("MDDB_TEST_INT_BAD", "not-a-number")
	v := envDefaultInt("MDDB_TEST_INT_BAD", 200)
	if v != 200 {
		t.Errorf("got %d, want 200 (fallback for invalid)", v)
	}
}
