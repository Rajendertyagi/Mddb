package graphql

import (
	"context"
)

//go:generate go run github.com/99designs/gqlgen generate

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// ServerInterface defines the interface that the main Server must implement
// This allows the graphql package to interact with the Server without importing main
type ServerInterface interface {
	// Auth methods
	GetClaimsFromContext(ctx context.Context) (Claims, bool)
	CheckPermission(ctx context.Context, collection string, perm int) error
	Authenticate(username, password string) (UserInfo, error)
	GenerateJWT(username string, isAdmin bool) (string, int64, error)

	// Document methods (to be implemented by adapter)
	GetDocument(ctx context.Context, collection, key, lang string, env map[string]string) (interface{}, error)
	SearchDocuments(ctx context.Context, collection string, filterMeta map[string][]string, sort string, asc bool, limit, offset int) (interface{}, error)
	AddDocument(ctx context.Context, collection, key, lang string, meta map[string][]string, contentMd string, ttl int64) (interface{}, bool, error)
	DeleteDocument(ctx context.Context, collection, key, lang string) error

	// Stats and search
	GetStats(ctx context.Context) (interface{}, error)
	VectorSearch(ctx context.Context, collection, query string, queryVector []float32, topK int, threshold float64, filterMeta map[string][]string, includeContent bool) (interface{}, error)
}

// Claims represents JWT token claims
type Claims struct {
	Username string
	Admin    bool
}

// UserInfo represents user information
type UserInfo struct {
	Username  string
	Admin     bool
	CreatedAt int64
}

type Resolver struct {
	server ServerInterface
}

// NewResolver creates a new root GraphQL resolver
func NewResolver(server ServerInterface) *Resolver {
	return &Resolver{server: server}
}
