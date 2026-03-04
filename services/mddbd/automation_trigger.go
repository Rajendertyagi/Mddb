package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"time"

	json "github.com/goccy/go-json"
)

// TriggerMatch represents a document that matched a trigger's search criteria.
type TriggerMatch struct {
	DocID      string  `json:"docId"`
	Key        string  `json:"key,omitempty"`
	Collection string  `json:"collection"`
	Score      float64 `json:"score"`
}

// TriggerPayload is sent to webhook URLs when a trigger fires.
type TriggerPayload struct {
	Event      string                `json:"event"` // "trigger.matched"
	Trigger    TriggerPayloadTrigger `json:"trigger"`
	Collection string                `json:"collection"`
	Document   *Doc                  `json:"document,omitempty"`
	Score      float64               `json:"score"`
	Timestamp  int64                 `json:"timestamp"`
}

// TriggerPayloadTrigger is the trigger info in the payload.
type TriggerPayloadTrigger struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EvaluateTriggers checks all enabled triggers for a collection after a document is added.
// Called asynchronously from addDocument().
func (am *AutomationManager) EvaluateTriggers(collection string, doc Doc) {
	triggers := am.EnabledTriggersForCollection(collection)
	if len(triggers) == 0 {
		return
	}

	for _, trigger := range triggers {
		trigger := trigger // capture
		am.evaluateSingleTrigger(&trigger, &doc)
	}
}

// evaluateSingleTrigger runs a trigger's search and fires webhook if the doc matches.
func (am *AutomationManager) evaluateSingleTrigger(trigger *AutomationRule, doc *Doc) {
	if am.server == nil {
		return
	}

	var score float64
	var matched bool

	switch trigger.SearchType {
	case "fts":
		score, matched = am.evalFTS(trigger, doc)
	case "vector":
		score, matched = am.evalVector(trigger, doc)
	case "hybrid":
		score, matched = am.evalHybrid(trigger, doc)
	}

	if !matched {
		return
	}

	// Resolve webhook
	webhook := am.GetWebhook(trigger.WebhookID)
	if webhook == nil || !webhook.Enabled {
		return
	}

	go fireAutomationWebhook(webhook, trigger, doc, trigger.Collection, score)
}

// evalFTS runs FTS search and checks if doc appears in results above threshold.
func (am *AutomationManager) evalFTS(trigger *AutomationRule, doc *Doc) (float64, bool) {
	s := am.server
	if s.FTSIndex == nil {
		return 0, false
	}

	results, err := s.FTSIndex.Search(trigger.Collection, trigger.Query, 100)
	if err != nil {
		log.Printf("trigger %s: FTS search error: %v", trigger.ID, err)
		return 0, false
	}

	for _, r := range results {
		if r.DocID == doc.ID {
			// FTS threshold: raw BM25 score
			if r.Score >= trigger.Threshold {
				return r.Score, true
			}
			return r.Score, false
		}
	}
	return 0, false
}

