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
					am.auditAuth(r, "", "auth.apikey", "fail", err.Error())
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}

				// Generate short-lived JWT from API key
				isAdmin := am.IsAdmin(username)
				token, err = GenerateJWT(username, isAdmin, am.config.JWTSecret, 1*3600*time.Second) // 1h
				if err != nil {
					am.auditAuth(r, username, "auth.apikey", "fail", "jwt generation failed")
					http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
					return
				}
				am.auditAuth(r, username, "auth.apikey", "ok", "")
			}
		}

		// No token found
		if token == "" {
			am.auditAuth(r, "", "auth.missing", "fail", "")
			http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
			return
		}

		// Validate JWT
		claims, err := ValidateJWT(token, am.config.JWTSecret)
		if err != nil {
			am.auditAuth(r, "", "auth.jwt", "fail", "invalid token")
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Check if user still exists and is not disabled.
		// Return identical response for both cases to avoid
		// leaking user existence (timing/enumeration side-channel).
		user, err := am.GetUser(claims.Username)
		if err != nil || user.Disabled {
			am.auditAuth(r, claims.Username, "auth.jwt", "fail", "user disabled or not found")
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Inject claims into context
		ctx := context.WithValue(r.Context(), authContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// auditAuth records an authentication attempt. Safe on nil server.
// When the attempt failed, also feeds the sliding-window tracker so
// a burst of failures from the same actor/IP fires the security
// incident event.
func (am *AuthManager) auditAuth(r *http.Request, actor, action, result, detail string) {
	if am == nil || am.server == nil {
		return
	}
	ip := ClientIP(r)
	if am.server.AuditManager != nil {
		am.server.AuditManager.Record(AuditEvent{
			Actor:     actor,
			Action:    action,
			Resource:  r.URL.Path,
			Result:    result,
			IP:        ip,
			UserAgent: r.UserAgent(),
			Detail:    detail,
		})
	}
	if result == "fail" && am.server.AuthFailureTracker != nil {
		am.server.AuthFailureTracker.Record(actor, ip)
	}
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
