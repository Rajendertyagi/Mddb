package main

// GraphQLServerAdapter implements the gql.ServerInterface contract by
// delegating to the in-process MCP DirectClient (which has full coverage of
// the REST/gRPC surface) plus AuthManager for user/group/permission ops.
//
// The adapter performs every authentication / authorization check at its
// boundary so resolvers in services/mddbd/graphql/schema.resolvers.go stay
// thin one-liners. Permission semantics:
//
//   - When AuthManager is nil or auth disabled → all operations allowed.
//   - Otherwise: require authenticated claims for every operation except
//     `Login` (handled by Authenticate/GenerateJWT pair).
//   - Per-collection ops additionally call CheckPermission for read or write.
//   - Admin-only ops verify IsAdmin(currentUser).

import (
	"context"
	"errors"
	"fmt"
	"time"

	gql "mddb/graphql"
)

// Sentinel errors returned by the GraphQL adapter.
var (
	ErrAuthNotEnabled         = errors.New("authentication not enabled")
	ErrVectorSearchNotEnabled = errors.New("vector search not enabled")
	ErrInvalidRequest         = errors.New("invalid request")
	ErrUnauthenticated        = errors.New("unauthenticated: missing or invalid credentials")
	ErrAdminRequired          = errors.New("forbidden: admin privileges required")
)

// GraphQLAdapter bridges the gql.ServerInterface to the main package's
// Server, AuthManager and DirectClient.
type GraphQLAdapter struct {
	server *Server
	mcp    *DirectClient
}

// NewGraphQLServerAdapter constructs the adapter wired to a running Server.
func NewGraphQLServerAdapter(s *Server) gql.ServerInterface {
	return &GraphQLAdapter{
		server: s,
		mcp:    NewDirectClient(s),
	}
}

// =============================================================================
// Auth primitives (called by directives, resolvers, and adapter internals).
// =============================================================================

func (a *GraphQLAdapter) IsAuthEnabled() bool {
	return a.server.AuthManager != nil && a.server.AuthManager.IsEnabled()
}

func (a *GraphQLAdapter) GetClaimsFromContext(ctx context.Context) (gql.Claims, bool) {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return gql.Claims{}, false
	}
	return gql.Claims{Username: claims.Username, Admin: claims.Admin}, true
}

func (a *GraphQLAdapter) CheckPermission(ctx context.Context, collection string, perm int) error {
	if !a.IsAuthEnabled() {
		return nil
	}
	return a.server.AuthManager.CheckPermission(ctx, collection, PermissionType(perm))
}

func (a *GraphQLAdapter) Authenticate(username, password string) (gql.UserInfo, error) {
	if !a.IsAuthEnabled() {
		return gql.UserInfo{}, ErrAuthNotEnabled
	}
	user, err := a.server.AuthManager.Authenticate(username, password)
	if err != nil {
		return gql.UserInfo{}, err
	}
	return gql.UserInfo{
		Username:  user.Username,
		Admin:     a.server.AuthManager.IsAdmin(username),
		CreatedAt: user.CreatedAt,
	}, nil
}

func (a *GraphQLAdapter) GenerateJWT(username string, isAdmin bool) (string, int64, error) {
	if !a.IsAuthEnabled() {
		return "", 0, ErrAuthNotEnabled
	}
	expiry := a.server.AuthManager.config.JWTExpiry
	token, err := GenerateJWT(username, isAdmin, a.server.AuthManager.config.JWTSecret, expiry)
	if err != nil {
		return "", 0, err
	}
	return token, time.Now().Add(expiry).Unix(), nil
}

// requireAuthenticated returns the current claims, or an error if no user is
// authenticated. When auth is disabled it returns a synthetic admin claim so
// every code path can proceed uniformly.
func (a *GraphQLAdapter) requireAuthenticated(ctx context.Context) (*JWTClaims, error) {
	if !a.IsAuthEnabled() {
		return &JWTClaims{Username: "anonymous", Admin: true}, nil
	}
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}

