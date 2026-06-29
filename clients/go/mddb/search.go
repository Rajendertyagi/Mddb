package mddb

import (
	"context"
	"encoding/json"
)

// FTS runs a full-text search. The response shape depends on the configured
// ranking/highlight options, so it is returned as raw JSON for the caller to
// decode into the shape it needs.
func (c *Client) FTS(ctx context.Context, req FTSRequest) (json.RawMessage, error) {
	return c.Do(ctx, "POST", "/v1/fts", req)
}

// VectorSearch runs a semantic (embedding) search. Returns raw JSON.
func (c *Client) VectorSearch(ctx context.Context, req VectorSearchRequest) (json.RawMessage, error) {
	return c.Do(ctx, "POST", "/v1/vector-search", req)
}

// VectorStats returns embedding/index statistics as raw JSON.
func (c *Client) VectorStats(ctx context.Context) (json.RawMessage, error) {
	return c.Do(ctx, "GET", "/v1/vector-stats", nil)
}

// VectorReindex rebuilds the vector index for a collection (all collections when
// collection is ""). Returns raw JSON describing the job.
func (c *Client) VectorReindex(ctx context.Context, collection string) (json.RawMessage, error) {
	return c.Do(ctx, "POST", "/v1/vector-reindex", map[string]string{"collection": collection})
}
