package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"mddb/internal/binlog"
	"mddb/internal/sliceutil"
	"mddb/internal/storage"
	"mddb/internal/temporal"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// --- handlers

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if err := s.SchemaManager.Validate(req.Collection, req.Meta); err != nil {
		bad(w, err)
		return
	}

	saved, _, err := s.addDocument(req.Collection, req.Key, req.Lang, req.Meta, req.ContentMD, req.TTL, true)
	if err != nil {
		bad(w, err)
		return
	}
	ok(w, saved)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	var req GetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var doc storage.Doc
	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bByK := tx.Bucket([]byte("bykey"))
		docID := bByK.Get(storage.ByKeyKey(req.Collection, req.Key, req.Lang))
		if docID == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(storage.DocKey(req.Collection, string(docID)))
		if v == nil {
			return errors.New("not found")
		}
		docPtr, unmErr := loadDoc(v)
		if unmErr != nil {
			return unmErr
		}
		doc = *docPtr
		return nil
	})
	if err != nil {
		bad(w, err)
		return
	}

	// Check TTL expiry
	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		bad(w, errors.New("not found"))
		return
	}

	// Temporal access tracking (gated on collection config)
	if s.TemporalManager != nil && s.CollectionManager != nil {
		if cfg, cfgOk := s.CollectionManager.Get(req.Collection); cfgOk && cfg.TrackAccess {
			actor := ""
			if claims, ok := GetClaimsFromContext(r.Context()); ok {
				actor = claims.Username
			}
			s.TemporalManager.RecordAsync(req.Collection, doc.ID, temporal.EventAccess, actor)
		}
	}

	// Templating via ENV: replace %%var%%
	if len(req.Env) > 0 && doc.ContentMD != "" {
		doc.ContentMD = applyEnv(doc.ContentMD, req.Env)
	}
	ok(w, doc)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	type row struct{ Doc storage.Doc }
	var rows []row

	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		seen := make(map[string]bool)

		// Jeśli brak filtra meta → pełny scan kolekcji (dla prostoty; można dodać bucket per collection)
		if len(req.FilterMeta) == 0 {
			c := bDocs.Cursor()
			prefix := []byte("doc|" + req.Collection + "|")
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				d, err := loadDoc(v)
				if err != nil {
					return err
				}
				if d.ExpiresAt > 0 && d.ExpiresAt < time.Now().Unix() {
					continue
				}
				rows = append(rows, row{*d})
			}
		} else {
			// Intersect po meta kluczach
			var sets [][]string
			for mk, mvals := range req.FilterMeta {
				var ids []string
				for _, mv := range mvals {
					prefix := storage.MetaKeyPrefix(req.Collection, mk, mv)
					c := bIdx.Cursor()
					for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
						id := string(k[len(prefix):])
						ids = append(ids, id)
					}
				}
				ids = sliceutil.Unique(ids)
				sets = append(sets, ids)
			}
			ids := intersect(sets...)
			for _, id := range ids {
				if seen[id] {
					continue
				}
				seen[id] = true
				v := tx.Bucket([]byte("docs")).Get(storage.DocKey(req.Collection, id))
				if v == nil {
					continue
				}
				d, err := loadDoc(v)
				if err != nil {
					return err
				}
				if d.ExpiresAt > 0 && d.ExpiresAt < time.Now().Unix() {
					continue
				}
				rows = append(rows, row{*d})
			}
		}
		return nil
	})
	if err != nil {
		bad(w, err)
		return
	}

	// sort
	switch req.Sort {
	case "addedAt":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.AddedAt < rows[j].Doc.AddedAt
			}
			return rows[i].Doc.AddedAt > rows[j].Doc.AddedAt
		})
	case "updatedAt":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.UpdatedAt < rows[j].Doc.UpdatedAt
			}
			return rows[i].Doc.UpdatedAt > rows[j].Doc.UpdatedAt
		})
	case "key":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.Key < rows[j].Doc.Key
			}
			return rows[i].Doc.Key > rows[j].Doc.Key
		})
	}

	// paginate
	start := req.Offset
	if start > len(rows) {
		start = len(rows)
	}
	end := start + req.Limit
	if end > len(rows) {
		end = len(rows)
	}

	out := make([]storage.Doc, 0, end-start)
	for _, r := range rows[start:end] {
		out = append(out, r.Doc)
	}
	w.Header().Set("X-Total-Count", fmt.Sprintf("%d", len(rows)))
	ok(w, out)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Format == "" {
		req.Format = "ndjson"
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// Reużyj /search
	sr := SearchRequest{Collection: req.Collection, FilterMeta: req.FilterMeta, Limit: 1 << 30}
	buf := new(bytes.Buffer)

	switch req.Format {
	case "ndjson":
		// stream NDJSON
		res, _ := http.Post("http://localhost"+env("MDDB_ADDR", ":11023")+"/v1/search", "application/json", bytes.NewReader(mustJSON(sr)))
		defer func() { _ = res.Body.Close() }()
		var docs []storage.Doc
		_ = json.NewDecoder(res.Body).Decode(&docs)
		for _, d := range docs {
			b, _ := json.Marshal(d)
			buf.Write(b)
			buf.WriteByte('\n')
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(buf.Bytes())

	case "zip":
		// pack contentMd as files {key}.{lang}.md
		res, _ := http.Post("http://localhost"+env("MDDB_ADDR", ":11023")+"/v1/search", "application/json", bytes.NewReader(mustJSON(sr)))
		defer func() { _ = res.Body.Close() }()
		var docs []storage.Doc
		_ = json.NewDecoder(res.Body).Decode(&docs)
		var z bytes.Buffer
		zw := zip.NewWriter(&z)
		for _, d := range docs {
			name := fmt.Sprintf("%s.%s.md", safe(d.Key), safe(d.Lang))
			f, _ := zw.Create(name)
			_, _ = io.WriteString(f, d.ContentMD)
		}
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(z.Bytes())

	default:
		http.Error(w, `{"error":"unsupported format"}`, 400)
	}
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	// Check admin permission (database-wide operation)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermAdmin); err != nil {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
	}

	// snapshot = copy pliku DB (najprościej)
	dst := r.URL.Query().Get("to")
	if dst == "" {
		dst = fmt.Sprintf("backup-%d.db", time.Now().Unix())
	}
	safeDst, err := safeBackupPath(dst, false)
	if err != nil {
		bad(w, err)
		return
	}
	if err := copyFile(s.Path, safeDst); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"backup": safeDst})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		bad(w, err)
		return
	}
	if body.From == "" {
		bad(w, errors.New("missing from"))
		return
	}

	// Check admin permission (database-wide operation)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermAdmin); err != nil {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
	}

	safeFrom, err := safeBackupPath(body.From, true)
	if err != nil {
		bad(w, err)
		return
	}

	// zamknij db, podmień plik, otwórz ponownie
	_ = s.DB.Close()
	if err := copyFile(safeFrom, s.Path); err != nil {
		bad(w, err)
		return
	}

	db, err := bolt.Open(s.Path, 0600, getOptimizedBoltOptions())
	if err != nil {
		bad(w, err)
		return
	}
	s.DB = db

	// Reset binlog after restore — forces followers to re-snapshot
	if s.Binlog != nil {
		if err := s.Binlog.Rotate(0); err != nil {
			log.Printf("Warning: failed to reset binlog after restore: %v", err)
		}
	}

	ok(w, map[string]string{"restored": body.From})
}