// evalVector runs vector search and checks if doc appears above threshold.
func (am *AutomationManager) evalVector(trigger *AutomationRule, doc *Doc) (float64, bool) {
	s := am.server
	if s.Embedding == nil {
		return 0, false
	}

	searcher, ok := s.VectorSearchers["flat"]
	if !ok || !searcher.IsReady() {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queryVector, err := s.Embedding.Embed(ctx, trigger.Query)
	if err != nil {
		log.Printf("trigger %s: embedding error: %v", trigger.ID, err)
		return 0, false
	}

	// Vector threshold: 0-100 maps to 0-1 similarity
	threshold := trigger.Threshold / 100.0
	results := searcher.Search(trigger.Collection, queryVector, 100, float64(threshold))

	for _, r := range results {
		if r.DocID == doc.ID {
			score := float64(r.Score) * 100 // normalize to 0-100
			return score, true
		}
	}
	return 0, false
}

// evalHybrid runs hybrid search and checks if doc appears above threshold.
func (am *AutomationManager) evalHybrid(trigger *AutomationRule, doc *Doc) (float64, bool) {
	s := am.server

	// Build a hybrid search request from trigger params
	req := HybridSearchRequest{
		Collection: trigger.Collection,
		Query:      trigger.Query,
		TopK:       100,
		Alpha:      0.5,
		Strategy:   "alpha",
	}

	// Override from searchParams if provided
	if sp := trigger.SearchParams; sp != nil {
		if v, ok := sp["alpha"].(float64); ok {
			req.Alpha = v
		}
		if v, ok := sp["strategy"].(string); ok {
			req.Strategy = v
		}
		if v, ok := sp["algorithm"].(string); ok {
			req.Algorithm = v
		}
		if v, ok := sp["vectorAlgorithm"].(string); ok {
			req.VectorAlgorithm = v
		}
	}

	if req.Algorithm == "" {
		req.Algorithm = "bm25"
	}
	if req.VectorAlgorithm == "" {
		req.VectorAlgorithm = "flat"
	}

	// Run FTS
	ftsResults, err := s.runFTSSearch(req)
	if err != nil {
		log.Printf("trigger %s: hybrid FTS error: %v", trigger.ID, err)
	}

	// Run vector
	ctx := context.Background()
	vectorResults, err := s.runVectorSearch(ctx, req)
	if err != nil {
		log.Printf("trigger %s: hybrid vector error: %v", trigger.ID, err)
	}

	// Merge
	var merged []HybridSearchResultItem
	switch req.Strategy {
	case "rrf":
		merged = mergeRRF(ftsResults, vectorResults, req.RRFK, req.TopK)
	default:
		merged = mergeAlpha(ftsResults, vectorResults, req.Alpha, req.TopK)
	}

	for _, m := range merged {
		if m.Document.ID == doc.ID {
			score := m.CombinedScore * 100 // normalize to 0-100
			if score >= trigger.Threshold {
				return score, true
			}
			return score, false
		}
	}
	return 0, false
}

// RunTrigger executes a trigger's search and returns all matches above threshold.
// Used by cron scheduler and manual test endpoint.
func (am *AutomationManager) RunTrigger(trigger *AutomationRule) ([]TriggerMatch, error) {
	if am.server == nil {
		return nil, nil
	}

	switch trigger.SearchType {
	case "fts":
		return am.runTriggerFTS(trigger)
	case "vector":
		return am.runTriggerVector(trigger)
	case "hybrid":
		return am.runTriggerHybrid(trigger)
	default:
		return nil, nil
	}
}

func (am *AutomationManager) runTriggerFTS(trigger *AutomationRule) ([]TriggerMatch, error) {
	s := am.server
	if s.FTSIndex == nil {
		return nil, nil
	}

	results, err := s.FTSIndex.Search(trigger.Collection, trigger.Query, 100)
	if err != nil {
		return nil, err
	}

	var matches []TriggerMatch
	for _, r := range results {
		if r.Score >= trigger.Threshold {
			matches = append(matches, TriggerMatch{
				DocID:      r.DocID,
				Collection: trigger.Collection,
				Score:      r.Score,
			})
		}
	}
	return matches, nil
}

func (am *AutomationManager) runTriggerVector(trigger *AutomationRule) ([]TriggerMatch, error) {
	s := am.server
	if s.Embedding == nil {
		return nil, nil
	}

	searcher, ok := s.VectorSearchers["flat"]
	if !ok || !searcher.IsReady() {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queryVector, err := s.Embedding.Embed(ctx, trigger.Query)
	if err != nil {
		return nil, err
	}

	threshold := trigger.Threshold / 100.0
	results := searcher.Search(trigger.Collection, queryVector, 100, float64(threshold))

	var matches []TriggerMatch
	for _, r := range results {
		score := float64(r.Score) * 100
		matches = append(matches, TriggerMatch{
			DocID:      r.DocID,
			Collection: trigger.Collection,
			Score:      score,
		})
	}
	return matches, nil
}

func (am *AutomationManager) runTriggerHybrid(trigger *AutomationRule) ([]TriggerMatch, error) {
	s := am.server

	req := HybridSearchRequest{
		Collection:      trigger.Collection,
		Query:           trigger.Query,
		TopK:            100,
		Alpha:           0.5,
		Strategy:        "alpha",
		Algorithm:       "bm25",
		VectorAlgorithm: "flat",
	}

	if sp := trigger.SearchParams; sp != nil {
		if v, ok := sp["alpha"].(float64); ok {
			req.Alpha = v
		}
		if v, ok := sp["strategy"].(string); ok {
			req.Strategy = v
		}
	}

	ftsResults, _ := s.runFTSSearch(req)
	ctx := context.Background()
	vectorResults, _ := s.runVectorSearch(ctx, req)

	var merged []HybridSearchResultItem
	switch req.Strategy {
	case "rrf":
		merged = mergeRRF(ftsResults, vectorResults, req.RRFK, req.TopK)
	default:
		merged = mergeAlpha(ftsResults, vectorResults, req.Alpha, req.TopK)
	}

	var matches []TriggerMatch
	for _, m := range merged {
		score := m.CombinedScore * 100
		if score >= trigger.Threshold {
			matches = append(matches, TriggerMatch{
				DocID:      m.Document.ID,
				Collection: trigger.Collection,
				Score:      score,
			})
		}
	}
	return matches, nil
}

// RunTriggerAndFire executes a trigger and fires the webhook for each match.
// Used by the cron scheduler.
func (am *AutomationManager) RunTriggerAndFire(trigger *AutomationRule) {
	webhook := am.GetWebhook(trigger.WebhookID)
	if webhook == nil || !webhook.Enabled {
		log.Printf("trigger %s: webhook %s not found or disabled", trigger.ID, trigger.WebhookID)
		return
	}

	matches, err := am.RunTrigger(trigger)
	if err != nil {
		log.Printf("trigger %s: search error: %v", trigger.ID, err)
		return
	}

	for _, match := range matches {
		go fireAutomationWebhook(webhook, trigger, nil, match.Collection, match.Score)
	}

	if len(matches) > 0 {
		log.Printf("trigger %s: fired webhook %s for %d matches", trigger.ID, webhook.ID, len(matches))
	}
}

// fireAutomationWebhook sends the trigger payload to a webhook URL.
func fireAutomationWebhook(webhook *AutomationRule, trigger *AutomationRule, doc *Doc, collection string, score float64) {
	payload := TriggerPayload{
		Event: "trigger.matched",
		Trigger: TriggerPayloadTrigger{
			ID:   trigger.ID,
			Name: trigger.Name,
		},
		Collection: collection,
		Document:   doc,
		Score:      score,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("trigger %s: marshal error: %v", trigger.ID, err)
		return
	}

	method := webhook.Method
	if method == "" {
		method = "POST"
	}

	backoffs := []time.Duration{0, 1 * time.Second, 5 * time.Second, 15 * time.Second}
	for attempt, backoff := range backoffs {
		if backoff > 0 {
			time.Sleep(backoff)
		}

		req, err := http.NewRequest(method, webhook.URL, bytes.NewReader(data))
		if err != nil {
			log.Printf("trigger %s → webhook %s: request error: %v", trigger.ID, webhook.ID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MDDB-Event", "trigger.matched")
		req.Header.Set("X-MDDB-Trigger-ID", trigger.ID)
		req.Header.Set("X-MDDB-Webhook-ID", webhook.ID)

		// Custom headers from webhook config
		for k, v := range webhook.Headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("trigger %s → webhook %s: attempt %d failed: %v", trigger.ID, webhook.ID, attempt+1, err)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return // success
		}
		log.Printf("trigger %s → webhook %s: attempt %d got status %d", trigger.ID, webhook.ID, attempt+1, resp.StatusCode)
	}
	log.Printf("trigger %s → webhook %s: all retries exhausted", trigger.ID, webhook.ID)
}
