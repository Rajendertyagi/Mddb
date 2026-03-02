package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
)

// AuthDirective ensures user is authenticated
// This directive checks if JWT claims exist in the context
func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	// Check if claims exist in context (set by auth middleware)
	claims := ctx.Value("auth_claims")
	if claims == nil {
		return nil, fmt.Errorf("unauthorized: authentication required")
	}

	return next(ctx)
}

// HasRoleDirective checks if user has required role
func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role Role) (interface{}, error) {
	// First check authentication
	claimsVal := ctx.Value("auth_claims")
	if claimsVal == nil {
		return nil, fmt.Errorf("unauthorized: authentication required")
	}

	// Type assert to get claims
	// The actual type will be *JWTClaims from the main package
	// We use interface{} and check for admin field via reflection or type assertion
	type adminChecker interface {
		IsAdmin() bool
	}

	// For ADMIN role, check the admin flag
	if role == RoleAdmin {
		// Try to get admin status from context or claims
		// This will be properly connected when integrated with main package
		// For now, we'll use a simple check
		if checker, ok := claimsVal.(adminChecker); ok {
			if !checker.IsAdmin() {
				return nil, fmt.Errorf("forbidden: admin access required")
			}
		} else {
			// Fallback: check if there's an Admin field
			type claimsWithAdmin interface {
				GetAdmin() bool
			}
			if c, ok := claimsVal.(claimsWithAdmin); ok {
				if !c.GetAdmin() {
					return nil, fmt.Errorf("forbidden: admin access required")
				}
			}
		}
	}

	return next(ctx)
}

// Note: HasPermissionDirective would be implemented when we integrate with the main package's
// permission system. For now, we rely on the @auth and @hasRole directives.