func (s *Server) handleTruncate(w http.ResponseWriter, r *http.Request) {
	var req TruncateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var bo binlog.BinlogOps
	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		bDocs := tx.Bucket([]byte("docs"))

		// Dla każdego dokumentu w kolekcji: utnij historię do N
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			dPtr, err := loadDoc(v)
			if err != nil {
				return err
			}
			d := *dPtr
			// Zbierz revety
			rc := bRev.Cursor()
			rp := storage.RevPrefix(req.Collection, d.ID)
			var revKeys [][]byte
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				cp := make([]byte, len(rk))
				copy(cp, rk)
				revKeys = append(revKeys, cp)
			}
			// jeśli trzeba ciąć
			if req.KeepRevs >= 0 && len(revKeys) > req.KeepRevs {
				// posortowane rosnąco po ts dzięki key; usuń najstarsze
				toDel := revKeys[:len(revKeys)-req.KeepRevs]
				for _, delk := range toDel {
					_ = bRev.Delete(delk)
					bo.Delete("rev", delk)
				}
			}
			// DropCache placeholder — jeśli trzymasz rendery, wyczyść je tutaj
			_ = req.DropCache
		}
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "truncated"})
}

// --- utils

func ok(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	w.WriteHeader(200)
	_, _ = w.Write(b) // #nosec G705 -- response write to http.ResponseWriter
}
func bad(w http.ResponseWriter, err error) {
	w.WriteHeader(400)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, err.Error()) // #nosec G705 -- response write to http.ResponseWriter
}

