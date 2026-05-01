package main

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateTimeBuckets(t *testing.T) {
	mkUnix := func(s string) int64 {
		tt, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return tt.Unix()
	}

	cases := []struct {
		name        string
		from, to    string
		interval    string
		minBuckets  int
		labelFormat string
	}{
		{"day", "2026-01-15", "2026-01-18", "day", 4, "2026-01-15"},
		{"week", "2026-01-05", "2026-01-26", "week", 3, "2026-W"},
		{"month", "2026-01-15", "2026-04-10", "month", 4, "2026-01"},
		{"year", "2024-06-01", "2026-02-01", "year", 3, "2024"},
		{"default-is-month", "2026-01-15", "2026-03-15", "weird", 2, "2026-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := generateTimeBuckets(mkUnix(tc.from), mkUnix(tc.to), tc.interval)
			if len(b) < tc.minBuckets {
				t.Errorf("got %d buckets, want >=%d", len(b), tc.minBuckets)
			}
			if !strings.HasPrefix(b[0].label, tc.labelFormat[:4]) {
				t.Errorf("first label=%q, want prefix %q", b[0].label, tc.labelFormat[:4])
			}
			// Buckets must be monotonically increasing.
			for i := 1; i < len(b); i++ {
				if b[i].from <= b[i-1].from {
					t.Errorf("non-monotonic at %d: %d <= %d", i, b[i].from, b[i-1].from)
				}
			}
		})
	}
}

func TestFindBucket(t *testing.T) {
	bs := []timeBucketRange{
		{label: "a", from: 0, to: 100},
		{label: "b", from: 100, to: 200},
		{label: "c", from: 200, to: 300},
	}
	if i := findBucket(bs, 50); i != 0 {
		t.Errorf("ts=50 → %d, want 0", i)
	}
	if i := findBucket(bs, 150); i != 1 {
		t.Errorf("ts=150 → %d, want 1", i)
	}
	if i := findBucket(bs, 250); i != 2 {
		t.Errorf("ts=250 → %d, want 2", i)
	}
}

