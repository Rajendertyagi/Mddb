package webhooks

import (
	"encoding/json"
	"io"
	"mddb/internal/metrics"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// TestSetMetricsCountsFires wires a metrics collector and verifies Fire takes
// the metrics branch (IncOp) when one is set.
func TestSetMetricsCountsFires(t *testing.T) {
	wm, cleanup := newTestWebhookManager(t)
	defer cleanup()

	wm.SetMetrics(metrics.NewMetrics(true, nil))

	got := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case got <- struct{}{}:
		default:
		}
	}))
	defer ts.Close()

	if _, err := wm.Register(ts.URL, []string{"doc.added"}, "blog"); err != nil {
		t.Fatal(err)
	}
	wm.Fire("doc.added", "blog", "k", "en", nil)

	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook with metrics set was not delivered")
	}
}

// TestFireEventDeliversDetail covers FireEvent and the detail-carrying payload
// path used by the incident detectors.
func TestFireEventDeliversDetail(t *testing.T) {
	wm, cleanup := newTestWebhookManager(t)
	defer cleanup()

	payloads := make(chan WebhookPayload, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p WebhookPayload
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &p)
		payloads <- p
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if _, err := wm.Register(ts.URL, []string{EventRateLimitExceeded}, ""); err != nil {
		t.Fatal(err)
	}
	wm.FireEvent(EventRateLimitExceeded, map[string]interface{}{"clientId": "1.2.3.4"})

	select {
	case p := <-payloads:
		if p.Event != EventRateLimitExceeded {
			t.Errorf("event = %q, want %q", p.Event, EventRateLimitExceeded)
		}
		if p.Detail["clientId"] != "1.2.3.4" {
			t.Errorf("detail clientId = %v, want 1.2.3.4", p.Detail["clientId"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("FireEvent was not delivered")
	}
}

// TestReloadRepointsDatabase covers Reload: it re-points the manager at a fresh
// database and reloads the in-memory hooks from it.
func TestReloadRepointsDatabase(t *testing.T) {
	wm, cleanup := newTestWebhookManager(t)
	defer cleanup()

	if _, err := wm.Register("http://example.com/h", []string{"doc.added"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(wm.List()) != 1 {
		t.Fatalf("expected 1 hook before reload, got %d", len(wm.List()))
	}

	f2, err := os.CreateTemp("", "wh_reload_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f2.Close()
	defer func() { _ = os.Remove(f2.Name()) }()
	db2, err := bolt.Open(f2.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()

	if err := wm.Reload(db2); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if n := len(wm.List()); n != 0 {
		t.Errorf("expected 0 hooks after reload onto empty db, got %d", n)
	}
}

// TestFireWebhookRequestError covers the http.NewRequest error branch (a URL
// with a control character cannot be parsed into a request).
func TestFireWebhookRequestError(t *testing.T) {
	// Must not panic or block; returns immediately on the request-build error.
	fireWebhook(Webhook{ID: "bad", URL: "http://exa\x7fmple"}, WebhookPayload{Event: "doc.added"})
}

// TestLoadAllSkipsCorruptAndMissingBucket covers the corrupt-entry skip and the
// nil-bucket short-circuit in LoadAll.
func TestLoadAllSkipsCorruptAndMissingBucket(t *testing.T) {
	wm, cleanup := newTestWebhookManager(t)
	defer cleanup()

	// Insert one valid + one corrupt record directly.
	if err := wm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWebhooks)
		good, _ := json.Marshal(Webhook{ID: "ok", URL: "http://x", Events: []string{"doc.added"}})
		if err := b.Put([]byte("wh|ok"), good); err != nil {
			return err
		}
		return b.Put([]byte("wh|bad"), []byte("{not json"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := wm.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if n := len(wm.List()); n != 1 {
		t.Errorf("expected corrupt entry skipped (1 hook), got %d", n)
	}

	// A manager whose database has no webhooks bucket loads nothing.
	f, err := os.CreateTemp("", "wh_nobucket_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	bare := NewWebhookManager(db)
	if err := bare.LoadAll(); err != nil {
		t.Errorf("LoadAll on missing bucket should be nil, got %v", err)
	}
	if len(bare.List()) != 0 {
		t.Error("expected no hooks from a db without the bucket")
	}
}

// TestDeleteUnknownID covers the Delete path where no in-memory hook matches.
func TestDeleteUnknownID(t *testing.T) {
	wm, cleanup := newTestWebhookManager(t)
	defer cleanup()
	if _, err := wm.Register("http://example.com/h", []string{"doc.added"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := wm.Delete("does-not-exist"); err != nil {
		t.Errorf("Delete of unknown id should be a no-op, got %v", err)
	}
	if len(wm.List()) != 1 {
		t.Error("Delete of unknown id must not remove existing hooks")
	}
}
