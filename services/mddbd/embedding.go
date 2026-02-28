package main

import (
	"context"
	"fmt"
	"log"
	"os"
)

// EmbeddingProvider generates embedding vectors from text.
type EmbeddingProvider interface {
	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embedding vectors for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Model returns the model name used for embeddings.
	Model() string

	// Dimensions returns the dimensionality of the embedding vectors.
	Dimensions() int
}

// NewEmbeddingProvider creates an embedding provider based on configuration.
// Returns nil if embedding is disabled (provider = "none" or empty).
func NewEmbeddingProvider() EmbeddingProvider {
	provider := os.Getenv("MDDB_EMBEDDING_PROVIDER")
	if provider == "" || provider == "none" {
		return nil
	}

	switch provider {
	case "openai":
		apiKey := os.Getenv("MDDB_EMBEDDING_API_KEY")
		if apiKey == "" {
			log.Println("WARNING: MDDB_EMBEDDING_PROVIDER=openai but MDDB_EMBEDDING_API_KEY not set")
			return nil
		}
		apiURL := envDefault("MDDB_EMBEDDING_API_URL", "https://api.openai.com/v1")
		model := envDefault("MDDB_EMBEDDING_MODEL", "text-embedding-3-small")
		dims := envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 1536)
		return NewOpenAIEmbeddingProvider(apiKey, apiURL, model, dims)

	case "ollama":
		apiURL := envDefault("MDDB_EMBEDDING_API_URL", "http://localhost:11434")
		model := envDefault("MDDB_EMBEDDING_MODEL", "nomic-embed-text")
		dims := envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 768)
		return NewOllamaEmbeddingProvider(apiURL, model, dims)

	case "voyage":
		apiKey := os.Getenv("MDDB_EMBEDDING_API_KEY")
		if apiKey == "" {
			log.Println("WARNING: MDDB_EMBEDDING_PROVIDER=voyage but MDDB_EMBEDDING_API_KEY not set")
			return nil
		}
		apiURL := envDefault("MDDB_EMBEDDING_API_URL", "https://api.voyageai.com/v1")
		model := envDefault("MDDB_EMBEDDING_MODEL", "voyage-3")
		dims := envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 1024)
		return NewVoyageEmbeddingProvider(apiKey, apiURL, model, dims)

	default:
		log.Printf("WARNING: unknown MDDB_EMBEDDING_PROVIDER=%q, embedding disabled", provider)
		return nil
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDefaultInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
