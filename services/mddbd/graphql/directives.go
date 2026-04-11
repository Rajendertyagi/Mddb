package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
)

// Directive implementations.
//
// In MDDB the @auth and @hasRole directives are intentionally pass-through.
// All authentication and authorization is enforced inside resolver bodies via
// `r.server.GetClaimsFromContext` and `r.server.CheckPermission`. This keeps
// the auth contract in one place (the adapter) and avoids the well-known
// gqlgen/Go context-key gotcha where typed and string keys do not match.
//
// If a request reaches GraphQL without auth, the adapter still rejects every
// data operation via CheckPermission / IsAuthEnabled.

// AuthDirective is a no-op pass-through; resolvers enforce auth themselves.
func AuthDirective(ctx context.Context, _ interface{}, next graphql.Resolver) (interface{}, error) {
	return next(ctx)
}

// HasRoleDirective is a no-op pass-through; resolvers enforce roles themselves.
func HasRoleDirective(ctx context.Context, _ interface{}, next graphql.Resolver, _ Role) (interface{}, error) {
	return next(ctx)
}
