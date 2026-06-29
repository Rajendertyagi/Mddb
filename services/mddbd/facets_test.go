package main

import (
	"mddb/internal/storage"
	"testing"
)

func TestComputeFacets_Empty(t *testing.T) {
	got := computeFacets(nil, []string{"x"}, 0)
	if got != nil {
		t.Fatalf("expected nil result for empty docs, got %v", got)
	}
	got = computeFacets([]storage.Doc{{ID: "a"}}, nil, 0)
	if got != nil {
		t.Fatalf("expected nil result for empty facetBy, got %v", got)
	}
}

func TestComputeFacets_CountsAndOrdering(t *testing.T) {
	docs := []storage.Doc{
		{ID: "1", Meta: map[string][]string{"category": {"tech"}, "lang": {"en"}}},
		{ID: "2", Meta: map[string][]string{"category": {"tech"}, "lang": {"en"}}},
		{ID: "3", Meta: map[string][]string{"category": {"blog"}, "lang": {"pl"}}},
		{ID: "4", Meta: map[string][]string{"category": {"tech", "blog"}, "lang": {"en"}}},
	}
	got := computeFacets(docs, []string{"category", "lang"}, 0)
	if got == nil {
		t.Fatal("expected non-nil facet result")
	}

	catBuckets := got["category"]
	if len(catBuckets) != 2 {
		t.Fatalf("expected 2 category buckets, got %d", len(catBuckets))
	}
	// tech: docs 1,2,4 → 3; blog: docs 3,4 → 2
	if catBuckets[0].Value != "tech" || catBuckets[0].Count != 3 {
		t.Errorf("expected tech=3 first, got %+v", catBuckets[0])
	}
	if catBuckets[1].Value != "blog" || catBuckets[1].Count != 2 {
		t.Errorf("expected blog=2 second, got %+v", catBuckets[1])
	}

	langBuckets := got["lang"]
	if len(langBuckets) != 2 {
		t.Fatalf("expected 2 lang buckets, got %d", len(langBuckets))
	}
	if langBuckets[0].Value != "en" || langBuckets[0].Count != 3 {
		t.Errorf("expected en=3 first, got %+v", langBuckets[0])
	}
}

func TestComputeFacets_MaxPerKey(t *testing.T) {
	docs := []storage.Doc{
		{ID: "1", Meta: map[string][]string{"t": {"a"}}},
		{ID: "2", Meta: map[string][]string{"t": {"b"}}},
		{ID: "3", Meta: map[string][]string{"t": {"c"}}},
	}
	got := computeFacets(docs, []string{"t"}, 2)
	if len(got["t"]) != 2 {
		t.Fatalf("expected 2 buckets after cap, got %d", len(got["t"]))
	}
}

func TestComputeFacets_IgnoresEmptyKeys(t *testing.T) {
	docs := []storage.Doc{{ID: "1", Meta: map[string][]string{"x": {"v"}}}}
	got := computeFacets(docs, []string{"", "x"}, 0)
	if _, bad := got[""]; bad {
		t.Error("empty facet key must be ignored")
	}
	if len(got["x"]) != 1 {
		t.Errorf("expected 1 bucket for x, got %d", len(got["x"]))
	}
}

func TestComputeFacets_MissingKeyProducesEmptyBucket(t *testing.T) {
	docs := []storage.Doc{{ID: "1", Meta: map[string][]string{"x": {"v"}}}}
	got := computeFacets(docs, []string{"missing"}, 0)
	// We want the key present in the map so UI renders a stable groups list.
	if _, ok := got["missing"]; !ok {
		t.Error("missing key must be present with empty bucket list")
	}
	if len(got["missing"]) != 0 {
		t.Errorf("expected 0 buckets for missing key, got %d", len(got["missing"]))
	}
}

func TestComputeFacets_StableTieBreak(t *testing.T) {
	// Two values with the same count → alphabetical tie-break.
	docs := []storage.Doc{
		{ID: "1", Meta: map[string][]string{"t": {"b"}}},
		{ID: "2", Meta: map[string][]string{"t": {"a"}}},
	}
	got := computeFacets(docs, []string{"t"}, 0)
	if got["t"][0].Value != "a" {
		t.Errorf("expected 'a' to sort before 'b' on tie, got %+v", got["t"])
	}
}
