package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	json "github.com/goccy/go-json"
)

// OllamaEmbeddingProvider generates embeddings using local Ollama server.
type OllamaEmbeddingProvider struct {
	apiURL     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider.
func NewOllamaEmbeddingProvider(apiURL, model string, dimensions int) *OllamaEmbeddingProvider {
	return &OllamaEmbeddingProvider{
		apiURL:     apiURL,
		model:      model,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 60 * time.Second, // Ollama may be slower on first call
		},
	}
}

func (p *OllamaEmbeddingProvider) Model() string  { return p.model }
func (p *OllamaEmbeddingProvider) Dimensions() int { return p.dimensions }

// Embed generates an embedding for a single text.
func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: p.model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("empty embedding response from Ollama")
	}

	// Update dimensions from actual response
	if len(result.Embeddings[0]) > 0 {
		p.dimensions = len(result.Embeddings[0])
	}

	return float64sToFloat32s(result.Embeddings[0]), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (p *OllamaEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		v, err := p.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		vectors[i] = v
	}
	return vectors, nil
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}
