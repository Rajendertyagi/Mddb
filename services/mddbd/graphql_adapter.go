package main

import (
	"context"
	"errors"
	"time"

	gql "mddb/graphql"
)

var (
	ErrAuthNotEnabled         = errors.New("authentication not enabled")
	ErrVectorSearchNotEnabled = errors.New("vector search not enabled")
	ErrInvalidRequest         = errors.New("invalid request")
)

// SimpleGraphQLAdapter is a simplified adapter
type SimpleGraphQLAdapter struct {
	server *Server
}

// NewGraphQLServerAdapter creates adapter
func NewGraphQLServerAdapter(s *Server) gql.ServerInterface {
	return &SimpleGraphQLAdapter{server: s}
}

// GetClaimsFromContext retrieves claims
func (a *SimpleGraphQLAdapter) GetClaimsFromContext(ctx context.Context) (gql.Claims, bool) {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return gql.Claims{}, false
	}
	return gql.Claims{
		Username: claims.Username,
		Admin:    claims.Admin,
	}, true
}

// CheckPermission checks permissions
func (a *SimpleGraphQLAdapter) CheckPermission(ctx context.Context, collection string, perm int) error {
	if a.server.AuthManager == nil {
		return nil
	}
	return a.server.AuthManager.CheckPermission(ctx, collection, PermissionType(perm))
}

// Authenticate validates credentials
func (a *SimpleGraphQLAdapter) Authenticate(username, password string) (gql.UserInfo, error) {
	if a.server.AuthManager == nil {
		return gql.UserInfo{}, ErrAuthNotEnabled
	}

	user, err := a.server.AuthManager.Authenticate(username, password)
	if err != nil {
		return gql.UserInfo{}, err
	}

	isAdmin := a.server.AuthManager.IsAdmin(username)

	return gql.UserInfo{
		Username:  user.Username,
		Admin:     isAdmin,
		CreatedAt: user.CreatedAt,
	}, nil
}

// GenerateJWT creates JWT token
func (a *SimpleGraphQLAdapter) GenerateJWT(username string, isAdmin bool) (string, int64, error) {
	if a.server.AuthManager == nil {
		return "", 0, ErrAuthNotEnabled
	}

	expiry := a.server.AuthManager.config.JWTExpiry
	token, err := GenerateJWT(username, isAdmin, a.server.AuthManager.config.JWTSecret, expiry)
	if err != nil {
		return "", 0, err
	}

	expiresAt := time.Now().Add(expiry).Unix()
	return token, expiresAt, nil
}

// Stub implementations for other methods - return not implemented for now
func (a *SimpleGraphQLAdapter) GetDocument(ctx context.Context, collection, key, lang string, env map[string]string) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

func (a *SimpleGraphQLAdapter) SearchDocuments(ctx context.Context, collection string, filterMeta map[string][]string, sort string, asc bool, limit, offset int) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

func (a *SimpleGraphQLAdapter) AddDocument(ctx context.Context, collection, key, lang string, meta map[string][]string, contentMd string, ttl int64) (interface{}, bool, error) {
	return nil, false, errors.New("not yet implemented - use REST API")
}

func (a *SimpleGraphQLAdapter) DeleteDocument(ctx context.Context, collection, key, lang string) error {
	return errors.New("not yet implemented - use REST API")
}

func (a *SimpleGraphQLAdapter) GetStats(ctx context.Context) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

func (a *SimpleGraphQLAdapter) VectorSearch(ctx context.Context, collection, query string, queryVector []float32, topK int, threshold float64, filterMeta map[string][]string, includeContent bool) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}