// handleHealth returns a simple health check response
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check if server has finished initialization
	if !s.Ready {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"status":"warming_up"}`))
		return
	}

	// Check if database is accessible
	err := s.DBView(func(tx *bolt.Tx) error {
		return nil
	})

	if err != nil {
		w.WriteHeader(503)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy","error":%q}`, err.Error())
		return
	}

	w.WriteHeader(200)
	_, _ = w.Write([]byte(`{"status":"healthy","mode":"` + string(s.Mode) + `"}`))
}

// handleComplianceStatus returns the ISO 27001 / SOC 2 production-guard
// state so operators (and the Panel) can see whether the server is
// running with the required security envelope.
func (s *Server) handleComplianceStatus(w http.ResponseWriter, r *http.Request) {
	missing := CheckProductionGuards()
	type missingEntry struct {
		EnvVar string `json:"envVar"`
		Want   string `json:"want"`
		Reason string `json:"reason"`
	}
	entries := make([]missingEntry, 0, len(missing))
	for _, m := range missing {
		entries = append(entries, missingEntry{EnvVar: m.EnvVar, Want: m.Want, Reason: m.Reason})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"production":   IsProduction(),
		"compliant":    len(missing) == 0,
		"missing":      entries,
		"missingCount": len(missing),
	})
}

func (s *Server) collectionChecksum(collection string) (string, int) {
	var checksum uint32
	var count int

	_ = s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		prefix := []byte("doc|" + collection + "|")
		c := bDocs.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			count++
			// Hash key + first 64 bytes of value (contains updatedAt in serialized form)
			h := crc32.ChecksumIEEE(k)
			if len(v) > 64 {
				h ^= crc32.ChecksumIEEE(v[:64])
			} else {
				h ^= crc32.ChecksumIEEE(v)
			}
			checksum ^= h
		}
		return nil
	})

	return fmt.Sprintf("%08x", checksum), count
}

