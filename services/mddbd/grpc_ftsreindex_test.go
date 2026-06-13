package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mddb/proto"
)

// TestGRPCFTSReindex_ReadOnlyDenied — GO-009: FTSReindex rewrites the FTS index,
// so it must be refused in read-only mode like every other mutating RPC.
func TestGRPCFTSReindex_ReadOnlyDenied(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()

	s.Mode = ModeRead
	_, err := gs.FTSReindex(context.Background(), &pb.FTSReindexRequest{Collection: "blog"})
	if st, _ := status.FromError(err); st.Code() != codes.PermissionDenied {
		t.Errorf("read-only FTSReindex: got %v, want PermissionDenied", err)
	}
}

// TestGRPCFTSReindex_OkOnWritable — happy path: with content present, reindex
// reports Status "ok" and counts the document.
func TestGRPCFTSReindex_OkOnWritable(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	ctx := context.Background()

	addDocViaGRPC(t, gs, "blog", "r1", "en", "alpha beta gamma", nil)

	resp, err := gs.FTSReindex(ctx, &pb.FTSReindexRequest{Collection: "blog"})
	if err != nil {
		t.Fatalf("FTSReindex: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Reindexed < 1 {
		t.Errorf("reindexed = %d, want >= 1", resp.Reindexed)
	}
}
