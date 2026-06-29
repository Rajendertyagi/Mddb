package mddb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeErrors makes every typed (doJSON-based) method fail to decode by
// returning malformed JSON, covering each method's error branch and doJSON's
// decode-error path.
func TestDecodeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	ctx := context.Background()

	calls := []struct {
		name string
		err  error
	}{
		{"Add", firstErr(c.Add(ctx, AddRequest{}))},
		{"Get", firstErr(c.Get(ctx, GetRequest{}))},
		{"SetTTL", firstErr(c.SetTTL(ctx, SetTTLRequest{}))},
		{"ImportURL", firstErr(c.ImportURL(ctx, ImportURLRequest{}))},
		{"Stats", firstErr(c.Stats(ctx))},
		{"RegisterWebhook", firstErr(c.RegisterWebhook(ctx, RegisterWebhookRequest{}))},
	}
	for _, tc := range calls {
		if tc.err == nil || !strings.Contains(tc.err.Error(), "decode response") {
			t.Errorf("%s: expected decode error, got %v", tc.name, tc.err)
		}
	}
	if _, err := c.Search(ctx, SearchRequest{}); err == nil {
		t.Error("Search: expected decode error")
	}
	if _, err := c.ListWebhooks(ctx); err == nil {
		t.Error("ListWebhooks: expected decode error")
	}
}

// firstErr discards a (T, error) result, keeping only the error, so the table
// above can hold heterogeneous calls.
func firstErr[T any](_ T, err error) error { return err }

func TestDoEncodeError(t *testing.T) {
	c := New("http://127.0.0.1:0")
	// A channel cannot be marshalled to JSON -> encode error before any dial.
	_, err := c.Do(context.Background(), "POST", "/x", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "encode request") {
		t.Errorf("expected encode error, got %v", err)
	}
}

func TestDoBadMethod(t *testing.T) {
	c := New("http://127.0.0.1:0")
	// An invalid HTTP method makes http.NewRequestWithContext fail.
	_, err := c.Do(context.Background(), "BAD\nMETHOD", "/x", nil)
	if err == nil {
		t.Error("expected error for invalid method")
	}
}

func TestDoConnectionError(t *testing.T) {
	// Port 1 on loopback refuses connections; the transport call errors.
	c := New("http://127.0.0.1:1")
	if _, err := c.Stats(context.Background()); err == nil {
		t.Error("expected connection error")
	}
	if err := c.DeleteWebhook(context.Background(), "x"); err == nil {
		t.Error("expected connection error for DeleteWebhook")
	}
}
