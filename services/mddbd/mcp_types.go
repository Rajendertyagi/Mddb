package main

import (
	"context"
	"io"
	"time"
)

// MCPDocument represents a markdown document in MCP format.
type MCPDocument struct {
	ID        string              `json:"id"`
	Key       string              `json:"key"`
	Lang      string              `json:"lang"`
	Meta      map[string][]string `json:"meta"`
	ContentMD string              `json:"content_md"`
	AddedAt   time.Time           `json:"added_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// MCPHealth represents server health status.
type MCPHealth struct {
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

// MCPStats represents server statistics.
type MCPStats struct {
	DatabasePath     string               `json:"database_path"`
	DatabaseSize     int64                `json:"database_size"`
	Mode             string               `json:"mode"`
	Collections      []MCPCollectionStats `json:"collections"`
	TotalDocuments   int                  `json:"total_documents"`
	TotalRevisions   int                  `json:"total_revisions"`
	TotalMetaIndices int                  `json:"total_meta_indices"`
}

// MCPCollectionStats represents collection statistics.
type MCPCollectionStats struct {
	Name           string `json:"name"`
	DocumentCount  int    `json:"document_count"`
	RevisionCount  int    `json:"revision_count"`
	MetaIndexCount int    `json:"meta_index_count"`
}

// MCPAddRequest represents request to add/update a document.
type MCPAddRequest struct {
	Collection   string              `json:"collection"`
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// MCPGetRequest represents request to get a document.
type MCPGetRequest struct {
	Collection string            `json:"collection"`
	Key        string            `json:"key"`
	Lang       string            `json:"lang"`
	Env        map[string]string `json:"env,omitempty"`
}

// MCPSearchRequest represents search request.
type MCPSearchRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filter_meta,omitempty"`
	Sort       string              `json:"sort,omitempty"`
	Asc        bool                `json:"asc,omitempty"`
	Limit      int                 `json:"limit,omitempty"`
	Offset     int                 `json:"offset,omitempty"`
}

// MCPSearchResponse represents search result.
type MCPSearchResponse struct {
	Documents []MCPDocument `json:"documents"`
	Total     int           `json:"total"`
}

// MCPDeleteRequest represents request to delete a document.
type MCPDeleteRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
}

// MCPDeleteCollectionRequest represents request to delete a collection.
type MCPDeleteCollectionRequest struct {
	Collection string `json:"collection"`
}

// MCPDeleteCollectionResponse represents result of collection deletion.
type MCPDeleteCollectionResponse struct {
	Deleted int `json:"deleted"`
}

