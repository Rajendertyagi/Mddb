package main

import (
	"encoding/json"
	"testing"
	"time"
)

// GO-005: the CLI parses server JSON into map[string]interface{}; these helpers
// must never panic on missing/null/wrong-typed fields (previously bare
// x.(float64) assertions crashed the CLI).

func TestAsFloat(t *testing.T) {
	if got := asFloat(float64(42)); got != 42 {
		t.Errorf("asFloat(42) = %v, want 42", got)
	}
	for _, v := range []interface{}{nil, "x", true, map[string]interface{}{}, int(5)} {
		if got := asFloat(v); got != 0 {
			t.Errorf("asFloat(%v) = %v, want 0", v, got)
		}
	}
}

func TestAsString(t *testing.T) {
	if got := asString("hi"); got != "hi" {
		t.Errorf("asString = %q, want hi", got)
	}
	for _, v := range []interface{}{nil, float64(1), true} {
		if got := asString(v); got != "" {
			t.Errorf("asString(%v) = %q, want empty", v, got)
		}
	}
}

func TestAsMap(t *testing.T) {
	m := map[string]interface{}{"a": 1}
	if got := asMap(m); got == nil || len(got) != 1 {
		t.Errorf("asMap(map) = %v, want the map", got)
	}
	if got := asMap(nil); got != nil {
		t.Errorf("asMap(nil) = %v, want nil", got)
	}
	// Reading a key from a nil result must be safe (Go returns the zero value).
	if v := asMap(nil)["missing"]; v != nil {
		t.Errorf("nil-map read = %v, want nil", v)
	}
}

func TestFormatUnix(t *testing.T) {
	ts := float64(1_700_000_000)
	want := time.Unix(1_700_000_000, 0).Format(time.RFC3339)
	if got := formatUnix(ts); got != want {
		t.Errorf("formatUnix(%v) = %q, want %q", ts, got, want)
	}
	for _, v := range []interface{}{nil, "not-a-number", map[string]interface{}{}} {
		if got := formatUnix(v); got != "-" {
			t.Errorf("formatUnix(%v) = %q, want %q", v, got, "-")
		}
	}
}

// TestDegenerateResponsesDoNotPanic exercises the helpers against the kinds of
// unexpected payloads (missing fields, an error object, null values) that used
// to crash the CLI.
func TestDegenerateResponsesDoNotPanic(t *testing.T) {
	payloads := []string{
		`{}`,
		`{"error":"forbidden"}`,
		`{"addedAt":null,"updatedAt":null}`,
		`{"id":"x"}`,
		`{"results":[{"document":null,"score":"oops","rank":null}]}`,
	}
	for _, p := range payloads {
		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(p), &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", p, err)
		}
		// Mimic the add/search formatting paths — must not panic.
		_ = formatUnix(doc["addedAt"])
		_ = formatUnix(doc["updatedAt"])
		results, _ := doc["results"].([]interface{})
		for _, r := range results {
			item := asMap(r)
			d := asMap(item["document"])
			_ = asFloat(item["score"])
			_ = int(asFloat(item["rank"]))
			_ = d["key"]
		}
	}
}