// requireAdmin asserts the current user is an admin (or auth is disabled).
func (a *GraphQLAdapter) requireAdmin(ctx context.Context) error {
	claims, err := a.requireAuthenticated(ctx)
	if err != nil {
		return err
	}
	if !a.IsAuthEnabled() {
		return nil
	}
	if !claims.Admin {
		return ErrAdminRequired
	}
	return nil
}

// =============================================================================
// Documents (queries + mutations)
// =============================================================================

func (a *GraphQLAdapter) GetDocument(ctx context.Context, collection, key, lang string, env map[string]string) (*gql.Document, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermRead)); err != nil {
		return nil, err
	}
	doc, err := a.mcp.Get(ctx, &MCPGetRequest{Collection: collection, Key: key, Lang: lang, Env: env})
	if err != nil {
		return nil, err
	}
	return mcpDocToGQL(doc), nil
}

func (a *GraphQLAdapter) SearchDocuments(ctx context.Context, input gql.SearchInput) (*gql.DocumentConnection, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, input.Collection, int(PermRead)); err != nil {
		return nil, err
	}
	req := &MCPSearchRequest{
		Collection: input.Collection,
		FilterMeta: gql.MapMetaInputToInternal(input.FilterMeta),
		Sort:       derefString(input.Sort),
		Asc:        derefBool(input.Asc, true),
		Limit:      derefInt(input.Limit, 100),
		Offset:     derefInt(input.Offset, 0),
	}
	resp, err := a.mcp.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	edges := make([]*gql.DocumentEdge, 0, len(resp.Documents))
	for i := range resp.Documents {
		d := resp.Documents[i]
		edges = append(edges, &gql.DocumentEdge{
			Cursor: fmt.Sprintf("%d", req.Offset+i),
			Node:   mcpDocToGQL(&d),
		})
	}
	hasNext := req.Offset+len(resp.Documents) < resp.Total
	hasPrev := req.Offset > 0
	var startCursor, endCursor *string
	if len(edges) > 0 {
		s := edges[0].Cursor
		e := edges[len(edges)-1].Cursor
		startCursor = &s
		endCursor = &e
	}
	return &gql.DocumentConnection{
		Edges: edges,
		PageInfo: &gql.PageInfo{
			HasNextPage:     hasNext,
			HasPreviousPage: hasPrev,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: resp.Total,
	}, nil
}

func (a *GraphQLAdapter) AddDocument(ctx context.Context, input gql.AddDocumentInput) (*gql.Document, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, input.Collection, int(PermWrite)); err != nil {
		return nil, err
	}
	doc, err := a.mcp.Add(ctx, &MCPAddRequest{
		Collection: input.Collection,
		Key:        input.Key,
		Lang:       input.Lang,
		Meta:       gql.MapMetaInputToInternal(input.Meta),
		ContentMD:  input.ContentMd,
	})
	if err != nil {
		return nil, err
	}
	if input.TTL != nil && *input.TTL > 0 {
		if _, err := a.mcp.SetTTL(ctx, &MCPSetTTLRequest{
			Collection: input.Collection, Key: input.Key, Lang: input.Lang, TTL: int64(*input.TTL),
		}); err != nil {
			return nil, fmt.Errorf("document added but TTL set failed: %w", err)
		}
	}
	return mcpDocToGQL(doc), nil
}

func (a *GraphQLAdapter) UpdateDocument(ctx context.Context, input gql.UpdateDocumentInput) (*gql.Document, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, input.Collection, int(PermWrite)); err != nil {
		return nil, err
	}
	req := &MCPUpdateDocumentRequest{
		Collection: input.Collection,
		Key:        input.Key,
		Lang:       input.Lang,
	}
	if input.Meta != nil {
		req.Meta = gql.MapMetaInputToInternal(input.Meta)
	}
	if input.ContentMd != nil {
		req.ContentMD = input.ContentMd
	}
	doc, err := a.mcp.UpdateDocument(ctx, req)
	if err != nil {
		return nil, err
	}
	return mcpDocToGQL(doc), nil
}

