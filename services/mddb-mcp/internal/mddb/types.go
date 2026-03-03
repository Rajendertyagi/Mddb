package mddb

import "time"

// Document represents a markdown document in MDDB.
type Document struct {
	ID        string              `json:"id"`
	Key       string              `json:"key"`
	Lang      string              `json:"lang"`
	Meta      map[string][]string `json:"meta"`
	ContentMD string              `json:"content_md"`
	AddedAt   time.Time           `json:"added_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// Health represents server health status.
type Health struct {
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

// Stats represents server statistics.
type Stats struct {
	DatabasePath     string            `json:"database_path"`
	DatabaseSize     int64             `json:"database_size"`
	Mode             string            `json:"mode"`
	Collections      []CollectionStats `json:"collections"`
	TotalDocuments   int               `json:"total_documents"`
	TotalRevisions   int               `json:"total_revisions"`
	TotalMetaIndices int               `json:"total_meta_indices"`
}

// CollectionStats represents collection statistics.
type CollectionStats struct {
	Name           string `json:"name"`
	DocumentCount  int    `json:"document_count"`
	RevisionCount  int    `json:"revision_count"`
	MetaIndexCount int    `json:"meta_index_count"`
}

// AddRequest represents request to add/update a document.
type AddRequest struct {
	Collection   string              `json:"collection"`
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// GetRequest represents request to get a document.
type GetRequest struct {
	Collection string            `json:"collection"`
	Key        string            `json:"key"`
	Lang       string            `json:"lang"`
	Env        map[string]string `json:"env,omitempty"`
}

// SearchRequest represents search request.
type SearchRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filter_meta,omitempty"`
	Sort       string              `json:"sort,omitempty"`
	Asc        bool                `json:"asc,omitempty"`
	Limit      int                 `json:"limit,omitempty"`
	Offset     int                 `json:"offset,omitempty"`
}

// SearchResponse represents search result.
type SearchResponse struct {
	Documents []Document `json:"documents"`
	Total     int        `json:"total"`
}

// DeleteRequest represents request to delete a document.
type DeleteRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
}

// DeleteCollectionRequest represents request to delete a collection.
type DeleteCollectionRequest struct {
	Collection string `json:"collection"`
}

// DeleteCollectionResponse represents result of collection deletion.
type DeleteCollectionResponse struct {
	Deleted int `json:"deleted"`
}

