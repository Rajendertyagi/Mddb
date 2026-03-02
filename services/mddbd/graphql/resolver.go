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

	// Add more methods as needed by resolvers
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
