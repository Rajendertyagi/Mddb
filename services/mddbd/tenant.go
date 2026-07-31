package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Multi-tenancy model (namespace isolation):
//
// A tenant is a namespace over collections. A user assigned to tenant "acme"
// can only touch collections named "acme/<something>" — the gate is enforced
// centrally in AuthManager.CheckPermission, which every handler already calls,
// so HTTP, gRPC, GraphQL and MCP inherit the isolation from one choke point.
//
// Users with an empty Tenant are global users and behave exactly as before
// multi-tenancy existed (100% backward compatible). Tenant users never carry
// the global admin flag in their JWT: admin-only endpoints check claims.Admin,
// and granting that to a namespaced user would leak cross-tenant control.
// Within their namespace, a wildcard ("*") permission gives a tenant user
// full access to every collection of that tenant — that is the "tenant admin".

// TenantSeparator splits the tenant namespace from the collection name.
const TenantSeparator = "/"

// tenantNameRe validates tenant identifiers: 1-64 chars, alphanumerics,
// dash and underscore. The separator character is intentionally excluded.
var tenantNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ErrInvalidTenant is returned when a tenant identifier fails validation.
var ErrInvalidTenant = errors.New("invalid tenant name")

// ValidTenantName reports whether name is a well-formed tenant identifier.
// The empty string is valid: it means "global user, no tenant".
func ValidTenantName(name string) bool {
	if name == "" {
		return true
	}
	return tenantNameRe.MatchString(name)
}

// CollectionInTenant reports whether collection belongs to the tenant's
// namespace. An empty tenant owns everything (global scope).
func CollectionInTenant(tenant, collection string) bool {
	if tenant == "" {
		return true
	}
	return strings.HasPrefix(collection, tenant+TenantSeparator) &&
		len(collection) > len(tenant)+len(TenantSeparator)
}

// TenantFromContext returns the tenant of the authenticated caller, or ""
// for global users and unauthenticated (auth-disabled) requests.
func TenantFromContext(ctx context.Context) string {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return ""
	}
	return claims.Tenant
}