// BatchDocument represents a document in batch operation.
type BatchDocument struct {
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// AddBatchRequest represents request to add multiple documents.
type AddBatchRequest struct {
	Collection string          `json:"collection"`
	Documents  []BatchDocument `json:"documents"`
}

// AddBatchResponse represents result of adding multiple documents.
type AddBatchResponse struct {
	Added   int      `json:"added"`
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// UpdateDocument represents a document to update.
type UpdateDocument struct {
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// UpdateBatchRequest represents request to update multiple documents.
type UpdateBatchRequest struct {
	Collection string           `json:"collection"`
	Documents  []UpdateDocument `json:"documents"`
}

// UpdateBatchResponse represents result of updating multiple documents.
type UpdateBatchResponse struct {
	Updated  int      `json:"updated"`
	NotFound int      `json:"not_found"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// DeleteDocument represents a document to delete.
type DeleteDocument struct {
	Key  string `json:"key"`
	Lang string `json:"lang"`
}

// DeleteBatchRequest represents request to delete multiple documents.
type DeleteBatchRequest struct {
	Collection string           `json:"collection"`
	Documents  []DeleteDocument `json:"documents"`
}

// DeleteBatchResponse represents result of deleting multiple documents.
type DeleteBatchResponse struct {
	Deleted  int      `json:"deleted"`
	NotFound int      `json:"not_found"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// ExportRequest represents export request.
type ExportRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filter_meta,omitempty"`
	Format     string              `json:"format"` // ndjson, zip
}

// BackupRequest represents backup request.
type BackupRequest struct {
	To string `json:"to"`
}

// BackupResponse represents backup result.
type BackupResponse struct {
	Backup string `json:"backup"`
}

// RestoreRequest represents restore from backup request.
type RestoreRequest struct {
	From string `json:"from"`
}

// RestoreResponse represents restore result.
type RestoreResponse struct {
	Restored string `json:"restored"`
}

// TruncateRequest represents request to truncate revision history.
type TruncateRequest struct {
	Collection string `json:"collection"`
	KeepRevs   int    `json:"keep_revs"`
	DropCache  bool   `json:"drop_cache"`
}

// TruncateResponse represents truncate result.
type TruncateResponse struct {
	Status string `json:"status"`
}

// VectorSearchRequest represents vector/semantic search request.
type VectorSearchRequest struct {
	Collection     string              `json:"collection"`
	Query          string              `json:"query"`
	QueryVector    []float32           `json:"queryVector,omitempty"`
	TopK           int                 `json:"topK,omitempty"`
	Threshold      float64             `json:"threshold,omitempty"`
	FilterMeta     map[string][]string `json:"filterMeta,omitempty"`
	IncludeContent bool                `json:"includeContent,omitempty"`
	Algorithm      string              `json:"algorithm,omitempty"` // "flat" (default), "hnsw", "ivf", "pq"
}

// VectorSearchResult represents a single semantic search result.
type VectorSearchResult struct {
	Document Document `json:"document"`
	Score    float32  `json:"score"`
	Rank     int      `json:"rank"`
}

// VectorSearchResponse represents vector search results.
type VectorSearchResponse struct {
	Results    []VectorSearchResult `json:"results"`
	Total      int                  `json:"total"`
	Model      string               `json:"model"`
	Dimensions int                  `json:"dimensions"`
	Algorithm  string               `json:"algorithm"`
}

// VectorReindexRequest represents a reindex request.
type VectorReindexRequest struct {
	Collection string `json:"collection"`
	Force      bool   `json:"force"`
}

// VectorReindexResponse represents reindex results.
type VectorReindexResponse struct {
	Embedded int      `json:"embedded"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// VectorStatsResponse represents vector stats.
type VectorStatsResponse struct {
	Provider    string                           `json:"provider"`
	Model       string                           `json:"model"`
	Dimensions  int                              `json:"dimensions"`
	Enabled     bool                             `json:"enabled"`
	Collections map[string]VectorCollectionStats `json:"collections"`
}

// VectorCollectionStats represents per-collection embedding stats.
type VectorCollectionStats struct {
	TotalDocuments    int `json:"total_documents"`
	EmbeddedDocuments int `json:"embedded_documents"`
}

// ImportURLRequest represents request to import a document from URL.
type ImportURLRequest struct {
	Collection string              `json:"collection"`
	URL        string              `json:"url"`
	Key        string              `json:"key,omitempty"`
	Lang       string              `json:"lang"`
	Meta       map[string][]string `json:"meta,omitempty"`
	TTL        int64               `json:"ttl,omitempty"`
}

// SetTTLRequest represents request to set TTL on a document.
type SetTTLRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
	TTL        int64  `json:"ttl"`
}

// FTSSearchRequest represents full-text search request.
type FTSSearchRequest struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	Limit      int    `json:"limit,omitempty"`
	Algorithm  string `json:"algorithm,omitempty"` // "tfidf" (default) or "bm25"
	Fuzzy      int    `json:"fuzzy,omitempty"`     // typo tolerance: 0 (off), 1 (1 edit), 2 (2 edits)
}

// FTSResult represents a single FTS result.
type FTSResult struct {
	Document     Document `json:"document"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matchedTerms"`
}

// FTSSearchResponse represents full-text search results.
type FTSSearchResponse struct {
	Results   []FTSResult `json:"results"`
	Total     int         `json:"total"`
	Algorithm string      `json:"algorithm"`
	Fuzzy     int         `json:"fuzzy"`
}

// Webhook represents a webhook subscription.
type Webhook struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Collection string   `json:"collection,omitempty"`
	CreatedAt  int64    `json:"createdAt"`
}

// RegisterWebhookRequest represents request to register a webhook.
type RegisterWebhookRequest struct {
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Collection string   `json:"collection,omitempty"`
}

// DeleteWebhookRequest represents request to delete a webhook.
type DeleteWebhookRequest struct {
	ID string `json:"id"`
}

// SetSchemaRequest represents request to set a collection schema.
type SetSchemaRequest struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
}

// SchemaResponse represents a schema get response.
type SchemaResponse struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
	Enabled    bool   `json:"enabled"`
}

// SchemaInfo represents a schema in a list.
type SchemaInfo struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
}

// ListSchemasResponse represents list schemas result.
type ListSchemasResponse struct {
	Schemas []SchemaInfo `json:"schemas"`
}

// ValidateRequest represents request to validate document metadata.
type ValidateRequest struct {
	Collection string              `json:"collection"`
	Meta       map[string][]string `json:"meta"`
}

// ValidateResponse represents validation result.
type ValidateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}
