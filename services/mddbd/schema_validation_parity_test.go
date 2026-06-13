package main

import (
	"context"
	"testing"

	pb "mddb/proto"
)

const strictSchema = `{"required":["author"],"properties":{"author":{"type":"string"}}}`

// TestAddDocument_EnforcesSchemaAndRequiredFields — GO-003: validation lives in
// the single write path (addDocument), so MCP (DirectClient), GraphQL and every
// internal caller are covered, not just gRPC Add / HTTP handleAdd.
func TestAddDocument_EnforcesSchemaAndRequiredFields(t *testing.T) {
	_, s, cleanup := newTestGRPCServer(t)
	defer cleanup()

	if err := s.SchemaManager.Set("strict", strictSchema); err != nil {
		t.Fatal(err)
	}

	// Missing required meta -> rejected.
	if _, _, err := s.addDocument("strict", "k", "en", map[string][]string{}, "x", 0, true); err == nil {
		t.Error("expected schema validation error for missing 'author'")
	}
	// Required meta present -> accepted.
	if _, _, err := s.addDocument("strict", "k", "en", map[string][]string{"author": {"alice"}}, "x", 0, true); err != nil {
		t.Errorf("valid doc rejected: %v", err)
	}
	// Missing required field (empty key) -> rejected.
	if _, _, err := s.addDocument("strict", "", "en", map[string][]string{"author": {"a"}}, "x", 0, true); err == nil {
		t.Error("expected error for empty key")
	}
}

// TestBatch_EnforcesSchema — GO-003: the batch path validated only key/lang and
// skipped schema validation; it now enforces the schema per document.
func TestBatch_EnforcesSchema(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()

	if err := s.SchemaManager.Set("strict", strictSchema); err != nil {
		t.Fatal(err)
	}

	resp, err := gs.AddBatch(context.Background(), &pb.AddBatchRequest{
		Collection: "strict",
		Documents: []*pb.BatchDocument{
			{Key: "ok", Lang: "en", ContentMd: "x", Meta: map[string]*pb.MetaValues{"author": {Values: []string{"a"}}}},
			{Key: "bad", Lang: "en", ContentMd: "x"}, // missing required "author"
		},
	})
	if err != nil {
		t.Fatalf("AddBatch: %v", err)
	}
	if resp.Added != 1 || resp.Failed != 1 {
		t.Errorf("added=%d failed=%d, want 1 and 1 (schema enforced in batch)", resp.Added, resp.Failed)
	}
}
