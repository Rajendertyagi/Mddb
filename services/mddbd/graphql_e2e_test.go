package main

// End-to-end coverage for the GraphQL adapter (services/mddbd/graphql_adapter.go).
//
// These tests instantiate a real Server with a temporary BoltDB (via the
// existing newHandlerTestServer helper), wrap it in the adapter that the
// graphql package consumes, and exercise queries / mutations through the
// public ServerInterface contract — the same code path the production
// /graphql HTTP handler hits.
//
// The goal is to prove that the new resolvers actually work end-to-end on a
// real BoltDB, not just that the package compiles. Auth is left disabled
// (s.AuthManager == nil) so the adapter's permission checks short-circuit
// to allow-all, which mirrors a fresh-out-of-the-box mddbd deployment.

import (
	"context"
	"strings"
	"testing"

	gql "mddb/graphql"
)

// gqlAdapterFromServer wires up the adapter against an in-memory test Server.
func gqlAdapterFromServer(t *testing.T) (*GraphQLAdapter, func()) {
	t.Helper()
	s, cleanup := newHandlerTestServer(t)
	a := NewGraphQLServerAdapter(s).(*GraphQLAdapter)
	return a, cleanup
}

func TestGraphQLE2E_AddDocumentThenGet(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()
	ctx := context.Background()

	// Mutation: addDocument
	added, err := a.AddDocument(ctx, gql.AddDocumentInput{
		Collection: "blog",
		Key:        "hello",
		Lang:       "en_US",
		ContentMd:  "# Hello\n\nFirst post.",
		Meta: []*gql.MetaInput{
			{Key: "tags", Values: []string{"intro", "test"}},
			{Key: "author", Values: []string{"alice"}},
		},
	})
	if err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	if added == nil || added.Key != "hello" || added.Lang != "en_US" {
		t.Fatalf("AddDocument returned wrong document: %+v", added)
	}
	if !strings.Contains(added.ContentMd, "Hello") {
		t.Errorf("expected content to contain 'Hello', got %q", added.ContentMd)
	}

	// Query: document
	got, err := a.GetDocument(ctx, "blog", "hello", "en_US", nil)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got == nil {
		t.Fatal("GetDocument returned nil")
	}
	if got.Key != "hello" || got.Lang != "en_US" {
		t.Errorf("GetDocument key/lang mismatch: %+v", got)
	}
	if !strings.Contains(got.ContentMd, "First post") {
		t.Errorf("expected fetched content to contain 'First post', got %q", got.ContentMd)
	}
}

func TestGraphQLE2E_SearchPagination(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()
	ctx := context.Background()

	// Seed three documents.
	for _, k := range []string{"a", "b", "c"} {
		if _, err := a.AddDocument(ctx, gql.AddDocumentInput{
			Collection: "notes",
			Key:        k,
			Lang:       "en_US",
			ContentMd:  "# " + k,
		}); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}

	limit := 2
	conn, err := a.SearchDocuments(ctx, gql.SearchInput{
		Collection: "notes",
		Limit:      &limit,
	})
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if conn == nil {
		t.Fatal("SearchDocuments returned nil")
	}
	if conn.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", conn.TotalCount)
	}
	if len(conn.Edges) != 2 {
		t.Errorf("Edges len = %d, want 2 (limit)", len(conn.Edges))
	}
	if conn.PageInfo == nil || !conn.PageInfo.HasNextPage {
		t.Errorf("PageInfo.HasNextPage should be true (3 docs total, page size 2)")
	}
}

func TestGraphQLE2E_DeleteDocument(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := a.AddDocument(ctx, gql.AddDocumentInput{
		Collection: "tmp",
		Key:        "soon-gone",
		Lang:       "en_US",
		ContentMd:  "# About to vanish",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := a.DeleteDocument(ctx, "tmp", "soon-gone", "en_US"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	// Subsequent get should fail (document gone).
	if _, err := a.GetDocument(ctx, "tmp", "soon-gone", "en_US", nil); err == nil {
		t.Error("expected error fetching deleted document, got nil")
	}
}

func TestGraphQLE2E_StatsContainsCollections(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := a.AddDocument(ctx, gql.AddDocumentInput{
		Collection: "stats-target",
		Key:        "k",
		Lang:       "en_US",
		ContentMd:  "# stats test",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stats, err := a.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	if stats.TotalDocuments < 1 {
		t.Errorf("TotalDocuments = %d, want >= 1", stats.TotalDocuments)
	}
	found := false
	for _, c := range stats.Collections {
		if c.Name == "stats-target" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'stats-target' collection in Stats.Collections")
	}
}

func TestGraphQLE2E_LoginWithoutAuthEnabled(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()

	// AuthManager is nil in newHandlerTestServer; Authenticate must surface
	// the not-enabled sentinel rather than panicking.
	_, err := a.Authenticate("admin", "secret")
	if err == nil {
		t.Fatal("expected ErrAuthNotEnabled when AuthManager is nil")
	}
	if err != ErrAuthNotEnabled {
		t.Errorf("got %v, want ErrAuthNotEnabled", err)
	}
}

func TestGraphQLE2E_NoPanicOnAllResolvers(t *testing.T) {
	a, cleanup := gqlAdapterFromServer(t)
	defer cleanup()
	ctx := context.Background()

	// Smoke-call every read-only adapter method to confirm none of them
	// panic on a freshly-initialised, empty Server. This is the regression
	// guard against the prior state where 29/32 resolvers panicked
	// `not implemented`. Errors are tolerated — empty BoltDB legitimately
	// returns "not found" / nil for many of these — only panics fail the test.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic from a resolver path: %v", r)
		}
	}()

	_, _ = a.GetStats(ctx)
	_, _ = a.VectorStats(ctx)
	_, _ = a.ListSchemas(ctx)
	_, _ = a.ListWebhooks(ctx, nil)
	_, _ = a.ListUsers(ctx)
	_, _ = a.ListGroups(ctx)
	_, _ = a.SearchDocuments(ctx, gql.SearchInput{Collection: "missing"})
	_, _ = a.GetDocument(ctx, "missing", "missing", "en_US", nil)
	_, _ = a.GetSchema(ctx, "missing")
}
