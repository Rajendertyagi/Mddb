package mddb

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "mddb/proto"
)

// GRPCClient implements Client via gRPC/Protobuf API.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.MDDBClient
	apiKey string // API key for authentication
}

// NewGRPCClient creates new gRPC client.
func NewGRPCClient(address string, apiKey string, timeout time.Duration) (*GRPCClient, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create grpc client: %w", err)
	}

	return &GRPCClient{
		conn:   conn,
		client: pb.NewMDDBClient(conn),
		apiKey: apiKey,
	}, nil
}

// withAuth injects authentication metadata into context if API key is configured.
func (c *GRPCClient) withAuth(ctx context.Context) context.Context {
	if c.apiKey == "" {
		return ctx
	}
	md := metadata.Pairs("x-api-key", c.apiKey)
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *GRPCClient) Health(ctx context.Context) (*Health, error) {
	// gRPC doesn't have dedicated health check in proto, use Stats as proxy
	stats, err := c.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return &Health{Status: "healthy", Mode: stats.Mode}, nil
}

func (c *GRPCClient) Stats(ctx context.Context) (*Stats, error) {
	resp, err := c.client.Stats(c.withAuth(ctx), &pb.StatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}

	collections := make([]CollectionStats, len(resp.Collections))
	for i, col := range resp.Collections {
		collections[i] = CollectionStats{
			Name:           col.Name,
			DocumentCount:  int(col.DocumentCount),
			RevisionCount:  int(col.RevisionCount),
			MetaIndexCount: int(col.MetaIndexCount),
		}
	}

	return &Stats{
		DatabasePath:     resp.DatabasePath,
		DatabaseSize:     resp.DatabaseSize,
		Mode:             resp.Mode,
		Collections:      collections,
		TotalDocuments:   int(resp.TotalDocuments),
		TotalRevisions:   int(resp.TotalRevisions),
		TotalMetaIndices: int(resp.TotalMetaIndices),
	}, nil
}

func (c *GRPCClient) Add(ctx context.Context, req *AddRequest) (*Document, error) {
	pbReq := &pb.AddRequest{
		Collection:   req.Collection,
		Key:          req.Key,
		Lang:         req.Lang,
		Meta:         convertMetaToProto(req.Meta),
		ContentMd:    req.ContentMD,
		SaveRevision: req.SaveRevision,
	}

	doc, err := c.client.Add(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}

	return convertDocumentFromProto(doc), nil
}

func (c *GRPCClient) AddBatch(ctx context.Context, req *AddBatchRequest) (*AddBatchResponse, error) {
	docs := make([]*pb.BatchDocument, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = &pb.BatchDocument{
			Key:          d.Key,
			Lang:         d.Lang,
			Meta:         convertMetaToProto(d.Meta),
			ContentMd:    d.ContentMD,
			SaveRevision: d.SaveRevision,
		}
	}

	pbReq := &pb.AddBatchRequest{
		Collection: req.Collection,
		Documents:  docs,
	}

	resp, err := c.client.AddBatch(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("add batch: %w", err)
	}

	return &AddBatchResponse{
		Added:   int(resp.Added),
		Updated: int(resp.Updated),
		Failed:  int(resp.Failed),
		Errors:  resp.Errors,
	}, nil
}

func (c *GRPCClient) UpdateBatch(ctx context.Context, req *UpdateBatchRequest) (*UpdateBatchResponse, error) {
	docs := make([]*pb.UpdateDocument, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = &pb.UpdateDocument{
			Key:          d.Key,
			Lang:         d.Lang,
			Meta:         convertMetaToProto(d.Meta),
			ContentMd:    d.ContentMD,
			SaveRevision: d.SaveRevision,
		}
	}

	pbReq := &pb.UpdateBatchRequest{
		Collection: req.Collection,
		Documents:  docs,
	}

	resp, err := c.client.UpdateBatch(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("update batch: %w", err)
	}

	return &UpdateBatchResponse{
		Updated:  int(resp.Updated),
		NotFound: int(resp.NotFound),
		Failed:   int(resp.Failed),
		Errors:   resp.Errors,
	}, nil
}

