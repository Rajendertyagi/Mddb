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

// authenticateContext validates the JWT / API key carried in the incoming gRPC
// metadata and returns a context with the resolved claims injected. It is the
// single source of truth shared by the unary and stream interceptors (SEC-003,
// DRY) so streaming RPCs cannot bypass the auth that unary RPCs enforce.
func (am *AuthManager) authenticateContext(ctx context.Context) (context.Context, error) {
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
	return context.WithValue(ctx, authContextKey, claims), nil
}

// GRPCUnaryInterceptor returns a gRPC unary interceptor for authentication.
func (am *AuthManager) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !am.enabled {
			return handler(ctx, req)
		}
		authCtx, err := am.authenticateContext(ctx)
		if err != nil {
			return nil, err
		}
		return handler(authCtx, req)
	}
}

// authServerStream wraps a grpc.ServerStream so the authenticated context
// (with injected claims) is what handlers and CheckPermission see via
// stream.Context().
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *authServerStream) Context() context.Context { return w.ctx }

// GRPCStreamInterceptor returns a gRPC stream interceptor for authentication.
//
// SEC-003: without this, every server-streaming RPC (MDDB.Export, the whole
// MDDBReplication service) bypassed authentication entirely — and Export, which
// calls CheckPermission(stream.Context(), ...), always failed because no claims
// were ever injected into the stream context. This authenticates the stream and
// forwards a context carrying the claims, fixing both the security hole and the
// fail-closed Export bug.
func (am *AuthManager) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !am.enabled {
			return handler(srv, ss)
		}
		authCtx, err := am.authenticateContext(ss.Context())
		if err != nil {
			return err
		}
		return handler(srv, &authServerStream{ServerStream: ss, ctx: authCtx})
	}
}
