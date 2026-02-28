package mddb

import (
	"context"
	"io"
)

// Client is the MDDB client interface supporting all API operations.
type Client interface {
	// Health checks server health status.
	Health(ctx context.Context) (*Health, error)

	// Stats returns server and database statistics.
	Stats(ctx context.Context) (*Stats, error)

	// Add adds or updates a document.
	Add(ctx context.Context, req *AddRequest) (*Document, error)

	// AddBatch adds or updates multiple documents in one transaction.
	AddBatch(ctx context.Context, req *AddBatchRequest) (*AddBatchResponse, error)

	// UpdateBatch updates multiple documents in one transaction.
	UpdateBatch(ctx context.Context, req *UpdateBatchRequest) (*UpdateBatchResponse, error)

	// DeleteBatch deletes multiple documents in one transaction.
	DeleteBatch(ctx context.Context, req *DeleteBatchRequest) (*DeleteBatchResponse, error)

	// Get retrieves a document by key and language.
	Get(ctx context.Context, req *GetRequest) (*Document, error)

	// Search searches documents with filtering and sorting.
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)

	// Delete deletes a single document.
	Delete(ctx context.Context, req *DeleteRequest) error

	// DeleteCollection deletes entire collection.
	DeleteCollection(ctx context.Context, req *DeleteCollectionRequest) (*DeleteCollectionResponse, error)

	// Export exports documents (returns data stream).
	Export(ctx context.Context, req *ExportRequest) (io.ReadCloser, error)

	// Backup creates database backup.
	Backup(ctx context.Context, req *BackupRequest) (*BackupResponse, error)

	// Restore restores database from backup.
	Restore(ctx context.Context, req *RestoreRequest) (*RestoreResponse, error)

	// Truncate truncates revision history.
	Truncate(ctx context.Context, req *TruncateRequest) (*TruncateResponse, error)

	// VectorSearch performs semantic/vector search.
	VectorSearch(ctx context.Context, req *VectorSearchRequest) (*VectorSearchResponse, error)

	// VectorReindex re-embeds documents in a collection.
	VectorReindex(ctx context.Context, req *VectorReindexRequest) (*VectorReindexResponse, error)

	// VectorStats returns vector/embedding statistics.
	VectorStats(ctx context.Context) (*VectorStatsResponse, error)

	// ImportURL imports a document from a URL.
	ImportURL(ctx context.Context, req *ImportURLRequest) (*Document, error)

	// SetTTL sets or removes TTL on a document.
	SetTTL(ctx context.Context, req *SetTTLRequest) (*Document, error)

	// FTSSearch performs full-text search.
	FTSSearch(ctx context.Context, req *FTSSearchRequest) (*FTSSearchResponse, error)

	// RegisterWebhook registers a webhook.
	RegisterWebhook(ctx context.Context, req *RegisterWebhookRequest) (*Webhook, error)

	// ListWebhooks lists all webhooks.
	ListWebhooks(ctx context.Context) ([]Webhook, error)

	// DeleteWebhook deletes a webhook by ID.
	DeleteWebhook(ctx context.Context, req *DeleteWebhookRequest) error

	// Close closes connection to server.
	Close() error
}
