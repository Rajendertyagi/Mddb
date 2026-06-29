package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mddb/proto"
)

// GO-002: the cache.DocumentCache (5-min TTL) feeds the gRPC Get path. Writes and
// deletes through the shared Server.addDocument / deleteDocumentInternal (used
// by HTTP/MCP/GraphQL) must keep that cache coherent, otherwise gRPC Get serves
// stale or already-deleted documents for up to the TTL.

// TestGRPCGet_CacheInvalidatedOnDelete — delete via the HTTP path must make a
// subsequent gRPC Get return NotFound immediately, not a stale cached hit.
func TestGRPCGet_CacheInvalidatedOnDelete(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := s.addDocument("blog", "c1", "en", nil, "# Hello", 0, true); err != nil {
		t.Fatalf("addDocument: %v", err)
	}
	// Prime the read cache via a gRPC Get.
	if _, err := gs.Get(ctx, &pb.GetRequest{Collection: "blog", Key: "c1", Lang: "en"}); err != nil {
		t.Fatalf("first Get: %v", err)
	}

	if err := s.deleteDocumentInternal("blog", "c1", "en"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := gs.Get(ctx, &pb.GetRequest{Collection: "blog", Key: "c1", Lang: "en"})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Errorf("after delete, gRPC Get returned %v, want NotFound (stale cache — GO-002)", err)
	}
}

// TestGRPCGet_CacheRefreshedOnUpdate — an update via the HTTP path must be
// visible to a subsequent gRPC Get, not the previous cached body.
func TestGRPCGet_CacheRefreshedOnUpdate(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := s.addDocument("blog", "c2", "en", nil, "# v1", 0, true); err != nil {
		t.Fatalf("addDocument v1: %v", err)
	}
	// Prime the cache with v1.
	if _, err := gs.Get(ctx, &pb.GetRequest{Collection: "blog", Key: "c2", Lang: "en"}); err != nil {
		t.Fatalf("Get v1: %v", err)
	}

	if _, _, err := s.addDocument("blog", "c2", "en", nil, "# v2", 0, true); err != nil {
		t.Fatalf("addDocument v2: %v", err)
	}

	doc, err := gs.Get(ctx, &pb.GetRequest{Collection: "blog", Key: "c2", Lang: "en"})
	if err != nil {
		t.Fatalf("Get v2: %v", err)
	}
	if doc.ContentMd != "# v2" {
		t.Errorf("after update, gRPC Get content = %q, want %q (stale cache — GO-002)", doc.ContentMd, "# v2")
	}
}
