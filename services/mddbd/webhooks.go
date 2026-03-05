package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var bucketWebhooks = []byte("webhooks")

// Webhook represents a registered webhook subscription.
type Webhook struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	Events     []string `json:"events"`               // "doc.added", "doc.updated", "doc.deleted"
	Collection string   `json:"collection,omitempty"` // empty = all collections
	CreatedAt  int64    `json:"createdAt"`
}

// WebhookPayload is the payload sent to webhook endpoints.
type WebhookPayload struct {
	Event      string `json:"event"`
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
	Timestamp  int64  `json:"timestamp"`
	Document   *Doc   `json:"document,omitempty"`
}

// WebhookManager manages webhook registrations and delivery.
type WebhookManager struct {
	db      *bolt.DB
	mu      sync.RWMutex
	hooks   []Webhook
	binlog  *Binlog
	metrics *Metrics
}

// SetBinlog sets the binlog for replication logging.
func (wm *WebhookManager) SetBinlog(bl *Binlog) {
	wm.binlog = bl
}

// NewWebhookManager creates a new webhook manager.
func NewWebhookManager(db *bolt.DB) *WebhookManager {
	return &WebhookManager{db: db}
}

// EnsureBucket creates the webhooks bucket.
func (wm *WebhookManager) EnsureBucket() error {
	return wm.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketWebhooks)
		return err
	})
}

// LoadAll loads all webhooks from the database into memory.
func (wm *WebhookManager) LoadAll() error {
	var hooks []Webhook
	err := wm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWebhooks)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var wh Webhook
			if err := json.Unmarshal(v, &wh); err != nil {
				return nil // skip corrupt entries
			}
			hooks = append(hooks, wh)
			return nil
		})
	})
	if err != nil {
		return err
	}
	wm.mu.Lock()
	wm.hooks = hooks
	wm.mu.Unlock()
	return nil
}

// Register creates a new webhook subscription.
func (wm *WebhookManager) Register(url string, events []string, collection string) (*Webhook, error) {
	if url == "" {
		return nil, errors.New("url is required")
	}
	if len(events) == 0 {
		return nil, errors.New("at least one event is required")
	}

	// Validate events
	validEvents := map[string]bool{"doc.added": true, "doc.updated": true, "doc.deleted": true}
	for _, e := range events {
		if !validEvents[e] {
			return nil, fmt.Errorf("invalid event: %s (valid: doc.added, doc.updated, doc.deleted)", e)
		}
	}

	wh := Webhook{
		ID:         generateWebhookID(),
		URL:        url,
		Events:     events,
		Collection: collection,
		CreatedAt:  time.Now().Unix(),
	}

	data, _ := json.Marshal(wh)
	whKey := []byte("wh|" + wh.ID)
	if err := wm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWebhooks)
		return b.Put(whKey, data)
	}); err != nil {
		return nil, err
	}

	if wm.binlog != nil {
		_ = wm.binlog.Append(&BinlogEntry{Type: BinlogPut, BucketName: "webhooks", Key: copyBytes(whKey), Value: copyBytes(data)})
	}

	wm.mu.Lock()
	wm.hooks = append(wm.hooks, wh)
	wm.mu.Unlock()

	return &wh, nil
}

// List returns all registered webhooks.
func (wm *WebhookManager) List() []Webhook {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	result := make([]Webhook, len(wm.hooks))
	copy(result, wm.hooks)
	return result
}

// Delete removes a webhook by ID.
func (wm *WebhookManager) Delete(id string) error {
	whKey := []byte("wh|" + id)
	if err := wm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWebhooks)
		return b.Delete(whKey)
	}); err != nil {
		return err
	}

	if wm.binlog != nil {
		_ = wm.binlog.Append(&BinlogEntry{Type: BinlogDelete, BucketName: "webhooks", Key: copyBytes(whKey)})
	}

	wm.mu.Lock()
	for i, h := range wm.hooks {
		if h.ID == id {
			wm.hooks = append(wm.hooks[:i], wm.hooks[i+1:]...)
			break
		}
	}
	wm.mu.Unlock()
	return nil
}

// Fire sends webhook notifications for a given event.
func (wm *WebhookManager) Fire(event, collection, key, lang string, doc *Doc) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	payload := WebhookPayload{
		Event:      event,
		Collection: collection,
		Key:        key,
		Lang:       lang,
		Timestamp:  time.Now().Unix(),
		Document:   doc,
	}

	for _, hook := range wm.hooks {
		if !hookMatches(hook, event, collection) {
			continue
		}
		if wm.metrics != nil {
			wm.metrics.IncOp("webhook_fire", event)
		}
		go fireWebhook(hook, payload)
	}
}

func hookMatches(hook Webhook, event, collection string) bool {
	// Check event match
	eventMatch := false
	for _, e := range hook.Events {
		if e == event {
			eventMatch = true
			break
		}
	}
	if !eventMatch {
		return false
	}
	// Check collection filter
	if hook.Collection != "" && hook.Collection != collection {
		return false
	}
	return true
}

func fireWebhook(hook Webhook, payload WebhookPayload) {
	data, _ := json.Marshal(payload)

	backoffs := []time.Duration{0, 1 * time.Second, 5 * time.Second, 15 * time.Second}
	for attempt, backoff := range backoffs {
		if backoff > 0 {
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", hook.URL, bytes.NewReader(data))
		if err != nil {
			log.Printf("webhook %s: request error: %v", hook.ID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MDDB-Event", payload.Event)
		req.Header.Set("X-MDDB-Webhook-ID", hook.ID)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("webhook %s: attempt %d failed: %v", hook.ID, attempt+1, err)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return // success
		}
		log.Printf("webhook %s: attempt %d got status %d", hook.ID, attempt+1, resp.StatusCode)
	}
	log.Printf("webhook %s: all retries exhausted for event %s", hook.ID, payload.Event)
}

func generateWebhookID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// --- HTTP handlers ---

type RegisterWebhookRequest struct {
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Collection string   `json:"collection,omitempty"`
}

type DeleteWebhookRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	if s.WebhookManager == nil {
		bad(w, errors.New("webhooks not initialized"))
		return
	}

	switch r.Method {
	case "GET":
		hooks := s.WebhookManager.List()
		ok(w, hooks)

	case "POST":
		if s.Mode == ModeRead {
			http.Error(w, `{"error":"read-only mode"}`, http.StatusForbidden)
			return
		}
		var req RegisterWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			bad(w, err)
			return
		}
		wh, err := s.WebhookManager.Register(req.URL, req.Events, req.Collection)
		if err != nil {
			bad(w, err)
			return
		}
		ok(w, wh)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	if s.WebhookManager == nil {
		bad(w, errors.New("webhooks not initialized"))
		return
	}

	var req DeleteWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.ID == "" {
		bad(w, errors.New("missing id"))
		return
	}

	if err := s.WebhookManager.Delete(req.ID); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "deleted", "id": req.ID})
}