func (a *GraphQLAdapter) DeleteDocument(ctx context.Context, collection, key, lang string) error {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return err
	}
	return a.mcp.Delete(ctx, &MCPDeleteRequest{Collection: collection, Key: key, Lang: lang})
}

func (a *GraphQLAdapter) DeleteCollection(ctx context.Context, collection string) (int, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return 0, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermAdmin)); err != nil {
		return 0, err
	}
	resp, err := a.mcp.DeleteCollection(ctx, &MCPDeleteCollectionRequest{Collection: collection})
	if err != nil {
		return 0, err
	}
	return resp.Deleted, nil
}

func (a *GraphQLAdapter) AddBatch(ctx context.Context, collection string, docs []*gql.AddBatchDocumentInput) (*gql.BatchAddResult, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return nil, err
	}
	mcpDocs := make([]MCPBatchDocument, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		mcpDocs = append(mcpDocs, MCPBatchDocument{
			Key:          d.Key,
			Lang:         d.Lang,
			Meta:         gql.MapMetaInputToInternal(d.Meta),
			ContentMD:    d.ContentMd,
			SaveRevision: derefBool(d.SaveRevision, false),
		})
	}
	resp, err := a.mcp.AddBatch(ctx, &MCPAddBatchRequest{Collection: collection, Documents: mcpDocs})
	if err != nil {
		return nil, err
	}
	return &gql.BatchAddResult{
		Added:   resp.Added,
		Updated: resp.Updated,
		Failed:  resp.Failed,
		Errors:  resp.Errors,
	}, nil
}

func (a *GraphQLAdapter) IngestDocuments(ctx context.Context, collection string, docs []*gql.IngestDocumentInput, opts *gql.IngestOptionsInput) (*gql.IngestResult, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return nil, err
	}
	mcpDocs := make([]MCPIngestDocument, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		md := MCPIngestDocument{
			Lang:      d.Lang,
			ContentMD: d.ContentMd,
			Meta:      gql.MapMetaInputToInternal(d.Meta),
		}
		if d.URL != nil {
			md.URL = *d.URL
		}
		if d.Key != nil {
			md.Key = *d.Key
		}
		if d.ExtractFrontmatter != nil {
			md.ExtractFrontmatter = *d.ExtractFrontmatter
		}
		if d.ScrapedAt != nil {
			md.ScrapedAt = int64(*d.ScrapedAt)
		}
		if d.Scraper != nil {
			md.Scraper = *d.Scraper
		}
		if d.TTL != nil {
			md.TTL = int64(*d.TTL)
		}
		mcpDocs = append(mcpDocs, md)
	}
	mcpOpts := MCPIngestOptions{}
	if opts != nil {
		mcpOpts.SkipDuplicates = derefBool(opts.SkipDuplicates, false)
		mcpOpts.SkipEmbeddings = derefBool(opts.SkipEmbeddings, false)
		mcpOpts.SkipFTS = derefBool(opts.SkipFts, false)
		mcpOpts.SkipWebhooks = derefBool(opts.SkipWebhooks, false)
		mcpOpts.AutoConfigureCollection = derefBool(opts.AutoConfigureCollection, false)
		mcpOpts.SaveRevision = derefBool(opts.SaveRevision, false)
	}
	resp, err := a.mcp.Ingest(ctx, &MCPIngestRequest{
		Collection: collection,
		Documents:  mcpDocs,
		Options:    mcpOpts,
	})
	if err != nil {
		return nil, err
	}
	return &gql.IngestResult{
		Added:      resp.Added,
		Updated:    resp.Updated,
		Skipped:    resp.Skipped,
		Failed:     resp.Failed,
		Errors:     resp.Errors,
		Collection: collection,
		DurationMs: int(resp.DurationMs),
	}, nil
}

func (a *GraphQLAdapter) SetTTL(ctx context.Context, collection, key, lang string, ttl int) (*gql.Document, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return nil, err
	}
	doc, err := a.mcp.SetTTL(ctx, &MCPSetTTLRequest{Collection: collection, Key: key, Lang: lang, TTL: int64(ttl)})
	if err != nil {
		return nil, err
	}
	return mcpDocToGQL(doc), nil
}