func (c *GRPCClient) DeleteBatch(ctx context.Context, req *DeleteBatchRequest) (*DeleteBatchResponse, error) {
	docs := make([]*pb.DeleteDocument, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = &pb.DeleteDocument{
			Key:  d.Key,
			Lang: d.Lang,
		}
	}

	pbReq := &pb.DeleteBatchRequest{
		Collection: req.Collection,
		Documents:  docs,
	}

	resp, err := c.client.DeleteBatch(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("delete batch: %w", err)
	}

	return &DeleteBatchResponse{
		Deleted:  int(resp.Deleted),
		NotFound: int(resp.NotFound),
		Failed:   int(resp.Failed),
		Errors:   resp.Errors,
	}, nil
}

func (c *GRPCClient) Get(ctx context.Context, req *GetRequest) (*Document, error) {
	pbReq := &pb.GetRequest{
		Collection: req.Collection,
		Key:        req.Key,
		Lang:       req.Lang,
		Env:        req.Env,
	}

	doc, err := c.client.Get(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	return convertDocumentFromProto(doc), nil
}

func (c *GRPCClient) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	pbReq := &pb.SearchRequest{
		Collection: req.Collection,
		FilterMeta: convertMetaToProto(req.FilterMeta),
		Sort:       req.Sort,
		Asc:        req.Asc,
		Limit:      int32(req.Limit),
		Offset:     int32(req.Offset),
	}

	resp, err := c.client.Search(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	docs := make([]Document, len(resp.Documents))
	for i, d := range resp.Documents {
		docs[i] = *convertDocumentFromProto(d)
	}

	return &SearchResponse{
		Documents: docs,
		Total:     int(resp.Total),
	}, nil
}

func (c *GRPCClient) Delete(ctx context.Context, req *DeleteRequest) error {
	// gRPC uses DeleteBatch for single deletions
	_, err := c.DeleteBatch(ctx, &DeleteBatchRequest{
		Collection: req.Collection,
		Documents: []DeleteDocument{
			{Key: req.Key, Lang: req.Lang},
		},
	})
	return err
}

func (c *GRPCClient) DeleteCollection(ctx context.Context, req *DeleteCollectionRequest) (*DeleteCollectionResponse, error) {
	// gRPC doesn't have dedicated DeleteCollection, use DeleteBatch after Search
	searchResp, err := c.Search(ctx, &SearchRequest{
		Collection: req.Collection,
		Limit:      10000,
	})
	if err != nil {
		return nil, fmt.Errorf("search for delete collection: %w", err)
	}

	if len(searchResp.Documents) == 0 {
		return &DeleteCollectionResponse{Deleted: 0}, nil
	}

	docs := make([]DeleteDocument, len(searchResp.Documents))
	for i, d := range searchResp.Documents {
		docs[i] = DeleteDocument{Key: d.Key, Lang: d.Lang}
	}

	delResp, err := c.DeleteBatch(ctx, &DeleteBatchRequest{
		Collection: req.Collection,
		Documents:  docs,
	})
	if err != nil {
		return nil, fmt.Errorf("delete batch: %w", err)
	}

	return &DeleteCollectionResponse{Deleted: delResp.Deleted}, nil
}

func (c *GRPCClient) Export(ctx context.Context, req *ExportRequest) (io.ReadCloser, error) {
	pbReq := &pb.ExportRequest{
		Collection: req.Collection,
		FilterMeta: convertMetaToProto(req.FilterMeta),
		Format:     req.Format,
	}

	stream, err := c.client.Export(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		defer func() {
			_ = pw.Close()
		}()
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = pw.CloseWithError(fmt.Errorf("recv chunk: %w", err))
				return
			}
			if _, err := pw.Write(chunk.Data); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("write chunk: %w", err))
				return
			}
		}
	}()

	return pr, nil
}

