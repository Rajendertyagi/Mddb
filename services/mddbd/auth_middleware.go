package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Public endpoints that don't require authentication
var publicEndpoints = map[string]bool{
	"/health":        true,
	"/v1/health":     true,
	"/v1/auth/login": true,
	"/metrics":       true, // configurable later
}

// HTTPMiddleware wraps HTTP handlers with authentication
func (am *AuthManager) HTTPMiddleware(next http.Handler) http.Handler {
	if !am.enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public endpoints
		if isPublicEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Try to extract token
		token := extractTokenFromRequest(r)

		// If no bearer token, try API key
		if token == "" {
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				username, err := am.ValidateAPIKey(apiKey)
				if err != nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}

				// Generate short-lived JWT from API key
				isAdmin := am.IsAdmin(username)
				token, err = GenerateJWT(username, isAdmin, am.config.JWTSecret, 1*3600*time.Second) // 1h
				if err != nil {
					http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
					return
				}
			}
		}

		// No token found
		if token == "" {
			http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
			return
		}

		// Validate JWT
		claims, err := ValidateJWT(token, am.config.JWTSecret)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Check if user still exists and is not disabled
		user, err := am.GetUser(claims.Username)
		if err != nil || user.Disabled {
			http.Error(w, `{"error":"user disabled or not found"}`, http.StatusUnauthorized)
			return
		}

		// Inject claims into context
		ctx := context.WithValue(r.Context(), authContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractTokenFromRequest extracts JWT token from Authorization header
func extractTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Bearer token format: "Bearer <token>"
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}

	return parts[1]
}

// isPublicEndpoint checks if endpoint is public (no auth required)
func isPublicEndpoint(path string) bool {
	return publicEndpoints[path]
}