func (a *GraphQLAdapter) ImportURL(ctx context.Context, collection, url string, key *string, lang string, meta []*gql.MetaInput, ttl *int) (*gql.Document, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return nil, err
	}
	req := &MCPImportURLRequest{
		Collection: collection,
		URL:        url,
		Lang:       lang,
		Meta:       gql.MapMetaInputToInternal(meta),
	}
	if key != nil {
		req.Key = *key
	}
	if ttl != nil {
		req.TTL = int64(*ttl)
	}
	doc, err := a.mcp.ImportURL(ctx, req)
	if err != nil {
		return nil, err
	}
	return mcpDocToGQL(doc), nil
}

// =============================================================================
// Vector / FTS / Stats
// =============================================================================

func (a *GraphQLAdapter) VectorSearch(ctx context.Context, input gql.VectorSearchInput) (*gql.VectorSearchResponse, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, input.Collection, int(PermRead)); err != nil {
		return nil, err
	}
	req := &MCPVectorSearchRequest{
		Collection:     input.Collection,
		FilterMeta:     gql.MapMetaInputToInternal(input.FilterMeta),
		TopK:           derefInt(input.TopK, 5),
		Threshold:      derefFloat64(input.Threshold, 0),
		IncludeContent: derefBool(input.IncludeContent, false),
	}
	if input.Query != nil {
		req.Query = *input.Query
	}
	if len(input.QueryVector) > 0 {
		req.QueryVector = make([]float32, len(input.QueryVector))
		for i, v := range input.QueryVector {
			req.QueryVector[i] = float32(v)
		}
	}
	resp, err := a.mcp.VectorSearch(ctx, req)
	if err != nil {
		return nil, err
	}
	results := make([]*gql.VectorSearchResult, 0, len(resp.Results))
	for i := range resp.Results {
		r := resp.Results[i]
		results = append(results, &gql.VectorSearchResult{
			Document: mcpDocToGQL(&r.Document),
			Score:    float64(r.Score),
			Rank:     r.Rank,
		})
	}
	model := resp.Model
	dims := resp.Dimensions
	return &gql.VectorSearchResponse{
		Results:    results,
		Total:      resp.Total,
		Model:      &model,
		Dimensions: &dims,
	}, nil
}

func (a *GraphQLAdapter) VectorStats(ctx context.Context) (*gql.VectorStats, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	resp, err := a.mcp.VectorStats(ctx)
	if err != nil {
		return nil, err
	}
	return mcpVectorStatsToGQL(resp), nil
}

func (a *GraphQLAdapter) VectorReindex(ctx context.Context, collection string, force *bool) (*gql.VectorStats, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermWrite)); err != nil {
		return nil, err
	}
	if _, err := a.mcp.VectorReindex(ctx, &MCPVectorReindexRequest{
		Collection: collection,
		Force:      derefBool(force, false),
	}); err != nil {
		return nil, err
	}
	stats, err := a.mcp.VectorStats(ctx)
	if err != nil {
		return nil, err
	}
	return mcpVectorStatsToGQL(stats), nil
}

func (a *GraphQLAdapter) FullTextSearch(ctx context.Context, input gql.FTSInput) (*gql.FTSResponse, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, input.Collection, int(PermRead)); err != nil {
		return nil, err
	}
	resp, err := a.mcp.FTSSearch(ctx, &MCPFTSSearchRequest{
		Collection: input.Collection,
		Query:      input.Query,
		Limit:      derefInt(input.Limit, 50),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*gql.FTSResult, 0, len(resp.Results))
	for i := range resp.Results {
		r := resp.Results[i]
		out = append(out, &gql.FTSResult{
			Document:     mcpDocToGQL(&r.Document),
			Score:        r.Score,
			MatchedTerms: r.MatchedTerms,
		})
	}
	return &gql.FTSResponse{Results: out, Total: resp.Total}, nil
}