func (c *GRPCClient) Backup(ctx context.Context, req *BackupRequest) (*BackupResponse, error) {
	pbReq := &pb.BackupRequest{To: req.To}
	resp, err := c.client.Backup(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	return &BackupResponse{Backup: resp.Backup}, nil
}

func (c *GRPCClient) Restore(ctx context.Context, req *RestoreRequest) (*RestoreResponse, error) {
	pbReq := &pb.RestoreRequest{From: req.From}
	resp, err := c.client.Restore(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("restore: %w", err)
	}
	return &RestoreResponse{Restored: resp.Restored}, nil
}

func (c *GRPCClient) Truncate(ctx context.Context, req *TruncateRequest) (*TruncateResponse, error) {
	pbReq := &pb.TruncateRequest{
		Collection: req.Collection,
		KeepRevs:   int32(req.KeepRevs),
		DropCache:  req.DropCache,
	}
	resp, err := c.client.Truncate(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("truncate: %w", err)
	}
	return &TruncateResponse{Status: resp.Status}, nil
}

func (c *GRPCClient) VectorSearch(ctx context.Context, req *VectorSearchRequest) (*VectorSearchResponse, error) {
	pbReq := &pb.VectorSearchRequest{
		Collection:     req.Collection,
		Query:          req.Query,
		QueryVector:    req.QueryVector,
		TopK:           int32(req.TopK),
		Threshold:      req.Threshold,
		FilterMeta:     convertMetaToProto(req.FilterMeta),
		IncludeContent: req.IncludeContent,
		Algorithm:      req.Algorithm,
	}

	resp, err := c.client.VectorSearch(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]VectorSearchResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = VectorSearchResult{
			Document: *convertDocumentFromProto(r.Document),
			Score:    r.Score,
			Rank:     int(r.Rank),
		}
	}

	return &VectorSearchResponse{
		Results:    results,
		Total:      int(resp.Total),
		Model:      resp.Model,
		Dimensions: int(resp.Dimensions),
		Algorithm:  resp.Algorithm,
	}, nil
}

func (c *GRPCClient) VectorReindex(ctx context.Context, req *VectorReindexRequest) (*VectorReindexResponse, error) {
	pbReq := &pb.VectorReindexRequest{
		Collection: req.Collection,
		Force:      req.Force,
	}

	resp, err := c.client.VectorReindex(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("vector reindex: %w", err)
	}

	return &VectorReindexResponse{
		Embedded: int(resp.Embedded),
		Skipped:  int(resp.Skipped),
		Failed:   int(resp.Failed),
		Errors:   resp.Errors,
	}, nil
}

func (c *GRPCClient) VectorStats(ctx context.Context) (*VectorStatsResponse, error) {
	resp, err := c.client.VectorStats(c.withAuth(ctx), &pb.VectorStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("vector stats: %w", err)
	}

	collections := make(map[string]VectorCollectionStats, len(resp.Collections))
	for name, cs := range resp.Collections {
		collections[name] = VectorCollectionStats{
			TotalDocuments:    int(cs.TotalDocuments),
			EmbeddedDocuments: int(cs.EmbeddedDocuments),
		}
	}

	return &VectorStatsResponse{
		Provider:    resp.Provider,
		Model:       resp.Model,
		Dimensions:  int(resp.Dimensions),
		Enabled:     resp.Enabled,
		Collections: collections,
	}, nil
}

func (c *GRPCClient) ImportURL(ctx context.Context, req *ImportURLRequest) (*Document, error) {
	pbReq := &pb.ImportURLRequest{
		Collection: req.Collection,
		Url:        req.URL,
		Key:        req.Key,
		Lang:       req.Lang,
		Meta:       convertMetaToProto(req.Meta),
		Ttl:        req.TTL,
	}
	doc, err := c.client.ImportURL(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("import url: %w", err)
	}
	return convertDocumentFromProto(doc), nil
}

func (c *GRPCClient) SetTTL(ctx context.Context, req *SetTTLRequest) (*Document, error) {
	pbReq := &pb.SetTTLRequest{
		Collection: req.Collection,
		Key:        req.Key,
		Lang:       req.Lang,
		Ttl:        req.TTL,
	}
	doc, err := c.client.SetTTL(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("set ttl: %w", err)
	}
	return convertDocumentFromProto(doc), nil
}