// MCPBatchDocument represents a document in batch operation.
type MCPBatchDocument struct {
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// MCPAddBatchRequest represents request to add multiple documents.
type MCPAddBatchRequest struct {
	Collection string             `json:"collection"`
	Documents  []MCPBatchDocument `json:"documents"`
}

// MCPAddBatchResponse represents result of adding multiple documents.
type MCPAddBatchResponse struct {
	Added   int      `json:"added"`
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// MCPUpdateDocument represents a document to update.
type MCPUpdateDocument struct {
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta"`
	ContentMD    string              `json:"content_md"`
	SaveRevision bool                `json:"save_revision"`
}

// MCPUpdateBatchRequest represents request to update multiple documents.
type MCPUpdateBatchRequest struct {
	Collection string              `json:"collection"`
	Documents  []MCPUpdateDocument `json:"documents"`
}

// MCPUpdateBatchResponse represents result of updating multiple documents.
type MCPUpdateBatchResponse struct {
	Updated  int      `json:"updated"`
	NotFound int      `json:"not_found"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// MCPDeleteDocument represents a document to delete.
type MCPDeleteDocument struct {
	Key  string `json:"key"`
	Lang string `json:"lang"`
}

// MCPDeleteBatchRequest represents request to delete multiple documents.
type MCPDeleteBatchRequest struct {
	Collection string              `json:"collection"`
	Documents  []MCPDeleteDocument `json:"documents"`
}

// MCPDeleteBatchResponse represents result of deleting multiple documents.
type MCPDeleteBatchResponse struct {
	Deleted  int      `json:"deleted"`
	NotFound int      `json:"not_found"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// MCPExportRequest represents export request.
type MCPExportRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filter_meta,omitempty"`
	Format     string              `json:"format"`
}

// MCPBackupRequest represents backup request.
type MCPBackupRequest struct {
	To string `json:"to"`
}

// MCPBackupResponse represents backup result.
type MCPBackupResponse struct {
	Backup string `json:"backup"`
}

// MCPRestoreRequest represents restore from backup request.
type MCPRestoreRequest struct {
	From string `json:"from"`
}

// MCPRestoreResponse represents restore result.
type MCPRestoreResponse struct {
	Restored string `json:"restored"`
}

// MCPTruncateRequest represents request to truncate revision history.
type MCPTruncateRequest struct {
	Collection string `json:"collection"`
	KeepRevs   int    `json:"keep_revs"`
	DropCache  bool   `json:"drop_cache"`
}

// MCPTruncateResponse represents truncate result.
type MCPTruncateResponse struct {
	Status string `json:"status"`
}

// MCPVectorSearchRequest represents vector/semantic search request.
type MCPVectorSearchRequest struct {
	Collection     string              `json:"collection"`
	Query          string              `json:"query"`
	QueryVector    []float32           `json:"queryVector,omitempty"`
	TopK           int                 `json:"topK,omitempty"`
	Threshold      float64             `json:"threshold,omitempty"`
	FilterMeta     map[string][]string `json:"filterMeta,omitempty"`
	IncludeContent bool                `json:"includeContent,omitempty"`
	Algorithm      string              `json:"algorithm,omitempty"`
}

// MCPVectorSearchResult represents a single semantic search result.
type MCPVectorSearchResult struct {
	Document MCPDocument `json:"document"`
	Score    float32     `json:"score"`
	Rank     int         `json:"rank"`
}

// MCPVectorSearchResponse represents vector search results.
type MCPVectorSearchResponse struct {
	Results    []MCPVectorSearchResult `json:"results"`
	Total      int                     `json:"total"`
	Model      string                  `json:"model"`
	Dimensions int                     `json:"dimensions"`
	Algorithm  string                  `json:"algorithm"`
}

// MCPVectorReindexRequest represents a reindex request.
type MCPVectorReindexRequest struct {
	Collection string `json:"collection"`
	Force      bool   `json:"force"`
}

// MCPVectorReindexResponse represents reindex results.
type MCPVectorReindexResponse struct {
	Embedded int      `json:"embedded"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// MCPVectorStatsResponse represents vector stats.
type MCPVectorStatsResponse struct {
	Provider    string                              `json:"provider"`
	Model       string                              `json:"model"`
	Dimensions  int                                 `json:"dimensions"`
	Enabled     bool                                `json:"enabled"`
	Collections map[string]MCPVectorCollectionStats `json:"collections"`
}

// MCPVectorCollectionStats represents per-collection embedding stats.
type MCPVectorCollectionStats struct {
	TotalDocuments    int `json:"total_documents"`
	EmbeddedDocuments int `json:"embedded_documents"`
}

// MCPImportURLRequest represents request to import a document from URL.
type MCPImportURLRequest struct {
	Collection string              `json:"collection"`
	URL        string              `json:"url"`
	Key        string              `json:"key,omitempty"`
	Lang       string              `json:"lang"`
	Meta       map[string][]string `json:"meta,omitempty"`
	TTL        int64               `json:"ttl,omitempty"`
}

// MCPSetTTLRequest represents request to set TTL on a document.
type MCPSetTTLRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
	TTL        int64  `json:"ttl"`
}

// MCPFTSSearchRequest represents full-text search request.
type MCPFTSSearchRequest struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	Limit      int    `json:"limit,omitempty"`
	Algorithm  string `json:"algorithm,omitempty"`
	Fuzzy      int    `json:"fuzzy,omitempty"`
}

// MCPFTSResult represents a single FTS result.
type MCPFTSResult struct {
	Document     MCPDocument `json:"document"`
	Score        float64     `json:"score"`
	MatchedTerms []string    `json:"matchedTerms"`
}

// MCPFTSSearchResponse represents full-text search results.
type MCPFTSSearchResponse struct {
	Results   []MCPFTSResult `json:"results"`
	Total     int            `json:"total"`
	Algorithm string         `json:"algorithm"`
	Fuzzy     int            `json:"fuzzy"`
}

// MCPHybridSearchRequest represents hybrid sparse+dense search request.
type MCPHybridSearchRequest struct {
	Collection      string              `json:"collection"`
	Query           string              `json:"query"`
	TopK            int                 `json:"topK,omitempty"`
	Algorithm       string              `json:"algorithm,omitempty"`       // FTS: "bm25", "bm25f"
	VectorAlgorithm string              `json:"vectorAlgorithm,omitempty"` // Vector: "flat", "hnsw", "ivf", "pq", "sq"
	Alpha           float64             `json:"alpha,omitempty"`           // 0-1, default 0.5
	Strategy        string              `json:"strategy,omitempty"`        // "alpha" or "rrf"
	RRFK            int                 `json:"rrfK,omitempty"`            // RRF k parameter
	Fuzzy           int                 `json:"fuzzy,omitempty"`
	Threshold       float64             `json:"threshold,omitempty"`
	FilterMeta      map[string][]string `json:"filterMeta,omitempty"`
}

// MCPHybridSearchResult represents a single hybrid search result.
type MCPHybridSearchResult struct {
	Document      MCPDocument `json:"document"`
	CombinedScore float64     `json:"combinedScore"`
	FTSScore      float64     `json:"ftsScore"`
	VectorScore   float64     `json:"vectorScore"`
	MatchedTerms  []string    `json:"matchedTerms,omitempty"`
	Rank          int         `json:"rank"`
}

// MCPHybridSearchResponse represents hybrid search results.
type MCPHybridSearchResponse struct {
	Results         []MCPHybridSearchResult `json:"results"`
	Total           int                     `json:"total"`
	Strategy        string                  `json:"strategy"`
	FTSAlgorithm    string                  `json:"ftsAlgorithm"`
	VectorAlgorithm string                  `json:"vectorAlgorithm"`
}

// MCPWebhook represents a webhook subscription.
type MCPWebhook struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Collection string   `json:"collection,omitempty"`
	CreatedAt  int64    `json:"createdAt"`
}

// MCPRegisterWebhookRequest represents request to register a webhook.
type MCPRegisterWebhookRequest struct {
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Collection string   `json:"collection,omitempty"`
}

// MCPDeleteWebhookRequest represents request to delete a webhook.
type MCPDeleteWebhookRequest struct {
	ID string `json:"id"`
}

// MCPSetSchemaRequest represents request to set a collection schema.
type MCPSetSchemaRequest struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
}

// MCPSchemaResponse represents a schema get response.
type MCPSchemaResponse struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
	Enabled    bool   `json:"enabled"`
}

// MCPSchemaInfo represents a schema in a list.
type MCPSchemaInfo struct {
	Collection string `json:"collection"`
	Schema     string `json:"schema"`
}

// MCPListSchemasResponse represents list schemas result.
type MCPListSchemasResponse struct {
	Schemas []MCPSchemaInfo `json:"schemas"`
}

// MCPValidateRequest represents request to validate document metadata.
type MCPValidateRequest struct {
	Collection string              `json:"collection"`
	Meta       map[string][]string `json:"meta"`
}

// MCPValidateResponse represents validation result.
type MCPValidateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// --- MCP Protocol Types ---

// MCPResource represents an MCP resource.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// MCPResourceReadRequest represents resource read request.
type MCPResourceReadRequest struct {
	URI string `json:"uri"`
}

// MCPToolCallRequest represents tool call request.
type MCPToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// --- MCP Client Interface ---

// MCPClient is the interface for MCP to access MDDB operations.
type MCPClient interface {
	Health(ctx context.Context) (*MCPHealth, error)
	Stats(ctx context.Context) (*MCPStats, error)
	Add(ctx context.Context, req *MCPAddRequest) (*MCPDocument, error)
	AddBatch(ctx context.Context, req *MCPAddBatchRequest) (*MCPAddBatchResponse, error)
	UpdateBatch(ctx context.Context, req *MCPUpdateBatchRequest) (*MCPUpdateBatchResponse, error)
	DeleteBatch(ctx context.Context, req *MCPDeleteBatchRequest) (*MCPDeleteBatchResponse, error)
	Get(ctx context.Context, req *MCPGetRequest) (*MCPDocument, error)
	Search(ctx context.Context, req *MCPSearchRequest) (*MCPSearchResponse, error)
	Delete(ctx context.Context, req *MCPDeleteRequest) error
	DeleteCollection(ctx context.Context, req *MCPDeleteCollectionRequest) (*MCPDeleteCollectionResponse, error)
	Export(ctx context.Context, req *MCPExportRequest) (io.ReadCloser, error)
	Backup(ctx context.Context, req *MCPBackupRequest) (*MCPBackupResponse, error)
	Restore(ctx context.Context, req *MCPRestoreRequest) (*MCPRestoreResponse, error)
	Truncate(ctx context.Context, req *MCPTruncateRequest) (*MCPTruncateResponse, error)
	VectorSearch(ctx context.Context, req *MCPVectorSearchRequest) (*MCPVectorSearchResponse, error)
	VectorReindex(ctx context.Context, req *MCPVectorReindexRequest) (*MCPVectorReindexResponse, error)
	VectorStats(ctx context.Context) (*MCPVectorStatsResponse, error)
	ImportURL(ctx context.Context, req *MCPImportURLRequest) (*MCPDocument, error)
	SetTTL(ctx context.Context, req *MCPSetTTLRequest) (*MCPDocument, error)
	FTSSearch(ctx context.Context, req *MCPFTSSearchRequest) (*MCPFTSSearchResponse, error)
	HybridSearch(ctx context.Context, req *MCPHybridSearchRequest) (*MCPHybridSearchResponse, error)
	RegisterWebhook(ctx context.Context, req *MCPRegisterWebhookRequest) (*MCPWebhook, error)
	ListWebhooks(ctx context.Context) ([]MCPWebhook, error)
	DeleteWebhook(ctx context.Context, req *MCPDeleteWebhookRequest) error
	SetSchema(ctx context.Context, req *MCPSetSchemaRequest) error
	GetSchema(ctx context.Context, collection string) (*MCPSchemaResponse, error)
	DeleteSchema(ctx context.Context, collection string) error
	ListSchemas(ctx context.Context) (*MCPListSchemasResponse, error)
	ValidateDocument(ctx context.Context, req *MCPValidateRequest) (*MCPValidateResponse, error)
	Close() error
}

// --- Type Conversion Helpers ---

func docToMCPDocument(d Doc) MCPDocument {
	return MCPDocument{
		ID:        d.ID,
		Key:       d.Key,
		Lang:      d.Lang,
		Meta:      d.Meta,
		ContentMD: d.ContentMD,
		AddedAt:   time.Unix(d.AddedAt, 0),
		UpdatedAt: time.Unix(d.UpdatedAt, 0),
	}
}
