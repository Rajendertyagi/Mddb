package metrics

import "testing"

func TestHistogramObserve(t *testing.T) {
	h := newHistogram()

	// Observe values in different buckets
	h.observe(0.0005) // bucket 0.001
	h.observe(0.003)  // bucket 0.005
	h.observe(0.02)   // bucket 0.025
	h.observe(0.5)    // bucket 0.5
	h.observe(15.0)   // exceeds all buckets (+Inf only)

	if h.total != 5 {
		t.Errorf("expected total=5, got %d", h.total)
	}

	// Verify per-bucket non-cumulative counts
	expected := map[float64]int64{
		0.001: 1, 0.005: 1, 0.025: 1, 0.5: 1,
	}
	for i, b := range h.buckets {
		if exp, ok := expected[b]; ok {
			if h.counts[i] != exp {
				t.Errorf("bucket le=%.3f: expected %d, got %d", b, exp, h.counts[i])
			}
		} else {
			if h.counts[i] != 0 {
				t.Errorf("bucket le=%.3f: expected 0, got %d", b, h.counts[i])
			}
		}
	}
}
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"/v1/add", "/v1/add"},
		{"/v1/search", "/v1/search"},
		{"/health", "/health"},
		{"/metrics", "/metrics"},
		{"/unknown", "/other"},
		{"/foo/bar", "/other"},
	}
	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
func TestNormalizePathCB2(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/v1/docs", "/v1/docs"},
		{"/health", "/health"},
		{"/metrics", "/metrics"},
		{"/random", "/other"},
		{"/api/foo", "/other"},
	}
	for _, tc := range tests {
		if got := normalizePath(tc.in); got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
func TestMetrics_IncOp_DisabledIsNoop(t *testing.T) {
	m := NewMetrics(false, nil)
	m.IncOp("a", "b")
	m.IncOp("a", "b")
	if v := m.opsCount["a|b"]; v != 0 {
		t.Errorf("disabled metrics should not record, got %d", v)
	}
}
func TestMetrics_IncOp_EnabledRecords(t *testing.T) {
	m := NewMetrics(true, nil)
	m.IncOp("op", "ok")
	m.IncOp("op", "ok")
	m.IncOp("op", "fail")
	if v := m.opsCount["op|ok"]; v != 2 {
		t.Errorf("op|ok = %d, want 2", v)
	}
	if v := m.opsCount["op|fail"]; v != 1 {
		t.Errorf("op|fail = %d, want 1", v)
	}
}