func (a *GraphQLAdapter) GetStats(ctx context.Context) (*gql.Stats, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	stats, err := a.mcp.Stats(ctx)
	if err != nil {
		return nil, err
	}
	cols := make([]*gql.CollectionStats, 0, len(stats.Collections))
	for i := range stats.Collections {
		c := stats.Collections[i]
		cols = append(cols, &gql.CollectionStats{
			Name:           c.Name,
			DocumentCount:  c.DocumentCount,
			RevisionCount:  c.RevisionCount,
			MetaIndexCount: c.MetaIndexCount,
		})
	}
	return &gql.Stats{
		DatabasePath:     stats.DatabasePath,
		DatabaseSize:     int(stats.DatabaseSize),
		Mode:             stats.Mode,
		Collections:      cols,
		TotalDocuments:   stats.TotalDocuments,
		TotalRevisions:   stats.TotalRevisions,
		TotalMetaIndices: stats.TotalMetaIndices,
	}, nil
}

// =============================================================================
// Schema
// =============================================================================

func (a *GraphQLAdapter) GetSchema(ctx context.Context, collection string) (*gql.Schema, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	resp, err := a.mcp.GetSchema(ctx, collection)
	if err != nil {
		return nil, err
	}
	return &gql.Schema{Collection: resp.Collection, Schema: resp.Schema, Enabled: resp.Enabled}, nil
}

func (a *GraphQLAdapter) ListSchemas(ctx context.Context) ([]*gql.Schema, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	resp, err := a.mcp.ListSchemas(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*gql.Schema, 0, len(resp.Schemas))
	for i := range resp.Schemas {
		s := resp.Schemas[i]
		out = append(out, &gql.Schema{Collection: s.Collection, Schema: s.Schema, Enabled: true})
	}
	return out, nil
}

func (a *GraphQLAdapter) SetSchema(ctx context.Context, input gql.SetSchemaInput) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	return a.mcp.SetSchema(ctx, &MCPSetSchemaRequest{Collection: input.Collection, Schema: input.Schema})
}

func (a *GraphQLAdapter) DeleteSchema(ctx context.Context, collection string) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	return a.mcp.DeleteSchema(ctx, collection)
}

func (a *GraphQLAdapter) ValidateDocument(ctx context.Context, collection string, meta []*gql.MetaInput) (*gql.ValidationResult, error) {
	if _, err := a.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := a.CheckPermission(ctx, collection, int(PermRead)); err != nil {
		return nil, err
	}
	resp, err := a.mcp.ValidateDocument(ctx, &MCPValidateRequest{
		Collection: collection,
		Meta:       gql.MapMetaInputToInternal(meta),
	})
	if err != nil {
		return nil, err
	}
	return &gql.ValidationResult{Valid: resp.Valid, Errors: resp.Errors}, nil
}

// =============================================================================
// Webhooks
// =============================================================================

func (a *GraphQLAdapter) ListWebhooks(ctx context.Context, collection *string) ([]*gql.Webhook, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	all, err := a.mcp.ListWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*gql.Webhook, 0, len(all))
	for i := range all {
		w := all[i]
		if collection != nil && *collection != "" && w.Collection != *collection {
			continue
		}
		out = append(out, &gql.Webhook{
			ID:         w.ID,
			URL:        w.URL,
			Events:     w.Events,
			Collection: w.Collection,
			CreatedAt:  w.CreatedAt,
		})
	}
	return out, nil
}

func (a *GraphQLAdapter) RegisterWebhook(ctx context.Context, input gql.RegisterWebhookInput) (*gql.Webhook, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	w, err := a.mcp.RegisterWebhook(ctx, &MCPRegisterWebhookRequest{
		URL:        input.URL,
		Events:     input.Events,
		Collection: input.Collection,
	})
	if err != nil {
		return nil, err
	}
	return &gql.Webhook{
		ID:         w.ID,
		URL:        w.URL,
		Events:     w.Events,
		Collection: w.Collection,
		CreatedAt:  w.CreatedAt,
	}, nil
}

