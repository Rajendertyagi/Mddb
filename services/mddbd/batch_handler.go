package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	proto "mddb/proto"

	json "github.com/goccy/go-json"
)

// ---- HTTP types for /v1/add-batch ----

// AddBatchHTTPRequest is the HTTP request body for adding documents in batch.
type AddBatchHTTPRequest struct {
	Collection string             `json:"collection"`
	Documents  []AddBatchDocument `json:"documents"`
}

// AddBatchDocument represents a single document within a batch add request.
type AddBatchDocument struct {
	Key          string              `json:"key"`
	Lang         string              `json:"lang"`
	Meta         map[string][]string `json:"meta,omitempty"`
	ContentMD    string              `json:"contentMd"`
	SaveRevision bool                `json:"saveRevision,omitempty"`
}

// AddBatchHTTPResponse is the HTTP response body for a batch add operation.
type AddBatchHTTPResponse struct {
	Added   int      `json:"added"`
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// handleAddBatch handles POST /v1/add-batch
func (s *Server) handleAddBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddBatchHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if len(req.Documents) == 0 {
		ok(w, AddBatchHTTPResponse{})
		return
	}

	// Convert to proto BatchDocument
	protoDocs := make([]*proto.BatchDocument, len(req.Documents))
	for i, d := range req.Documents {
		protoDocs[i] = &proto.BatchDocument{
			Key:          d.Key,
			Lang:         d.Lang,
			Meta:         toProtoMeta(d.Meta),
			ContentMd:    d.ContentMD,
			SaveRevision: d.SaveRevision,
		}
	}

	// Process batch
	resp, processed, err := s.processBatchWithDocs(r.Context(), req.Collection, protoDocs)
	if err != nil {
		bad(w, err)
		return
	}

	// Fire post-batch hooks
	s.firePostBatchHooks(req.Collection, processed, postBatchOptions{})

	s.Metrics.IncOp("add_batch")

	ok(w, AddBatchHTTPResponse{
		Added:   int(resp.Added),
		Updated: int(resp.Updated),
		Failed:  int(resp.Failed),
		Errors:  resp.Errors,
	})
}

// postBatchOptions controls which post-commit hooks to fire.
type postBatchOptions struct {
	SkipEmbeddings bool
	SkipFTS        bool
	SkipWebhooks   bool
}

// processBatchWithDocs runs the batch processor and returns both the response and processed docs.
func (s *Server) processBatchWithDocs(ctx context.Context, collection string, protoDocs []*proto.BatchDocument) (*proto.AddBatchResponse, []*ProcessedDoc, error) {
	now := time.Now().Unix()

	var resp *proto.AddBatchResponse
	var processed []*ProcessedDoc
	var err error

	if s.UseExtreme && s.finalBatchProcessor != nil {
		// FinalBatchProcessor: single read tx → parallel marshal → single write tx
		existingMap := s.finalBatchProcessor.batchReadAll(collection, protoDocs)
		all := s.finalBatchProcessor.parallelMarshal(ctx, collection, protoDocs, existingMap, now)
		resp, processed = s.finalBatchProcessor.commitBatch(collection, all, now)
	} else {
		bp := NewBatchProcessor(s, 8)
		all := bp.parallelProcess(ctx, collection, protoDocs, now)
		resp, processed = bp.commitBatch(collection, all, now)
	}

	if resp.Failed == safeInt32(len(protoDocs)) && len(resp.Errors) > 0 {
		err = fmt.Errorf("all documents failed: %s", resp.Errors[0])
	}

	// `processed` is now the set of durably-committed docs, so the caller's
	// firePostBatchHooks only fires for documents that actually landed.
	return resp, processed, err
}

// firePostBatchHooks fires embedding, FTS, webhook, TTL, and automation hooks
// for all successfully processed documents after batch commit.
func (s *Server) firePostBatchHooks(collection string, processed []*ProcessedDoc, opts postBatchOptions) {
	for _, p := range processed {
		if p.Error != nil {
			continue
		}

		// Embedding
		if !opts.SkipEmbeddings && s.EmbeddingWorker != nil && p.Doc.ContentMD != "" {
			s.EmbeddingWorker.Enqueue(EmbeddingJob{
				Collection: collection,
				DocID:      p.DocID,
				ContentMD:  p.Doc.ContentMD,
			})
		}

		// TTL
		if s.TTLManager != nil && p.Doc.ExpiresAt > 0 {
			_ = s.TTLManager.Set(collection, p.DocID, p.Doc.ExpiresAt)
		}

		// Geo indexing (R-tree + geohash + GeoStore) — parity with the
		// single-doc path (GO-001); previously every batch path skipped geo.
		if s.GeoIndex != nil && s.GeoStore != nil {
			if lat, lng, okGeo := s.GeoIndex.AddFromMeta(collection, p.DocID, p.Doc.Meta); okGeo {
				_ = s.GeoStore.Put(collection, p.DocID, lat, lng)
				if s.GeoHashIndex != nil {
					s.GeoHashIndex.Add(collection, p.DocID, lat, lng)
				}
			} else if p.IsUpdate {
				s.GeoIndex.Remove(collection, p.DocID)
				if s.GeoHashIndex != nil {
					s.GeoHashIndex.Remove(collection, p.DocID)
				}
				_ = s.GeoStore.Delete(collection, p.DocID)
			}
		}

		// Temporal tracking — parity with the single-doc path (GO-001).
		if s.TemporalManager != nil {
			et := EventUpdate
			if !p.IsUpdate {
				et = EventCreate
			}
			s.TemporalManager.RecordAsync(collection, p.DocID, et, "")
		}

		// FTS indexing (language-aware)
		if !opts.SkipFTS && s.FTSIndex != nil && p.Doc.ContentMD != "" {
			_ = s.FTSIndex.IndexWithLang(collection, p.DocID, p.Doc.ContentMD, p.Doc.Lang)
			_ = s.FTSIndex.IndexPositionsWithLang(collection, p.DocID, p.Doc.ContentMD, p.Doc.Lang)
			fields := map[string]string{"content": p.Doc.ContentMD}
			for k, vals := range p.Doc.Meta {
				if len(vals) > 0 {
					fields["meta."+k] = strings.Join(vals, " ")
				}
			}
			_ = s.FTSIndex.IndexFieldsWithLang(collection, p.DocID, fields, p.Doc.Lang)
		}

		// Webhooks + SSE
		if !opts.SkipWebhooks {
			event := "doc.updated"
			if !p.IsUpdate {
				event = "doc.added"
			}
			if s.WebhookManager != nil {
				s.WebhookManager.Fire(event, collection, p.Doc.Key, p.Doc.Lang, &p.Doc)
			}
			if s.SSEHub != nil {
				s.SSEHub.BroadcastWithAuth(event, collection, p.Doc.Key, p.Doc.Lang, s.AuthManager)
			}
		}

		// Automation triggers
		if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
			triggerEvent := "insert"
			if p.IsUpdate {
				triggerEvent = "update"
			}
			go s.AutomationManager.EvaluateTriggers(collection, p.Doc, triggerEvent)
		}
	}
}