func (s *Server) handleChecksum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, `{"error":"collection is required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	checksum, count := s.collectionChecksum(collection)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"collection":    collection,
		"checksum":      checksum,
		"documentCount": count,
	})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse raw JSON to detect which fields are present
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		bad(w, err)
		return
	}

	// Required fields
	var collection, key, lang string
	if v, ok := raw["collection"]; ok {
		_ = json.Unmarshal(v, &collection)
	}
	if v, ok := raw["key"]; ok {
		_ = json.Unmarshal(v, &key)
	}
	if v, ok := raw["lang"]; ok {
		_ = json.Unmarshal(v, &lang)
	}

	if collection == "" || key == "" || lang == "" {
		bad(w, errors.New("missing required fields: collection, key, lang"))
		return
	}

	// Check which optional fields are present
	_, hasMeta := raw["meta"]
	_, hasContent := raw["contentMd"]
	_, hasTTL := raw["ttl"]

	if !hasMeta && !hasContent && !hasTTL {
		bad(w, errors.New("no fields to update"))
		return
	}

	// Parse optional fields
	var newMeta map[string][]string
	if hasMeta {
		_ = json.Unmarshal(raw["meta"], &newMeta)
	}
	var newContent string
	if hasContent {
		_ = json.Unmarshal(raw["contentMd"], &newContent)
	}
	var newTTL int64
	if hasTTL {
		_ = json.Unmarshal(raw["ttl"], &newTTL)
	}

	// Auth check
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// Schema validation for meta update
	if hasMeta {
		if err := s.SchemaManager.Validate(collection, newMeta); err != nil {
			bad(w, err)
			return
		}
	}

	// Load existing doc, apply partial changes, save
	now := time.Now().Unix()
	var saved storage.Doc
	var bo binlog.BinlogOps
	var metaDidChange bool

	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		// Find existing doc
		docIDBytes := bByK.Get(storage.ByKeyKey(collection, key, lang))
		if docIDBytes == nil {
			return errors.New("not found")
		}

		v := bDocs.Get(storage.DocKey(collection, string(docIDBytes)))
		if v == nil {
			return errors.New("not found")
		}

		existing, err := loadDoc(v)
		if err != nil {
			return err
		}

		// Check TTL expiry
		if existing.ExpiresAt > 0 && existing.ExpiresAt < now {
			return errors.New("not found")
		}

		// Apply partial updates
		doc := *existing
		doc.UpdatedAt = now

		if hasMeta {
			metaDidChange = metadataChanged(doc.Meta, newMeta)
			doc.Meta = newMeta
		}
		if hasContent {
			doc.ContentMD = newContent
		}
		if hasTTL {
			if newTTL > 0 {
				doc.ExpiresAt = now + newTTL
			} else {
				doc.ExpiresAt = 0
			}
		}

		// Persist
		buf, err := marshalAndEncrypt(&doc, collection)
		if err != nil {
			return err
		}

		docKey := storage.DocKey(collection, doc.ID)
		if err := bDocs.Put(docKey, buf); err != nil {
			return err
		}
		bo.Put("docs", docKey, buf)

		// Reindex metadata if changed
		if metaDidChange {
			// Remove old meta index entries
			if existing.Meta != nil {
				for mk, vals := range existing.Meta {
					for _, mv := range vals {
						mkey := append(storage.MetaKeyPrefix(collection, mk, mv), []byte(doc.ID)...)
						_ = bIdx.Delete(mkey)
						bo.Delete("idxmeta", mkey)
					}
				}
			}
			// Add new meta index entries
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(storage.MetaKeyPrefix(collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Put(mkey, []byte("1")); err != nil {
						return err
					}
					bo.Put("idxmeta", mkey, []byte("1"))
				}
			}
		}

		// Save revision
		rkey := append(storage.RevPrefix(collection, doc.ID), []byte(fmt.Sprintf("%020d", now))...)
		if err := bRev.Put(rkey, buf); err != nil {
			return err
		}
		bo.Put("rev", rkey, buf)

		if s.CollectionManager != nil {
			if cfg, found := s.CollectionManager.Get(collection); found && cfg.MaxRevisions > 0 {
				if err := trimRevisions(tx, &bo, collection, doc.ID, cfg.MaxRevisions); err != nil {
					return err
				}
			}
		}

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		if err.Error() == "not found" {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		bad(w, err)
		return
	}

	// Post-update hooks
	if hasContent && s.EmbeddingWorker != nil && saved.ContentMD != "" {
		s.EmbeddingWorker.Enqueue(EmbeddingJob{
			Collection: collection,
			DocID:      saved.ID,
			ContentMD:  saved.ContentMD,
		})
	}

	if s.TTLManager != nil {
		if saved.ExpiresAt > 0 {
			_ = s.TTLManager.Set(collection, saved.ID, saved.ExpiresAt)
		} else if hasTTL {
			_ = s.TTLManager.Remove(collection, saved.ID)
		}
	}

	if hasContent && s.FTSIndex != nil && saved.ContentMD != "" {
		_ = s.FTSIndex.IndexWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		_ = s.FTSIndex.IndexPositionsWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		fields := map[string]string{"content": saved.ContentMD}
		for k, vals := range saved.Meta {
			if len(vals) > 0 {
				fields["meta."+k] = strings.Join(vals, " ")
			}
		}
		_ = s.FTSIndex.IndexFieldsWithLang(collection, saved.ID, fields, saved.Lang)
	}

	// Geo re-index on partial update. Mirrors the Add/Upsert path above:
	// if meta now contains a usable point, upsert it into both indexes;
	// otherwise drop any stale points.
	if s.GeoIndex != nil && s.GeoStore != nil {
		if lat, lng, ok := s.GeoIndex.AddFromMeta(collection, saved.ID, saved.Meta); ok {
			_ = s.GeoStore.Put(collection, saved.ID, lat, lng)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Add(collection, saved.ID, lat, lng)
			}
		} else {
			s.GeoIndex.Remove(collection, saved.ID)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Remove(collection, saved.ID)
			}
			_ = s.GeoStore.Delete(collection, saved.ID)
		}
	}

	if s.WebhookManager != nil {
		s.WebhookManager.Fire("doc.updated", collection, key, lang, &saved)
	}

	if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		go s.AutomationManager.EvaluateTriggers(collection, saved, "update")
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("doc_update")
	}

	ok(w, saved)
}

func (s *Server) handleDocMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	key := r.URL.Query().Get("key")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	if collection == "" || key == "" {
		http.Error(w, `{"error":"collection and key are required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var doc storage.Doc
	err := s.DBView(func(tx *bolt.Tx) error {
		bByK := tx.Bucket([]byte("bykey"))
		bDocs := tx.Bucket([]byte("docs"))
		docID := bByK.Get(storage.ByKeyKey(collection, key, lang))
		if docID == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(storage.DocKey(collection, string(docID)))
		if v == nil {
			return errors.New("not found")
		}
		d, err := loadDoc(v)
		if err != nil {
			return err
		}
		doc = *d
		return nil
	})
	if err != nil {
		if err.Error() == "not found" {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		bad(w, err)
		return
	}

	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Return metadata only (no contentMd)
	resp := map[string]interface{}{
		"key":       doc.Key,
		"lang":      doc.Lang,
		"meta":      doc.Meta,
		"addedAt":   doc.AddedAt,
		"updatedAt": doc.UpdatedAt,
	}
	if doc.ExpiresAt > 0 {
		resp["expiresAt"] = doc.ExpiresAt
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("doc_meta")
	}

	ok(w, resp)
}

func (s *Server) handleMetaKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, `{"error":"collection is required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	meta := make(map[string][]string)

	_ = s.DBView(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx == nil {
			return nil
		}

		prefix := []byte("meta|" + collection + "|")
		c := bIdx.Cursor()
		seen := make(map[string]map[string]bool)

		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			rest := string(k[len(prefix):])
			parts := strings.SplitN(rest, "|", 3)
			if len(parts) < 2 {
				continue
			}
			mk, mv := parts[0], parts[1]
			if seen[mk] == nil {
				seen[mk] = make(map[string]bool)
			}
			if !seen[mk][mv] {
				seen[mk][mv] = true
				meta[mk] = append(meta[mk], mv)
			}
		}
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	type CollectionStats struct {
		Name           string `json:"name"`
		DocumentCount  int    `json:"documentCount"`
		RevisionCount  int    `json:"revisionCount"`
		MetaIndexCount int    `json:"metaIndexCount"`
		Checksum       string `json:"checksum"`
		Type           string `json:"type,omitempty"`
		Description    string `json:"description,omitempty"`
		Icon           string `json:"icon,omitempty"`
		Color          string `json:"color,omitempty"`
	}

	// Check read permission (database-wide stats)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// IndexQueueStats surfaces async meta-indexing health (GO-010): how many
	// jobs were processed, failed, or had to be indexed synchronously because
	// the queue was full (fallbacks), plus the current queue depth.
	type IndexQueueStats struct {
		Processed uint64 `json:"processed"`
		Failed    uint64 `json:"failed"`
		Fallbacks uint64 `json:"fallbacks"`
		QueueLen  int    `json:"queueLen"`
	}

	type Stats struct {
		DatabasePath     string            `json:"databasePath"`
		DatabaseSize     int64             `json:"databaseSize"`
		Mode             string            `json:"mode"`
		Collections      []CollectionStats `json:"collections"`
		TotalDocuments   int               `json:"totalDocuments"`
		TotalRevisions   int               `json:"totalRevisions"`
		TotalMetaIndices int               `json:"totalMetaIndices"`
		IndexQueue       *IndexQueueStats  `json:"indexQueue,omitempty"`
		Uptime           string            `json:"uptime"`
	}

	stats := Stats{
		DatabasePath: s.Path,
		Mode:         string(s.Mode),
		Collections:  []CollectionStats{},
	}

	// Get database file size
	if info, err := os.Stat(s.Path); err == nil {
		stats.DatabaseSize = info.Size()
	}

	// Collect statistics per collection
	collectionMap := make(map[string]*CollectionStats)

	err := s.DBView(func(tx *bolt.Tx) error {
		// Count documents per collection
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs != nil {
			c := bDocs.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: doc|collection|id
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].DocumentCount++
					stats.TotalDocuments++
				}
			}
		}

		// Count revisions per collection
		bRev := tx.Bucket([]byte("rev"))
		if bRev != nil {
			c := bRev.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: rev|collection|docID|ts
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].RevisionCount++
					stats.TotalRevisions++
				}
			}
		}

		// Count meta indices per collection
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx != nil {
			c := bIdx.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: meta|collection|key|value|docID
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].MetaIndexCount++
					stats.TotalMetaIndices++
				}
			}
		}

		return nil
	})

	if err != nil {
		bad(w, err)
		return
	}

	// Compute checksums per collection
	for name, cs := range collectionMap {
		cs.Checksum, _ = s.collectionChecksum(name)
	}

	// Enrich with collection config attributes
	if s.CollectionManager != nil {
		for name, cs := range collectionMap {
			if cfg, ok := s.CollectionManager.Get(name); ok {
				cs.Type = cfg.Type
				cs.Description = cfg.Description
				cs.Icon = cfg.Icon
				cs.Color = cfg.Color
			}
		}
	}

	// Convert map to slice
	for _, cs := range collectionMap {
		stats.Collections = append(stats.Collections, *cs)
	}

	// Sort collections by name
	sort.Slice(stats.Collections, func(i, j int) bool {
		return stats.Collections[i].Name < stats.Collections[j].Name
	})

	// Async meta-indexing queue health
	if s.IndexQueue != nil {
		processed, failed, fallbacks, queueLen := s.IndexQueue.Stats()
		stats.IndexQueue = &IndexQueueStats{
			Processed: processed,
			Failed:    failed,
			Fallbacks: fallbacks,
			QueueLen:  queueLen,
		}
	}

	ok(w, stats)
}

// handleDelete deletes a single document from a collection
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if err := s.deleteDocumentInternal(req.Collection, req.Key, req.Lang); err != nil {
		bad(w, err)
		return
	}

	ok(w, map[string]interface{}{
		"status":     "deleted",
		"collection": req.Collection,
		"key":        req.Key,
		"lang":       req.Lang,
	})
}

// handleDeleteBatch deletes multiple documents in a single request.
func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string `json:"collection"`
		Documents  []struct {
			Key  string `json:"key"`
			Lang string `json:"lang"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if len(req.Documents) == 0 {
		bad(w, errors.New("missing documents"))
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var deleted, notFound, failed int
	var errs []string
	for _, d := range req.Documents {
		if d.Key == "" || d.Lang == "" {
			failed++
			errs = append(errs, "missing key or lang")
			continue
		}
		if err := s.deleteDocumentInternal(req.Collection, d.Key, d.Lang); err != nil {
			if strings.Contains(err.Error(), "not found") {
				notFound++
			} else {
				failed++
				errs = append(errs, err.Error())
			}
			continue
		}
		deleted++
	}

	ok(w, map[string]interface{}{
		"deleted":   deleted,
		"not_found": notFound,
		"failed":    failed,
		"errors":    errs,
	})
}

// handleDeleteCollection deletes all documents in a collection
func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	var req DeleteCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var deletedCount int
	var bo binlog.BinlogOps

	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		// Delete all documents in collection
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			// Load document to get metadata for index cleanup
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr

			// Delete document
			if err := bDocs.Delete(k); err != nil {
				return err
			}
			bo.Delete("docs", k)

			// Delete from bykey index
			bykKey := storage.ByKeyKey(req.Collection, doc.Key, doc.Lang)
			if err := bByK.Delete(bykKey); err != nil {
				return err
			}
			bo.Delete("bykey", bykKey)

			// Delete all revisions
			rc := bRev.Cursor()
			rp := storage.RevPrefix(req.Collection, doc.ID)
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				if err := bRev.Delete(rk); err != nil {
					return err
				}
				bo.Delete("rev", rk)
			}

			// Delete metadata indices
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(storage.MetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Delete(mkey); err != nil {
						return err
					}
					bo.Delete("idxmeta", mkey)
				}
			}

			deletedCount++
		}

		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}

	if err != nil {
		bad(w, err)
		return
	}

	// Clean up collection config
	if s.CollectionManager != nil {
		_ = s.CollectionManager.Delete(req.Collection)
	}

	// Clean up both geo indexes and persisted geo points for this collection.
	if s.GeoIndex != nil {
		s.GeoIndex.Clear(req.Collection)
	}
	if s.GeoHashIndex != nil {
		s.GeoHashIndex.Clear(req.Collection)
	}
	if s.GeoStore != nil {
		_ = s.GeoStore.DeleteCollection(req.Collection)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "deleted",
		"collection":   req.Collection,
		"deletedCount": deletedCount,
	}); err != nil {
		log.Printf("Error encoding delete collection response: %v", err)
	}
}