func (a *GraphQLAdapter) DeleteWebhook(ctx context.Context, id string) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	return a.mcp.DeleteWebhook(ctx, &MCPDeleteWebhookRequest{ID: id})
}

// =============================================================================
// Auth / Users / Groups / Permissions
// =============================================================================

func (a *GraphQLAdapter) Me(ctx context.Context) (*gql.User, error) {
	claims, err := a.requireAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return &gql.User{Username: claims.Username, Admin: claims.Admin, CreatedAt: 0}, nil
	}
	user, err := a.server.AuthManager.GetUser(claims.Username)
	if err != nil {
		return nil, err
	}
	return &gql.User{Username: user.Username, Admin: claims.Admin, CreatedAt: user.CreatedAt}, nil
}

func (a *GraphQLAdapter) ListUsers(ctx context.Context) ([]*gql.User, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return []*gql.User{}, nil
	}
	users := a.server.AuthManager.ListAllUsers()
	out := make([]*gql.User, 0, len(users))
	for _, u := range users {
		out = append(out, &gql.User{
			Username:  u.Username,
			Admin:     a.server.AuthManager.IsAdmin(u.Username),
			CreatedAt: u.CreatedAt,
		})
	}
	return out, nil
}

func (a *GraphQLAdapter) Register(ctx context.Context, username, password string) (*gql.User, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return nil, ErrAuthNotEnabled
	}
	user, err := a.server.AuthManager.CreateUser(username, password)
	if err != nil {
		return nil, err
	}
	return &gql.User{Username: user.Username, Admin: false, CreatedAt: user.CreatedAt}, nil
}

func (a *GraphQLAdapter) CreateAPIKey(ctx context.Context, input gql.CreateAPIKeyInput) (*gql.APIKey, error) {
	claims, err := a.requireAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return nil, ErrAuthNotEnabled
	}
	expiresAt := int64(0)
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}
	key, err := a.server.AuthManager.CreateAPIKey(claims.Username, input.Description, expiresAt)
	if err != nil {
		return nil, err
	}
	out := &gql.APIKey{
		Key:         key,
		Description: input.Description,
		CreatedAt:   time.Now().Unix(),
	}
	if expiresAt > 0 {
		out.ExpiresAt = &expiresAt
	}
	return out, nil
}

func (a *GraphQLAdapter) SetPermission(ctx context.Context, input gql.SetPermissionInput) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	if !a.IsAuthEnabled() {
		return ErrAuthNotEnabled
	}
	return a.server.AuthManager.SetPermission(&Permission{
		Username:   input.Username,
		Collection: input.Collection,
		Read:       input.Read,
		Write:      input.Write,
		Admin:      input.Admin,
	})
}

func (a *GraphQLAdapter) UserPermissionsList(ctx context.Context, username string) ([]*gql.UserPermission, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return []*gql.UserPermission{}, nil
	}
	perms := a.server.AuthManager.GetPermissions(username)
	out := make([]*gql.UserPermission, 0, len(perms))
	for _, p := range perms {
		out = append(out, &gql.UserPermission{
			Username:   p.Username,
			Collection: p.Collection,
			Read:       p.Read,
			Write:      p.Write,
			Admin:      p.Admin,
		})
	}
	return out, nil
}

func (a *GraphQLAdapter) ListGroups(ctx context.Context) ([]*gql.Group, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return []*gql.Group{}, nil
	}
	groups := a.server.AuthManager.ListGroups()
	out := make([]*gql.Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, &gql.Group{
			Name:        g.Name,
			Description: g.Description,
			Members:     g.Members,
			CreatedAt:   g.CreatedAt,
		})
	}
	return out, nil
}

func (a *GraphQLAdapter) GroupPermissionsList(ctx context.Context, groupName string) ([]*gql.GroupPermission, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return []*gql.GroupPermission{}, nil
	}
	perms := a.server.AuthManager.GetGroupPermissions(groupName)
	out := make([]*gql.GroupPermission, 0, len(perms))
	for _, p := range perms {
		out = append(out, &gql.GroupPermission{
			GroupName:  p.GroupName,
			Collection: p.Collection,
			Read:       p.Read,
			Write:      p.Write,
			Admin:      p.Admin,
		})
	}
	return out, nil
}

