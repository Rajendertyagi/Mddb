package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SEC-005: withMaxBody caps request body size on the main JSON endpoints.
func TestWithMaxBody(t *testing.T) {
	const limit = int64(100)
	exempt := map[string]bool{"/v1/upload": true}

	var lastReadErr error
	var lastBodyLen int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		lastReadErr = err
		lastBodyLen = len(b)
		w.WriteHeader(http.StatusOK)
	})
	h := withMaxBody(limit, exempt, next)

	t.Run("oversize Content-Length -> 413", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/add", strings.NewReader(strings.Repeat("x", 200)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("got %d, want 413", rec.Code)
		}
	})

	t.Run("under limit -> 200, body intact", func(t *testing.T) {
		lastBodyLen = 0
		req := httptest.NewRequest(http.MethodPost, "/v1/add", strings.NewReader("small"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || lastBodyLen != 5 {
			t.Errorf("got code=%d len=%d, want 200 and 5", rec.Code, lastBodyLen)
		}
	})

	t.Run("exempt path is not capped", func(t *testing.T) {
		lastBodyLen = 0
		req := httptest.NewRequest(http.MethodPost, "/v1/upload", strings.NewReader(strings.Repeat("y", 500)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || lastBodyLen != 500 {
			t.Errorf("got code=%d len=%d, want 200 and 500 (exempt)", rec.Code, lastBodyLen)
		}
	})

	t.Run("unknown Content-Length is capped on read", func(t *testing.T) {
		lastReadErr = nil
		req := httptest.NewRequest(http.MethodPost, "/v1/add", strings.NewReader(strings.Repeat("z", 200)))
		req.ContentLength = -1 // unknown length: ContentLength check can't catch it
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if lastReadErr == nil {
			t.Error("expected MaxBytesReader to error once the body exceeds the limit")
		}
	})

	t.Run("limit <= 0 disables capping", func(t *testing.T) {
		lastBodyLen = 0
		hOff := withMaxBody(0, exempt, next)
		req := httptest.NewRequest(http.MethodPost, "/v1/add", strings.NewReader(strings.Repeat("w", 300)))
		rec := httptest.NewRecorder()
		hOff.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || lastBodyLen != 300 {
			t.Errorf("got code=%d len=%d, want 200 and 300 (disabled)", rec.Code, lastBodyLen)
		}
	})
}