func (c *GRPCClient) FTSSearch(ctx context.Context, req *FTSSearchRequest) (*FTSSearchResponse, error) {
	pbReq := &pb.FTSRequest{
		Collection: req.Collection,
		Query:      req.Query,
		Limit:      int32(req.Limit),
		Algorithm:  req.Algorithm,
		Fuzzy:      int32(req.Fuzzy),
	}
	resp, err := c.client.FTS(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	results := make([]FTSResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = FTSResult{
			Document:     *convertDocumentFromProto(r.Document),
			Score:        r.Score,
			MatchedTerms: r.MatchedTerms,
		}
	}
	return &FTSSearchResponse{Results: results, Total: int(resp.Total), Algorithm: resp.Algorithm, Fuzzy: int(resp.Fuzzy)}, nil
}

func (c *GRPCClient) RegisterWebhook(ctx context.Context, req *RegisterWebhookRequest) (*Webhook, error) {
	pbReq := &pb.RegisterWebhookRequest{
		Url:        req.URL,
		Events:     req.Events,
		Collection: req.Collection,
	}
	resp, err := c.client.RegisterWebhook(c.withAuth(ctx), pbReq)
	if err != nil {
		return nil, fmt.Errorf("register webhook: %w", err)
	}
	return &Webhook{
		ID:         resp.Id,
		URL:        resp.Url,
		Events:     resp.Events,
		Collection: resp.Collection,
		CreatedAt:  resp.CreatedAt,
	}, nil
}

func (c *GRPCClient) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	resp, err := c.client.ListWebhooks(c.withAuth(ctx), &pb.ListWebhooksRequest{})
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	hooks := make([]Webhook, len(resp.Webhooks))
	for i, h := range resp.Webhooks {
		hooks[i] = Webhook{
			ID:         h.Id,
			URL:        h.Url,
			Events:     h.Events,
			Collection: h.Collection,
			CreatedAt:  h.CreatedAt,
		}
	}
	return hooks, nil
}

func (c *GRPCClient) DeleteWebhook(ctx context.Context, req *DeleteWebhookRequest) error {
	_, err := c.client.DeleteWebhook(c.withAuth(ctx), &pb.DeleteWebhookRequest{Id: req.ID})
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	return nil
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// convertMetaToProto konwertuje meta z map[string][]string na proto format.
func convertMetaToProto(meta map[string][]string) map[string]*pb.MetaValues {
	if meta == nil {
		return nil
	}
	result := make(map[string]*pb.MetaValues, len(meta))
	for k, v := range meta {
		result[k] = &pb.MetaValues{Values: v}
	}
	return result
}

// convertMetaFromProto konwertuje meta z proto na map[string][]string.
func convertMetaFromProto(meta map[string]*pb.MetaValues) map[string][]string {
	if meta == nil {
		return nil
	}
	result := make(map[string][]string, len(meta))
	for k, v := range meta {
		result[k] = v.Values
	}
	return result
}

func (c *GRPCClient) SetSchema(ctx context.Context, req *SetSchemaRequest) error {
	_, err := c.client.SetSchema(c.withAuth(ctx), &pb.SetSchemaRequest{
		Collection: req.Collection,
		Schema:     req.Schema,
	})
	return err
}

func (c *GRPCClient) GetSchema(ctx context.Context, collection string) (*SchemaResponse, error) {
	resp, err := c.client.GetSchema(c.withAuth(ctx), &pb.GetSchemaRequest{Collection: collection})
	if err != nil {
		return nil, err
	}
	return &SchemaResponse{
		Collection: resp.Collection,
		Schema:     resp.Schema,
		Enabled:    resp.Enabled,
	}, nil
}

func (c *GRPCClient) DeleteSchema(ctx context.Context, collection string) error {
	_, err := c.client.DeleteSchema(c.withAuth(ctx), &pb.DeleteSchemaRequest{Collection: collection})
	return err
}

func (c *GRPCClient) ListSchemas(ctx context.Context) (*ListSchemasResponse, error) {
	resp, err := c.client.ListSchemas(c.withAuth(ctx), &pb.ListSchemasRequest{})
	if err != nil {
		return nil, err
	}
	var schemas []SchemaInfo
	for _, s := range resp.Schemas {
		schemas = append(schemas, SchemaInfo{Collection: s.Collection, Schema: s.Schema})
	}
	return &ListSchemasResponse{Schemas: schemas}, nil
}

func (c *GRPCClient) ValidateDocument(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error) {
	resp, err := c.client.ValidateDocument(c.withAuth(ctx), &pb.ValidateDocumentRequest{
		Collection: req.Collection,
		Meta:       convertMetaToProto(req.Meta),
	})
	if err != nil {
		return nil, err
	}
	return &ValidateResponse{Valid: resp.Valid, Errors: resp.Errors}, nil
}

// convertDocumentFromProto converts Document from proto to internal type.
func convertDocumentFromProto(doc *pb.Document) *Document {
	return &Document{
		ID:        doc.Id,
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      convertMetaFromProto(doc.Meta),
		ContentMD: doc.ContentMd,
		AddedAt:   time.Unix(doc.AddedAt, 0),
		UpdatedAt: time.Unix(doc.UpdatedAt, 0),
	}
}