func (a *GraphQLAdapter) CreateGroup(ctx context.Context, input gql.CreateGroupInput) (*gql.Group, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return nil, ErrAuthNotEnabled
	}
	g, err := a.server.AuthManager.CreateGroup(input.Name, input.Description, input.Members)
	if err != nil {
		return nil, err
	}
	return &gql.Group{Name: g.Name, Description: g.Description, Members: g.Members, CreatedAt: g.CreatedAt}, nil
}

func (a *GraphQLAdapter) UpdateGroup(ctx context.Context, name, description string, members []string) (*gql.Group, error) {
	if err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !a.IsAuthEnabled() {
		return nil, ErrAuthNotEnabled
	}
	g, err := a.server.AuthManager.UpdateGroup(name, description, members)
	if err != nil {
		return nil, err
	}
	return &gql.Group{Name: g.Name, Description: g.Description, Members: g.Members, CreatedAt: g.CreatedAt}, nil
}

func (a *GraphQLAdapter) DeleteGroup(ctx context.Context, name string) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	if !a.IsAuthEnabled() {
		return ErrAuthNotEnabled
	}
	return a.server.AuthManager.DeleteGroup(name)
}

func (a *GraphQLAdapter) SetGroupPermission(ctx context.Context, input gql.SetGroupPermissionInput) error {
	if err := a.requireAdmin(ctx); err != nil {
		return err
	}
	if !a.IsAuthEnabled() {
		return ErrAuthNotEnabled
	}
	return a.server.AuthManager.SetGroupPermission(&GroupPermission{
		GroupName:  input.GroupName,
		Collection: input.Collection,
		Read:       input.Read,
		Write:      input.Write,
		Admin:      input.Admin,
	})
}

// =============================================================================
// Conversion helpers
// =============================================================================

// mcpDocToGQL converts an MCP document to the GraphQL Document type.
func mcpDocToGQL(d *MCPDocument) *gql.Document {
	if d == nil {
		return nil
	}
	out := &gql.Document{
		ID:        d.ID,
		Key:       d.Key,
		Lang:      d.Lang,
		Meta:      gql.MapMetaToGraphQL(d.Meta),
		ContentMd: d.ContentMD,
		AddedAt:   d.AddedAt.Unix(),
		UpdatedAt: d.UpdatedAt.Unix(),
	}
	if out.ID == "" {
		out.ID = fmt.Sprintf("%s|%s", d.Key, d.Lang)
	}
	return out
}

// mcpVectorStatsToGQL converts MCP vector stats response to the GraphQL type.
func mcpVectorStatsToGQL(s *MCPVectorStatsResponse) *gql.VectorStats {
	if s == nil {
		return &gql.VectorStats{Enabled: false, Collections: []*gql.VectorCollectionStats{}}
	}
	cols := make([]*gql.VectorCollectionStats, 0, len(s.Collections))
	for name, c := range s.Collections {
		cols = append(cols, &gql.VectorCollectionStats{
			Collection:        name,
			TotalDocuments:    c.TotalDocuments,
			EmbeddedDocuments: c.EmbeddedDocuments,
		})
	}
	provider := s.Provider
	model := s.Model
	dims := s.Dimensions
	return &gql.VectorStats{
		Enabled:     s.Enabled,
		Provider:    &provider,
		Model:       &model,
		Dimensions:  &dims,
		Collections: cols,
		IndexReady:  s.Enabled,
	}
}

// derefString returns *p or "" if nil.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefBool returns *p or fallback if nil.
func derefBool(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

// derefInt returns *p or fallback if nil.
func derefInt(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}

// derefFloat64 returns *p or fallback if nil.
func derefFloat64(p *float64, fallback float64) float64 {
	if p == nil {
		return fallback
	}
	return *p
}
