package main

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCUnaryInterceptor returns a gRPC unary interceptor for authentication
func (am *AuthManager) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !am.enabled {
			return handler(ctx, req)
		}

		// Extract metadata from context
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		// Try to extract token from authorization metadata
		var token string
		if values := md.Get("authorization"); len(values) > 0 {
			auth := values[0]
			// Support both "Bearer <token>" and raw token
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
				token = strings.TrimPrefix(token, "bearer ")
			} else {
				token = auth
			}
		}

		// If no bearer token, try x-api-key
		if token == "" {
			if values := md.Get("x-api-key"); len(values) > 0 {
				apiKey := values[0]
				username, err := am.ValidateAPIKey(apiKey)
				if err != nil {
					return nil, status.Error(codes.Unauthenticated, "invalid api key")
				}

				// Generate short-lived JWT from API key
				isAdmin := am.IsAdmin(username)
				token, err = GenerateJWT(username, isAdmin, am.config.JWTSecret, 1*time.Hour)
				if err != nil {
					return nil, status.Error(codes.Internal, "failed to generate token")
				}
			}
		}

		// No token found
		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "missing authentication")
		}

		// Validate JWT
		claims, err := ValidateJWT(token, am.config.JWTSecret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		// Check if user still exists and is not disabled
		user, err := am.GetUser(claims.Username)
		if err != nil || user.Disabled {
			return nil, status.Error(codes.Unauthenticated, "user disabled or not found")
		}

		// Inject claims into context
		ctx = context.WithValue(ctx, authContextKey, claims)

		// Call handler with authenticated context
		return handler(ctx, req)
	}
}
