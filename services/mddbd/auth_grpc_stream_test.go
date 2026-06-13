package main

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// authMockServerStream is a minimal grpc.ServerStream whose Context() returns a
// configurable value; only Context() is exercised by the interceptor tests.
type authMockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *authMockServerStream) Context() context.Context { return m.ctx }

// streamDummyHandler succeeds and records the context it was invoked with so
// tests can assert claims were injected (SEC-003).
func streamDummyHandler(captured *context.Context) grpc.StreamHandler {
	return func(srv interface{}, ss grpc.ServerStream) error {
		if captured != nil {
			*captured = ss.Context()
		}
		return nil
	}
}

func runStream(am *AuthManager, ctx context.Context, captured *context.Context) error {
	interceptor := am.GRPCStreamInterceptor()
	return interceptor(
		nil,
		&authMockServerStream{ctx: ctx},
		&grpc.StreamServerInfo{FullMethod: "/mddb.MDDB/Export"},
		streamDummyHandler(captured),
	)
}

func TestGRPCStreamInterceptor_Disabled(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()
	am.enabled = false

	if err := runStream(am, context.Background(), nil); err != nil {
		t.Fatalf("expected no error when auth disabled, got: %v", err)
	}
}

func TestGRPCStreamInterceptor_NoMetadata(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()

	err := runStream(am, context.Background(), nil) // no metadata
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", st.Code())
	}
}

func TestGRPCStreamInterceptor_MissingAuth(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()

	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	err := runStream(am, ctx, nil)
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", st.Code())
	}
}

func TestGRPCStreamInterceptor_InvalidToken(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()

	md := metadata.Pairs("authorization", "Bearer not-a-jwt")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	err := runStream(am, ctx, nil)
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", st.Code())
	}
}

// TestGRPCStreamInterceptor_ValidBearerInjectsClaims is the core SEC-003 check:
// a valid token authenticates the stream AND the claims are visible via
// stream.Context() (so Export's CheckPermission no longer fail-closes).
func TestGRPCStreamInterceptor_ValidBearerInjectsClaims(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()

	token, err := GenerateJWT("admin", true, am.config.JWTSecret, am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var handlerCtx context.Context
	if err := runStream(am, ctx, &handlerCtx); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	claims, ok := GetClaimsFromContext(handlerCtx)
	if !ok {
		t.Fatal("claims not present in stream context — SEC-003 regression")
	}
	if claims.Username != "admin" || !claims.Admin {
		t.Errorf("claims = %+v, want admin/true", claims)
	}
}

func TestGRPCStreamInterceptor_ValidAPIKey(t *testing.T) {
	am, cleanup := authGrpcSetup(t)
	defer cleanup()

	apiKey, err := am.CreateAPIKey("admin", "stream key", 0)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	md := metadata.Pairs("x-api-key", apiKey)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	if err := runStream(am, ctx, nil); err != nil {
		t.Fatalf("expected success for valid API key, got: %v", err)
	}
}
