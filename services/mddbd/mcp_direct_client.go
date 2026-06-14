package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	proto "mddb/proto"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// DirectClient implements MCPClient by calling Server methods directly.
// This eliminates the gRPC/REST network hop that the old mddb-mcp service required.
type DirectClient struct {
	server *Server
}

// NewDirectClient creates a new DirectClient wrapping the given Server.
func NewDirectClient(s *Server) *DirectClient {
	return &DirectClient{server: s}
}

// Health checks if the database is healthy via the direct client.
func (c *DirectClient) Health(ctx context.Context) (*MCPHealth, error) {
	err := c.server.DBView(func(tx *bolt.Tx) error { return nil })
	if err != nil {
		return &MCPHealth{Status: "unhealthy", Mode: string(c.server.Mode)}, err
	}
	return &MCPHealth{Status: "healthy", Mode: string(c.server.Mode)}, nil
}

// Stats returns database statistics via the direct client.
func (c *DirectClient) Stats(ctx context.Context) (*MCPStats, error) {
	stats := &MCPStats{
		DatabasePath: c.server.Path,
		Mode:         string(c.server.Mode),
	}

	if info, err := os.Stat(c.server.Path); err == nil { // #nosec G703 -- path from server config
		stats.DatabaseSize = info.Size()
	}

	collectionMap := make(map[string]*MCPCollectionStats)

	err := c.server.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs != nil {
			c2 := bDocs.Cursor()
			for k, _ := c2.First(); k != nil; k, _ = c2.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &MCPCollectionStats{Name: coll}
					}
					collectionMap[coll].DocumentCount++
					stats.TotalDocuments++
				}
			}
		}

		bRev := tx.Bucket([]byte("rev"))
		if bRev != nil {
			c2 := bRev.Cursor()
			for k, _ := c2.First(); k != nil; k, _ = c2.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &MCPCollectionStats{Name: coll}
					}
					collectionMap[coll].RevisionCount++
					stats.TotalRevisions++
				}
			}
		}

		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx != nil {
			c2 := bIdx.Cursor()
			for k, _ := c2.First(); k != nil; k, _ = c2.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &MCPCollectionStats{Name: coll}
					}
					collectionMap[coll].MetaIndexCount++
					stats.TotalMetaIndices++
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, cs := range collectionMap {
		stats.Collections = append(stats.Collections, *cs)
	}
	sort.Slice(stats.Collections, func(i, j int) bool {
		return stats.Collections[i].Name < stats.Collections[j].Name
	})

	return stats, nil
}

// Add creates a new document via the direct client.
func (c *DirectClient) Add(ctx context.Context, req *MCPAddRequest) (*MCPDocument, error) {
	saved, _, err := c.server.addDocument(req.Collection, req.Key, req.Lang, req.Meta, req.ContentMD, 0, true)
	if err != nil {
		return nil, err
	}
	doc := docToMCPDocument(saved)
	return &doc, nil
}

// AddBatch creates multiple documents in a single operation via the direct client.
func (c *DirectClient) AddBatch(ctx context.Context, req *MCPAddBatchRequest) (*MCPAddBatchResponse, error) {
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

	var resp *proto.AddBatchResponse
	var err error

	if c.server.finalBatchProcessor != nil {
		resp, err = c.server.finalBatchProcessor.ProcessBatch(ctx, req.Collection, protoDocs)
	} else {
		bp := NewBatchProcessor(c.server, 8)
		resp, err = bp.ProcessBatch(ctx, req.Collection, protoDocs)
	}
	if err != nil {
		return nil, err
	}

	return &MCPAddBatchResponse{
		Added:   int(resp.Added),
		Updated: int(resp.Updated),
		Failed:  int(resp.Failed),
		Errors:  resp.Errors,
	}, nil
}

// UpdateBatch updates multiple documents in a single operation via the direct client.
func (c *DirectClient) UpdateBatch(ctx context.Context, req *MCPUpdateBatchRequest) (*MCPUpdateBatchResponse, error) {
	protoDocs := make([]*proto.UpdateDocument, len(req.Documents))
	for i, d := range req.Documents {
		protoDocs[i] = &proto.UpdateDocument{
			Key:          d.Key,
			Lang:         d.Lang,
			Meta:         toProtoMeta(d.Meta),
			ContentMd:    d.ContentMD,
			SaveRevision: d.SaveRevision,
		}
	}

	bu := NewBatchUpdater(c.server, 8)
	resp, err := bu.ProcessBatchUpdate(ctx, req.Collection, protoDocs)
	if err != nil {
		return nil, err
	}

	return &MCPUpdateBatchResponse{
		Updated:  int(resp.Updated),
		NotFound: int(resp.NotFound),
		Failed:   int(resp.Failed),
		Errors:   resp.Errors,
	}, nil
}

// DeleteBatch removes multiple documents in a single operation via the direct client.
func (c *DirectClient) DeleteBatch(ctx context.Context, req *MCPDeleteBatchRequest) (*MCPDeleteBatchResponse, error) {
	protoDocs := make([]*proto.DeleteDocument, len(req.Documents))
	for i, d := range req.Documents {
		protoDocs[i] = &proto.DeleteDocument{
			Key:  d.Key,
			Lang: d.Lang,
		}
	}

	bd := NewBatchDeleter(c.server, 8)
	resp, err := bd.ProcessBatchDelete(ctx, req.Collection, protoDocs)
	if err != nil {
		return nil, err
	}

	return &MCPDeleteBatchResponse{
		Deleted:  int(resp.Deleted),
		NotFound: int(resp.NotFound),
		Failed:   int(resp.Failed),
		Errors:   resp.Errors,
	}, nil
}

// Get retrieves a document via the direct client.
func (c *DirectClient) Get(ctx context.Context, req *MCPGetRequest) (*MCPDocument, error) {
	var doc Doc
	err := c.server.DBView(func(tx *bolt.Tx) error {
		bByK := tx.Bucket([]byte("bykey"))
		docID := bByK.Get(kByKey(req.Collection, req.Key, req.Lang))
		if docID == nil {
			return errors.New("not found")
		}
		bDocs := tx.Bucket([]byte("docs"))
		v := bDocs.Get(kDoc(req.Collection, string(docID)))
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
		return nil, err
	}

	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		return nil, errors.New("not found")
	}

	if len(req.Env) > 0 && doc.ContentMD != "" {
		doc.ContentMD = applyEnv(doc.ContentMD, req.Env)
	}

	result := docToMCPDocument(doc)
	return &result, nil
}

// Search queries documents via the direct client.
func (c *DirectClient) Search(ctx context.Context, req *MCPSearchRequest) (*MCPSearchResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 50
	}

	type row struct{ Doc Doc }
	var rows []row

	err := c.server.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		seen := make(map[string]bool)

		if len(req.FilterMeta) == 0 {
			c2 := bDocs.Cursor()
			prefix := []byte("doc|" + req.Collection + "|")
			for k, v := c2.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c2.Next() {
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
			var sets [][]string
			for mk, mvals := range req.FilterMeta {
				var ids []string
				for _, mv := range mvals {
					prefix := kMetaKeyPrefix(req.Collection, mk, mv)
					c2 := bIdx.Cursor()
					for k, _ := c2.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c2.Next() {
						id := string(k[len(prefix):])
						ids = append(ids, id)
					}
				}
				ids = unique(ids)
				sets = append(sets, ids)
			}
			ids := intersect(sets...)
			for _, id := range ids {
				if seen[id] {
					continue
				}
				seen[id] = true
				v := bDocs.Get(kDoc(req.Collection, id))
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
		return nil, err
	}

	// Sort
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

	// Paginate
	total := len(rows)
	start := req.Offset
	if start > total {
		start = total
	}
	end := start + req.Limit
	if end > total {
		end = total
	}

	docs := make([]MCPDocument, 0, end-start)
	for _, r := range rows[start:end] {
		docs = append(docs, docToMCPDocument(r.Doc))
	}

	return &MCPSearchResponse{
		Documents: docs,
		Total:     total,
	}, nil
}

// Delete removes a document via the direct client.
func (c *DirectClient) Delete(ctx context.Context, req *MCPDeleteRequest) error {
	return c.server.deleteDocumentInternal(req.Collection, req.Key, req.Lang)
}

// DeleteCollection removes an entire collection via the direct client.
func (c *DirectClient) DeleteCollection(ctx context.Context, req *MCPDeleteCollectionRequest) (*MCPDeleteCollectionResponse, error) {
	var deletedCount int

	err := c.server.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		cur := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr
			if err := bDocs.Delete(k); err != nil {
				return err
			}
			if err := bByK.Delete(kByKey(req.Collection, doc.Key, doc.Lang)); err != nil {
				return err
			}

			rc := bRev.Cursor()
			rp := kRevPrefix(req.Collection, doc.ID)
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				if err := bRev.Delete(rk); err != nil {
					return err
				}
			}

			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Delete(mkey); err != nil {
						return err
					}
				}
			}

			deletedCount++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &MCPDeleteCollectionResponse{Deleted: deletedCount}, nil
}

// Export exports documents from a collection via the direct client.
func (c *DirectClient) Export(ctx context.Context, req *MCPExportRequest) (io.ReadCloser, error) {
	// Collect matching documents
	var docs []Doc

	err := c.server.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))

		if len(req.FilterMeta) == 0 {
			cur := bDocs.Cursor()
			prefix := []byte("doc|" + req.Collection + "|")
			for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
				d, err := loadDoc(v)
				if err != nil {
					continue
				}
				docs = append(docs, *d)
			}
		} else {
			var sets [][]string
			for mk, mvals := range req.FilterMeta {
				var ids []string
				for _, mv := range mvals {
					prefix := kMetaKeyPrefix(req.Collection, mk, mv)
					cur := bIdx.Cursor()
					for k, _ := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cur.Next() {
						id := string(k[len(prefix):])
						ids = append(ids, id)
					}
				}
				ids = unique(ids)
				sets = append(sets, ids)
			}
			for _, id := range intersect(sets...) {
				v := bDocs.Get(kDoc(req.Collection, id))
				if v == nil {
					continue
				}
				d, err := loadDoc(v)
				if err != nil {
					continue
				}
				docs = append(docs, *d)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Build NDJSON stream
	var buf bytes.Buffer
	for _, d := range docs {
		b, _ := json.Marshal(d)
		buf.Write(b)
		buf.WriteByte('\n')
	}

	return io.NopCloser(&buf), nil
}

// Backup creates a database backup via the direct client.
func (c *DirectClient) Backup(ctx context.Context, req *MCPBackupRequest) (*MCPBackupResponse, error) {
	dst := req.To
	if dst == "" {
		dst = fmt.Sprintf("backup-%d.db", time.Now().Unix())
	}
	safeDst, err := safeBackupPath(dst, false)
	if err != nil {
		return nil, err
	}
	if err := copyFile(c.server.Path, safeDst); err != nil {
		return nil, err
	}
	return &MCPBackupResponse{Backup: safeDst}, nil
}

// Restore restores documents from a backup via the direct client.
func (c *DirectClient) Restore(ctx context.Context, req *MCPRestoreRequest) (*MCPRestoreResponse, error) {
	if req.From == "" {
		return nil, errors.New("missing from")
	}
	safeFrom, err := safeBackupPath(req.From, true)
	if err != nil {
		return nil, err
	}
	_ = c.server.DB.Close()
	if err := copyFile(safeFrom, c.server.Path); err != nil {
		return nil, err
	}
	db, err := bolt.Open(c.server.Path, 0600, getOptimizedBoltOptions())
	if err != nil {
		return nil, err
	}
	c.server.DB = db
	return &MCPRestoreResponse{Restored: safeFrom}, nil
}

// Truncate removes all documents from a collection via the direct client.
func (c *DirectClient) Truncate(ctx context.Context, req *MCPTruncateRequest) (*MCPTruncateResponse, error) {
	err := c.server.DBUpdate(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		bDocs := tx.Bucket([]byte("docs"))

		cur := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			dPtr, err := loadDoc(v)
			if err != nil {
				return err
			}
			d := *dPtr
			rc := bRev.Cursor()
			rp := kRevPrefix(req.Collection, d.ID)
			var revKeys [][]byte
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				cp := make([]byte, len(rk))
				copy(cp, rk)
				revKeys = append(revKeys, cp)
			}
			if req.KeepRevs >= 0 && len(revKeys) > req.KeepRevs {
				toDel := revKeys[:len(revKeys)-req.KeepRevs]
				for _, delk := range toDel {
					_ = bRev.Delete(delk)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &MCPTruncateResponse{Status: "truncated"}, nil
}

// VectorSearch performs a vector similarity search via the direct client.
func (c *DirectClient) VectorSearch(ctx context.Context, req *MCPVectorSearchRequest) (*MCPVectorSearchResponse, error) {
	s := c.server

	if req.Collection == "" {
		return nil, errors.New("missing collection")
	}
	if req.Query == "" && len(req.QueryVector) == 0 {
		return nil, errors.New("either query or queryVector is required")
	}

	algo := req.Algorithm
	if algo == "" {
		algo = "flat"
	}
	searcher, ok := s.VectorSearchers[algo]
	if !ok {
		return nil, errors.New("unknown algorithm: " + algo)
	}
	if !searcher.IsReady() {
		searcher = s.VectorSearchers["flat"]
		algo = "flat"
	}
	if !searcher.IsReady() {
		return nil, errors.New("vector index is loading, please retry")
	}

	var queryVector []float32
	if len(req.QueryVector) > 0 {
		queryVector = req.QueryVector
	} else if s.Embedding != nil {
		embedCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var err error
		queryVector, err = s.Embedding.Embed(embedCtx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to embed query: %w", err)
		}
	} else {
		return nil, errors.New("no embedding provider configured and no queryVector provided")
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	// Oversample for chunk deduplication
	searchTopK := topK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	metric := ResolveSimilarity(req.DistanceMetric)
	metricName := req.DistanceMetric
	if metricName == "" {
		metricName = "cosine"
	}

	var results []VectorResult
	if len(req.FilterMeta) > 0 {
		allowedIDs := s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowedIDs) == 0 {
			resp := &MCPVectorSearchResponse{
				Results:        []MCPVectorSearchResult{},
				Algorithm:      algo,
				DistanceMetric: metricName,
			}
			if s.Embedding != nil {
				resp.Model = s.Embedding.Model()
				resp.Dimensions = s.Embedding.Dimensions()
			}
			return resp, nil
		}
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs, metric)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold, metric)
	}

	// Deduplicate chunk results: group by base docID, take max score
	results = DeduplicateChunkResults(results)
	if len(results) > topK {
		results = results[:topK]
	}

	items := make([]MCPVectorSearchResult, 0, len(results))
	_ = s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		for rank, vr := range results {
			v := bDocs.Get(kDoc(req.Collection, vr.DocID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr
			if !req.IncludeContent {
				doc.ContentMD = ""
			}
			items = append(items, MCPVectorSearchResult{
				Document: docToMCPDocument(doc),
				Score:    vr.Score,
				Rank:     rank + 1,
			})
		}
		return nil
	})

	resp := &MCPVectorSearchResponse{
		Results:   items,
		Total:     len(items),
		Algorithm: algo,
	}
	if s.Embedding != nil {
		resp.Model = s.Embedding.Model()
		resp.Dimensions = s.Embedding.Dimensions()
	}

	return resp, nil
}

// VectorReindex rebuilds the vector index via the direct client.
func (c *DirectClient) VectorReindex(ctx context.Context, req *MCPVectorReindexRequest) (*MCPVectorReindexResponse, error) {
	s := c.server

	if req.Collection == "" {
		return nil, errors.New("missing collection")
	}
	if s.Embedding == nil {
		return nil, errors.New("no embedding provider configured")
	}

	type docEntry struct {
		ID        string
		ContentMD string
	}
	var docs []docEntry

	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		cur := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			d, err := loadDoc(v)
			if err != nil {
				continue
			}
			docs = append(docs, docEntry{ID: d.ID, ContentMD: d.ContentMD})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	embedded, skipped, failed := 0, 0, 0
	var errs []string

	for _, d := range docs {
		if d.ContentMD == "" {
			skipped++
			continue
		}

		if !req.Force {
			existing, err := s.VectorStore.Get(req.Collection, d.ID)
			if err == nil && existing != nil {
				currentHash := ContentHash(d.ContentMD)
				if existing.ContentHash == currentHash {
					skipped++
					continue
				}
			}
		}

		// Split into chunks
		chunkSize := envDefaultInt("MDDB_EMBEDDING_CHUNK_SIZE", 1500)
		chunkEnabled := envDefault("MDDB_EMBEDDING_CHUNK_ENABLED", "true") == "true"
		var chunks []string
		if chunkEnabled {
			chunks = ChunkText(d.ContentMD, chunkSize)
		} else {
			chunks = []string{d.ContentMD}
		}
		if len(chunks) == 0 {
			skipped++
			continue
		}

		// Embed each chunk
		var chunkEmbeddings []ChunkEmbedding
		chunkFailed := false
		for i, chunk := range chunks {
			embedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			vector, err := s.Embedding.Embed(embedCtx, chunk)
			cancel()
			if err != nil {
				failed++
				errs = append(errs, fmt.Sprintf("%s chunk %d: %s", d.ID, i, err.Error()))
				chunkFailed = true
				break
			}
			chunkEmbeddings = append(chunkEmbeddings, ChunkEmbedding{ChunkIndex: i, Vector: vector})
		}
		if chunkFailed {
			continue
		}

		contentHash := ContentHash(d.ContentMD)
		if err := s.VectorStore.PutChunks(req.Collection, d.ID, chunkEmbeddings, s.Embedding.Model(), contentHash); err != nil {
			failed++
			errs = append(errs, d.ID+": store: "+err.Error())
			continue
		}
		for _, ce := range chunkEmbeddings {
			chunkKey := fmt.Sprintf("%s#%d", d.ID, ce.ChunkIndex)
			for _, searcher := range s.VectorSearchers {
				searcher.Add(req.Collection, chunkKey, ce.Vector)
			}
		}
		s.VectorStore.CleanStaleChunks(req.Collection, d.ID, len(chunkEmbeddings), s.VectorIndex)
		embedded++
	}

	if embedded > 0 {
		if records, loadErr := s.VectorStore.LoadCollection(req.Collection); loadErr == nil {
			collVecs := make(map[string][]float32, len(records))
			for docID, rec := range records {
				collVecs[docID] = rec.Vector
			}
			for _, searcher := range s.VectorSearchers {
				if trainer, ok := searcher.(Trainable); ok {
					go trainer.Train(req.Collection, collVecs)
				}
			}
		}
	}

	return &MCPVectorReindexResponse{
		Embedded: embedded,
		Skipped:  skipped,
		Failed:   failed,
		Errors:   errs,
	}, nil
}

// VectorStats returns vector index statistics via the direct client.
func (c *DirectClient) VectorStats(ctx context.Context) (*MCPVectorStatsResponse, error) {
	s := c.server
	resp := &MCPVectorStatsResponse{
		Enabled:     s.Embedding != nil,
		Collections: make(map[string]MCPVectorCollectionStats),
	}

	if s.Embedding != nil {
		resp.Provider = s.Embedding.Model()
		resp.Model = s.Embedding.Model()
		resp.Dimensions = s.Embedding.Dimensions()
	}

	vectorCounts, _ := s.VectorStore.CountByCollection()

	docCounts := make(map[string]int)
	_ = s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		cur := bDocs.Cursor()
		for k, _ := cur.First(); k != nil; k, _ = cur.Next() {
			parts := splitKey(k)
			if len(parts) >= 2 {
				docCounts[parts[1]]++
			}
		}
		return nil
	})

	allColls := make(map[string]bool)
	for col := range docCounts {
		allColls[col] = true
	}
	for col := range vectorCounts {
		allColls[col] = true
	}

	for coll := range allColls {
		resp.Collections[coll] = MCPVectorCollectionStats{
			TotalDocuments:    docCounts[coll],
			EmbeddedDocuments: vectorCounts[coll],
		}
	}

	return resp, nil
}

// ImportURL imports a document from a URL via the direct client.
func (c *DirectClient) ImportURL(ctx context.Context, req *MCPImportURLRequest) (*MCPDocument, error) {
	if req.Collection == "" || req.URL == "" || req.Lang == "" {
		return nil, errors.New("missing required fields: collection, url, lang")
	}

	key := req.Key
	if key == "" {
		key = deriveKeyFromURL(req.URL)
		if key == "" {
			return nil, errors.New("cannot derive key from URL; provide key explicitly")
		}
	}

	content, err := fetchURL(req.URL)
	if err != nil {
		return nil, err
	}

	fmMeta, body := parseFrontmatter(content)
	mergedMeta := fmMeta
	if mergedMeta == nil {
		mergedMeta = make(map[string][]string)
	}
	for k, v := range req.Meta {
		mergedMeta[k] = v
	}

	saved, _, err := c.server.addDocument(req.Collection, key, req.Lang, mergedMeta, body, req.TTL, true)
	if err != nil {
		return nil, err
	}

	doc := docToMCPDocument(saved)
	return &doc, nil
}

// SetTTL sets a time-to-live on a document via the direct client.
func (c *DirectClient) SetTTL(ctx context.Context, req *MCPSetTTLRequest) (*MCPDocument, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, errors.New("missing required fields")
	}

	docID := genID(req.Collection, req.Key, req.Lang)

	now := time.Now().Unix()
	var expiresAt int64
	if req.TTL > 0 {
		expiresAt = now + req.TTL
	}

	var updated Doc
	err := c.server.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		dk := kDoc(req.Collection, docID)
		v := bDocs.Get(dk)
		if v == nil {
			return errors.New("document not found")
		}
		docPtr, err := loadDoc(v)
		if err != nil {
			return err
		}
		updated = *docPtr
		updated.ExpiresAt = expiresAt
		buf, err := marshalDoc(&updated)
		if err != nil {
			return err
		}
		return bDocs.Put(dk, buf)
	})
	if err != nil {
		return nil, err
	}

	if c.server.TTLManager != nil {
		if expiresAt > 0 {
			_ = c.server.TTLManager.Set(req.Collection, docID, expiresAt)
		} else {
			_ = c.server.TTLManager.Remove(req.Collection, docID)
		}
	}

	doc := docToMCPDocument(updated)
	return &doc, nil
}

// FTSSearch performs a full-text search via the direct client.
func (c *DirectClient) FTSSearch(ctx context.Context, req *MCPFTSSearchRequest) (*MCPFTSSearchResponse, error) {
	if req.Collection == "" || req.Query == "" {
		return nil, errors.New("missing required fields: collection, query")
	}
	if c.server.FTSIndex == nil {
		return nil, errors.New("full-text search not initialized")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	algo := req.Algorithm
	if algo == "" {
		algo = "tfidf"
	}
	fuzzy := req.Fuzzy
	if fuzzy < 0 {
		fuzzy = 0
	}
	if fuzzy > 2 {
		fuzzy = 2
	}

	var results []FTSResult
	var err error
	switch algo {
	case "bm25":
		if fuzzy > 0 {
			results, err = c.server.FTSIndex.SearchBM25Fuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = c.server.FTSIndex.SearchBM25(req.Collection, req.Query, limit)
		}
	case "tfidf":
		if fuzzy > 0 {
			results, err = c.server.FTSIndex.SearchFuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = c.server.FTSIndex.Search(req.Collection, req.Query, limit)
		}
	case "pmisparse":
		if fuzzy > 0 {
			results, err = c.server.FTSIndex.SearchPMISparseFuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = c.server.FTSIndex.SearchPMISparse(req.Collection, req.Query, limit)
		}
	default:
		return nil, fmt.Errorf("unknown algorithm: %s, available: tfidf, bm25, pmisparse", algo)
	}
	if err != nil {
		return nil, err
	}

	// Apply per-query boosts/demotions (metadata-based score multipliers).
	results = c.server.applyBoostFTS(req.Collection, results, req.Boost)

	resp := &MCPFTSSearchResponse{
		Algorithm: algo,
		Fuzzy:     fuzzy,
		Lang:      req.Lang,
		Results:   make([]MCPFTSResult, 0, len(results)),
	}

	_ = c.server.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		for _, res := range results {
			v := bDocs.Get(kDoc(req.Collection, res.DocID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			if docPtr.ExpiresAt > 0 && docPtr.ExpiresAt < time.Now().Unix() {
				continue
			}
			resp.Results = append(resp.Results, MCPFTSResult{
				Document:     docToMCPDocument(*docPtr),
				Score:        res.Score,
				MatchedTerms: res.MatchedTerms,
			})
		}
		return nil
	})
	resp.Total = len(resp.Results)

	return resp, nil
}

// FTSReindex re-indexes all documents in a collection using their lang field.
func (c *DirectClient) FTSReindex(ctx context.Context, req *MCPFTSReindexRequest) (*MCPFTSReindexResponse, error) {
	if req.Collection == "" {
		return nil, errors.New("missing required field: collection")
	}
	if c.server.FTSIndex == nil {
		return nil, errors.New("full-text search not initialized")
	}

	// Collect docs first (read tx), then index outside to avoid deadlock
	type reindexDoc struct {
		ID, ContentMD, Lang string
		Meta                map[string][]string
	}
	var docs []reindexDoc
	var skipped int
	_ = c.server.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		prefix := []byte("doc|" + req.Collection + "|")
		cur := bDocs.Cursor()
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			docPtr, err := loadDoc(v)
			if err != nil || docPtr.ContentMD == "" {
				skipped++
				continue
			}
			if docPtr.ExpiresAt > 0 && docPtr.ExpiresAt < time.Now().Unix() {
				skipped++
				continue
			}
			docs = append(docs, reindexDoc{docPtr.ID, docPtr.ContentMD, docPtr.Lang, docPtr.Meta})
		}
		return nil
	})

	reindexed := 0
	for _, d := range docs {
		_ = c.server.FTSIndex.IndexWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)
		_ = c.server.FTSIndex.IndexPositionsWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)
		fields := map[string]string{"content": d.ContentMD}
		for mk, vals := range d.Meta {
			if len(vals) > 0 {
				fields["meta."+mk] = strings.Join(vals, " ")
			}
		}
		_ = c.server.FTSIndex.IndexFieldsWithLang(req.Collection, d.ID, fields, d.Lang)
		reindexed++
	}

	return &MCPFTSReindexResponse{
		Status:    "ok",
		Reindexed: reindexed,
		Skipped:   skipped,
	}, nil
}

// FTSLanguages returns the list of supported FTS languages.
func (c *DirectClient) FTSLanguages(ctx context.Context) (*MCPFTSLanguagesResponse, error) {
	if c.server.FTSIndex == nil || c.server.FTSIndex.langRegistry == nil {
		return &MCPFTSLanguagesResponse{Languages: []MCPFTSLanguageInfo{}}, nil
	}

	var langs []MCPFTSLanguageInfo
	for _, code := range c.server.FTSIndex.langRegistry.Languages() {
		cfg := c.server.FTSIndex.langRegistry.Resolve(code)
		name := code
		if cfg != nil {
			name = cfg.Name
		}
		langs = append(langs, MCPFTSLanguageInfo{Code: code, Name: name})
	}

	return &MCPFTSLanguagesResponse{
		Languages:   langs,
		DefaultLang: c.server.FTSIndex.langRegistry.DefaultLang(),
	}, nil
}

// HybridSearch performs a combined full-text and vector search via the direct client.
func (c *DirectClient) HybridSearch(ctx context.Context, req *MCPHybridSearchRequest) (*MCPHybridSearchResponse, error) {
	if req.Collection == "" || req.Query == "" {
		return nil, errors.New("missing required fields: collection, query")
	}

	httpReq := HybridSearchRequest{
		Collection:      req.Collection,
		Query:           req.Query,
		TopK:            req.TopK,
		Algorithm:       req.Algorithm,
		VectorAlgorithm: req.VectorAlgorithm,
		Alpha:           req.Alpha,
		Strategy:        req.Strategy,
		RRFK:            req.RRFK,
		Fuzzy:           req.Fuzzy,
		Threshold:       req.Threshold,
		DistanceMetric:  req.DistanceMetric,
		FilterMeta:      req.FilterMeta,
		Boost:           req.Boost,
		Sort:            req.Sort,
		IncludeContent:  true,
	}

	// Run FTS
	ftsResults, err := c.server.runFTSSearch(httpReq)
	if err != nil {
		return nil, err
	}

	// Run vector
	vectorResults, err := c.server.runVectorSearch(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	// Defaults
	if httpReq.TopK <= 0 {
		httpReq.TopK = 10
	}
	if httpReq.Strategy == "" {
		httpReq.Strategy = "alpha"
	}
	if httpReq.RRFK <= 0 {
		httpReq.RRFK = 60
	}
	if httpReq.Strategy == "alpha" && httpReq.Alpha == 0 {
		httpReq.Alpha = 0.5
	}

	var merged []HybridSearchResultItem
	switch httpReq.Strategy {
	case "rrf":
		merged = mergeRRF(ftsResults, vectorResults, httpReq.RRFK, httpReq.TopK)
	default:
		merged = mergeAlpha(ftsResults, vectorResults, httpReq.Alpha, httpReq.TopK)
	}

	// Apply per-query boosts/demotions (metadata-based score multipliers).
	merged = c.server.applyBoostHybrid(req.Collection, merged, req.Boost)
	if len(merged) > httpReq.TopK {
		merged = merged[:httpReq.TopK]
	}

	items := c.server.loadHybridDocs(req.Collection, merged, true)

	distMetric := httpReq.DistanceMetric
	if distMetric == "" {
		distMetric = "cosine"
	}
	resp := &MCPHybridSearchResponse{
		Strategy:        httpReq.Strategy,
		FTSAlgorithm:    httpReq.Algorithm,
		VectorAlgorithm: httpReq.VectorAlgorithm,
		DistanceMetric:  distMetric,
		Results:         make([]MCPHybridSearchResult, 0, len(items)),
	}
	for _, item := range items {
		resp.Results = append(resp.Results, MCPHybridSearchResult{
			Document:      docToMCPDocument(item.Document),
			CombinedScore: item.CombinedScore,
			FTSScore:      item.FTSScore,
			VectorScore:   item.VectorScore,
			MatchedTerms:  item.MatchedTerms,
			Rank:          item.Rank,
		})
	}
	resp.Total = len(resp.Results)

	return resp, nil
}

// RegisterWebhook registers a new webhook via the direct client.
func (c *DirectClient) RegisterWebhook(ctx context.Context, req *MCPRegisterWebhookRequest) (*MCPWebhook, error) {
	if c.server.WebhookManager == nil {
		return nil, errors.New("webhooks not initialized")
	}
	wh, err := c.server.WebhookManager.Register(req.URL, req.Events, req.Collection)
	if err != nil {
		return nil, err
	}
	return &MCPWebhook{
		ID:         wh.ID,
		URL:        wh.URL,
		Events:     wh.Events,
		Collection: wh.Collection,
		CreatedAt:  wh.CreatedAt,
	}, nil
}

// ListWebhooks returns all registered webhooks via the direct client.
func (c *DirectClient) ListWebhooks(ctx context.Context) ([]MCPWebhook, error) {
	if c.server.WebhookManager == nil {
		return nil, errors.New("webhooks not initialized")
	}
	hooks := c.server.WebhookManager.List()
	result := make([]MCPWebhook, len(hooks))
	for i, wh := range hooks {
		result[i] = MCPWebhook(wh)
	}
	return result, nil
}

// DeleteWebhook removes a webhook via the direct client.
func (c *DirectClient) DeleteWebhook(ctx context.Context, req *MCPDeleteWebhookRequest) error {
	if c.server.WebhookManager == nil {
		return errors.New("webhooks not initialized")
	}
	return c.server.WebhookManager.Delete(req.ID)
}

// SetSchema sets a JSON schema for a collection via the direct client.
func (c *DirectClient) SetSchema(ctx context.Context, req *MCPSetSchemaRequest) error {
	if c.server.SchemaManager == nil {
		return errors.New("schema manager not initialized")
	}
	return c.server.SchemaManager.Set(req.Collection, req.Schema)
}

// GetSchema retrieves the JSON schema for a collection via the direct client.
func (c *DirectClient) GetSchema(ctx context.Context, collection string) (*MCPSchemaResponse, error) {
	if c.server.SchemaManager == nil {
		return nil, errors.New("schema manager not initialized")
	}
	raw, found := c.server.SchemaManager.Get(collection)
	return &MCPSchemaResponse{
		Collection: collection,
		Schema:     raw,
		Enabled:    found,
	}, nil
}

// DeleteSchema removes the JSON schema for a collection via the direct client.
func (c *DirectClient) DeleteSchema(ctx context.Context, collection string) error {
	if c.server.SchemaManager == nil {
		return errors.New("schema manager not initialized")
	}
	return c.server.SchemaManager.Delete(collection)
}

// ListSchemas returns all registered schemas via the direct client.
func (c *DirectClient) ListSchemas(ctx context.Context) (*MCPListSchemasResponse, error) {
	if c.server.SchemaManager == nil {
		return nil, errors.New("schema manager not initialized")
	}
	schemas := c.server.SchemaManager.List()
	result := &MCPListSchemasResponse{
		Schemas: make([]MCPSchemaInfo, 0, len(schemas)),
	}
	for col, raw := range schemas {
		result.Schemas = append(result.Schemas, MCPSchemaInfo{
			Collection: col,
			Schema:     raw,
		})
	}
	return result, nil
}

// ValidateDocument validates a document against its collection schema via the direct client.
func (c *DirectClient) ValidateDocument(ctx context.Context, req *MCPValidateRequest) (*MCPValidateResponse, error) {
	if c.server.SchemaManager == nil {
		return &MCPValidateResponse{Valid: true, Errors: []string{}}, nil
	}
	err := c.server.SchemaManager.Validate(req.Collection, req.Meta)
	if err != nil {
		parts := strings.Split(err.Error(), "; ")
		return &MCPValidateResponse{Valid: false, Errors: parts}, nil
	}
	return &MCPValidateResponse{Valid: true, Errors: []string{}}, nil
}

// UpdateDocument updates an existing document via the direct client.
func (c *DirectClient) UpdateDocument(ctx context.Context, req *MCPUpdateDocumentRequest) (*MCPDocument, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, errors.New("missing required fields: collection, key, lang")
	}

	hasMeta := req.Meta != nil
	hasContent := req.ContentMD != nil
	hasTTL := req.TTL != nil

	if !hasMeta && !hasContent && !hasTTL {
		return nil, errors.New("no fields to update")
	}

	now := time.Now().Unix()
	var saved Doc

	err := c.server.DBUpdate(func(tx *bolt.Tx) error {
		bByK := tx.Bucket([]byte("bykey"))
		bDocs := tx.Bucket([]byte("docs"))

		docIDBytes := bByK.Get(kByKey(req.Collection, req.Key, req.Lang))
		if docIDBytes == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(kDoc(req.Collection, string(docIDBytes)))
		if v == nil {
			return errors.New("not found")
		}
		existing, err := loadDoc(v)
		if err != nil {
			return err
		}

		doc := *existing
		doc.UpdatedAt = now

		if hasMeta {
			doc.Meta = req.Meta
		}
		if hasContent {
			doc.ContentMD = *req.ContentMD
		}
		if hasTTL {
			if *req.TTL > 0 {
				doc.ExpiresAt = now + *req.TTL
			} else {
				doc.ExpiresAt = 0
			}
		}

		buf, err := marshalAndEncrypt(&doc, req.Collection)
		if err != nil {
			return err
		}
		if err := bDocs.Put(kDoc(req.Collection, doc.ID), buf); err != nil {
			return err
		}

		saved = doc
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := docToMCPDocument(saved)
	return &result, nil
}

// GetDocumentMeta retrieves document metadata via the direct client.
func (c *DirectClient) GetDocumentMeta(ctx context.Context, req *MCPGetDocMetaRequest) (*MCPDocMetaResponse, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, errors.New("missing required fields: collection, key, lang")
	}

	var doc Doc
	err := c.server.DBView(func(tx *bolt.Tx) error {
		bByK := tx.Bucket([]byte("bykey"))
		bDocs := tx.Bucket([]byte("docs"))
		docID := bByK.Get(kByKey(req.Collection, req.Key, req.Lang))
		if docID == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(kDoc(req.Collection, string(docID)))
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
		return nil, err
	}

	return &MCPDocMetaResponse{
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      doc.Meta,
		AddedAt:   doc.AddedAt,
		UpdatedAt: doc.UpdatedAt,
		ExpiresAt: doc.ExpiresAt,
	}, nil
}

// Classify classifies a document via the direct client.
func (c *DirectClient) Classify(ctx context.Context, req *MCPClassifyRequest) (*MCPClassifyResponse, error) {
	resp, err := c.server.classifyDocument(ctx, req.Collection, req.Key, req.Lang, req.Text, req.Labels, req.TopK, req.Multi, req.Threshold)
	if err != nil {
		return nil, err
	}

	results := make([]MCPClassifyLabelScore, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = MCPClassifyLabelScore(r)
	}

	return &MCPClassifyResponse{
		Results:    results,
		Model:      resp.Model,
		Dimensions: resp.Dimensions,
	}, nil
}

// --- Synonyms ---

// ListSynonyms returns synonyms for a collection via the direct client.
func (c *DirectClient) ListSynonyms(ctx context.Context, collection string) (*MCPSynonymListResponse, error) {
	if c.server.SynonymManager == nil {
		return nil, errors.New("synonym manager not initialized")
	}
	m := c.server.SynonymManager.List(collection)
	entries := make([]MCPSynonymEntry, 0, len(m))
	for term, syns := range m {
		entries = append(entries, MCPSynonymEntry{Term: term, Synonyms: syns})
	}
	return &MCPSynonymListResponse{
		Collection: collection,
		Entries:    entries,
		Total:      len(entries),
	}, nil
}

// SetSynonym sets a synonym mapping via the direct client.
func (c *DirectClient) SetSynonym(ctx context.Context, collection, term string, synonyms []string) error {
	if c.server.SynonymManager == nil {
		return errors.New("synonym manager not initialized")
	}
	return c.server.SynonymManager.Set(collection, term, synonyms)
}

// DeleteSynonym removes a synonym mapping via the direct client.
func (c *DirectClient) DeleteSynonym(ctx context.Context, collection, term string) error {
	if c.server.SynonymManager == nil {
		return errors.New("synonym manager not initialized")
	}
	return c.server.SynonymManager.Delete(collection, term)
}

// --- Stop Words ---

// ListStopWords returns stop words for a collection via the direct client.
func (c *DirectClient) ListStopWords(ctx context.Context, collection string) (*MCPStopWordListResponse, error) {
	if c.server.StopWordManager == nil {
		return nil, errors.New("stop word manager not initialized")
	}
	defaults, custom := c.server.StopWordManager.List(collection)
	entries := make([]MCPStopWordEntry, 0, len(defaults)+len(custom))
	for _, w := range defaults {
		entries = append(entries, MCPStopWordEntry{Word: w, IsDefault: true})
	}
	for _, w := range custom {
		entries = append(entries, MCPStopWordEntry{Word: w, IsDefault: false})
	}
	return &MCPStopWordListResponse{
		Collection: collection,
		Entries:    entries,
		Total:      len(entries),
		Defaults:   len(defaults),
		Custom:     len(custom),
	}, nil
}

// AddStopWords adds stop words for a collection via the direct client.
func (c *DirectClient) AddStopWords(ctx context.Context, collection string, words []string) error {
	if c.server.StopWordManager == nil {
		return errors.New("stop word manager not initialized")
	}
	return c.server.StopWordManager.Add(collection, words)
}

// DeleteStopWords removes stop words from a collection via the direct client.
func (c *DirectClient) DeleteStopWords(ctx context.Context, collection string, words []string) error {
	if c.server.StopWordManager == nil {
		return errors.New("stop word manager not initialized")
	}
	var errs []string
	for _, w := range words {
		if err := c.server.StopWordManager.Delete(collection, w); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("some deletions failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// --- Meta Keys / Checksum ---

// GetMetaKeys returns all metadata keys for a collection via the direct client.
func (c *DirectClient) GetMetaKeys(ctx context.Context, collection string) (*MCPMetaKeysResponse, error) {
	meta := make(map[string][]string)
	_ = c.server.DBView(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx == nil {
			return nil
		}
		prefix := []byte("meta|" + collection + "|")
		cur := bIdx.Cursor()
		seen := make(map[string]map[string]bool)
		for k, _ := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cur.Next() {
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
	return &MCPMetaKeysResponse{Meta: meta}, nil
}

// GetChecksum returns a checksum for a collection via the direct client.
func (c *DirectClient) GetChecksum(ctx context.Context, collection string) (*MCPChecksumResponse, error) {
	checksum, count := c.server.collectionChecksum(collection)
	return &MCPChecksumResponse{
		Collection:    collection,
		Checksum:      checksum,
		DocumentCount: count,
	}, nil
}

// --- Automation ---

// ListAutomation returns automation rules via the direct client.
func (c *DirectClient) ListAutomation(ctx context.Context, filterType string) (*MCPAutomationListResponse, error) {
	if c.server.AutomationManager == nil {
		return nil, errors.New("automation not initialized")
	}
	rules := c.server.AutomationManager.List(filterType)
	return &MCPAutomationListResponse{Rules: rules, Total: len(rules)}, nil
}

// CreateAutomation creates a new automation rule via the direct client.
func (c *DirectClient) CreateAutomation(ctx context.Context, rule AutomationRule) (*AutomationRule, error) {
	if c.server.AutomationManager == nil {
		return nil, errors.New("automation not initialized")
	}
	return c.server.AutomationManager.Create(rule)
}

// GetAutomation retrieves an automation rule by ID via the direct client.
func (c *DirectClient) GetAutomation(ctx context.Context, id string) (*AutomationRule, error) {
	if c.server.AutomationManager == nil {
		return nil, errors.New("automation not initialized")
	}
	rule := c.server.AutomationManager.Get(id)
	if rule == nil {
		return nil, errors.New("not found")
	}
	return rule, nil
}

// UpdateAutomation updates an automation rule via the direct client.
func (c *DirectClient) UpdateAutomation(ctx context.Context, id string, rule AutomationRule) (*AutomationRule, error) {
	if c.server.AutomationManager == nil {
		return nil, errors.New("automation not initialized")
	}
	return c.server.AutomationManager.Update(id, rule)
}

// DeleteAutomation removes an automation rule via the direct client.
func (c *DirectClient) DeleteAutomation(ctx context.Context, id string) error {
	if c.server.AutomationManager == nil {
		return errors.New("automation not initialized")
	}
	return c.server.AutomationManager.Delete(id)
}

// TestAutomation runs an automation rule in test mode via the direct client.
func (c *DirectClient) TestAutomation(ctx context.Context, id string) (string, error) {
	if c.server.AutomationManager == nil {
		return "", errors.New("automation not initialized")
	}
	rule := c.server.AutomationManager.Get(id)
	if rule == nil {
		return "", errors.New("not found")
	}
	if rule.Type != "trigger" {
		return "", fmt.Errorf("can only test trigger rules, got: %s", rule.Type)
	}
	matches, err := c.server.AutomationManager.RunTrigger(rule)
	if err != nil {
		return "", err
	}
	resp := map[string]interface{}{
		"trigger": map[string]interface{}{
			"id":         rule.ID,
			"name":       rule.Name,
			"searchType": rule.SearchType,
			"query":      rule.Query,
			"threshold":  rule.Threshold,
		},
		"matches": matches,
		"total":   len(matches),
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

// ListAutomationLogs returns automation execution logs via the direct client.
func (c *DirectClient) ListAutomationLogs(ctx context.Context, limit int, cursor, ruleID, status string) (*MCPAutomationLogListResponse, error) {
	if c.server.AutomationLogStore == nil {
		return nil, errors.New("automation logs not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	logs, nextCursor, err := c.server.AutomationLogStore.List(limit, cursor, ruleID, status)
	if err != nil {
		return nil, err
	}
	total, _ := c.server.AutomationLogStore.Count(ruleID, status)
	return &MCPAutomationLogListResponse{
		Logs:       logs,
		Total:      total,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}

// ListRevisions returns document revision history via the direct client.
func (c *DirectClient) ListRevisions(ctx context.Context, collection, key, lang string) (*RevisionListResponse, error) {
	docID := genID(collection, key, lang)
	var revisions []RevisionEntry
	err := c.server.DBView(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		if bRev == nil {
			return nil
		}
		prefix := kRevPrefix(collection, docID)
		cur := bRev.Cursor()
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			lastPipe := bytes.LastIndexByte(k, '|')
			if lastPipe < 0 || lastPipe >= len(k)-1 {
				continue
			}
			ts, err := strconv.ParseInt(string(k[lastPipe+1:]), 10, 64)
			if err != nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			revisions = append(revisions, RevisionEntry{
				Timestamp: ts,
				UpdatedAt: docPtr.UpdatedAt,
				ContentMD: docPtr.ContentMD,
				Meta:      docPtr.Meta,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Timestamp > revisions[j].Timestamp
	})
	return &RevisionListResponse{
		Collection: collection,
		Key:        key,
		Lang:       lang,
		Revisions:  revisions,
		Total:      len(revisions),
	}, nil
}

// RestoreRevision restores a document to a previous revision via the direct client.
func (c *DirectClient) RestoreRevision(ctx context.Context, collection, key, lang string, timestamp int64) (*MCPDocument, error) {
	docID := genID(collection, key, lang)
	tsKey := fmt.Sprintf("%020d", timestamp)
	revKey := append(kRevPrefix(collection, docID), []byte(tsKey)...)

	var revDoc *Doc
	err := c.server.DBView(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		if bRev == nil {
			return fmt.Errorf("revision not found")
		}
		v := bRev.Get(revKey)
		if v == nil {
			return fmt.Errorf("revision not found for timestamp %d", timestamp)
		}
		var err error
		revDoc, err = loadDoc(v)
		return err
	})
	if err != nil {
		return nil, err
	}

	doc, _, err := c.server.addDocument(collection, key, lang, revDoc.Meta, revDoc.ContentMD, 0, true)
	if err != nil {
		return nil, err
	}
	mcpDoc := docToMCPDocument(doc)
	return &mcpDoc, nil
}

// --- Collection Config ---

// GetCollectionConfig retrieves configuration for a collection via the direct client.
func (c *DirectClient) GetCollectionConfig(ctx context.Context, collection string) (*MCPCollectionConfigResponse, error) {
	cfg, found := c.server.CollectionManager.Get(collection)
	if !found {
		cfg = &CollectionConfig{Type: "default"}
	}
	return &MCPCollectionConfigResponse{
		Collection: collection,
		Config:     cfg,
		Configured: found,
	}, nil
}

// SetCollectionConfig updates configuration for a collection via the direct client.
func (c *DirectClient) SetCollectionConfig(ctx context.Context, req *MCPSetCollectionConfigRequest) error {
	cfg := &CollectionConfig{
		Type:         req.Type,
		Description:  req.Description,
		Icon:         req.Icon,
		Color:        req.Color,
		CustomMeta:   req.CustomMeta,
		MaxRevisions: req.MaxRevisions,
	}
	return c.server.CollectionManager.Set(req.Collection, cfg)
}

// ListCurationRules returns all curation rules for a collection (or all collections if empty).
func (c *DirectClient) ListCurationRules(ctx context.Context, collection string) ([]*CurationRule, error) {
	if c.server.CurationManager == nil {
		return nil, fmt.Errorf("curation manager not initialized")
	}
	if collection == "" {
		return c.server.CurationManager.ListAll(), nil
	}
	return c.server.CurationManager.ListByCollection(collection), nil
}

// CreateCurationRule creates a new rule and returns it with its assigned id.
func (c *DirectClient) CreateCurationRule(ctx context.Context, rule *CurationRule) (*CurationRule, error) {
	if c.server.CurationManager == nil {
		return nil, fmt.Errorf("curation manager not initialized")
	}
	if rule == nil {
		return nil, fmt.Errorf("nil rule")
	}
	rule.ID = "" // always new on create
	if err := c.server.CurationManager.Set(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// UpdateCurationRule replaces a rule by id.
func (c *DirectClient) UpdateCurationRule(ctx context.Context, rule *CurationRule) (*CurationRule, error) {
	if c.server.CurationManager == nil {
		return nil, fmt.Errorf("curation manager not initialized")
	}
	if rule == nil || rule.ID == "" {
		return nil, fmt.Errorf("rule.id is required")
	}
	prev, exists := c.server.CurationManager.Get(rule.ID)
	if !exists {
		return nil, fmt.Errorf("rule %q not found", rule.ID)
	}
	if rule.Collection == "" {
		rule.Collection = prev.Collection
	}
	rule.CreatedAt = prev.CreatedAt
	if err := c.server.CurationManager.Set(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// DeleteCurationRule removes a rule by id.
func (c *DirectClient) DeleteCurationRule(ctx context.Context, id string) error {
	if c.server.CurationManager == nil {
		return fmt.Errorf("curation manager not initialized")
	}
	if _, exists := c.server.CurationManager.Get(id); !exists {
		return fmt.Errorf("rule %q not found", id)
	}
	return c.server.CurationManager.Delete(id)
}

// ListCollectionConfigs returns all collection configurations via the direct client.
func (c *DirectClient) ListCollectionConfigs(ctx context.Context) (*MCPCollectionConfigListResponse, error) {
	all := c.server.CollectionManager.ListAll()
	return &MCPCollectionConfigListResponse{
		Configs: all,
		Total:   len(all),
	}, nil
}

// --- Cross-Collection Search ---

// CrossSearch searches across multiple collections via the direct client.
func (c *DirectClient) CrossSearch(ctx context.Context, req *MCPCrossSearchRequest) (*MCPCrossSearchResponse, error) {
	s := c.server

	// Select algorithm
	algo := req.Algorithm
	if algo == "" {
		algo = "flat"
	}
	searcher, ok2 := s.VectorSearchers[algo]
	if !ok2 {
		return nil, fmt.Errorf("unknown algorithm: %s", algo)
	}
	if !searcher.IsReady() {
		searcher = s.VectorSearchers["flat"]
		algo = "flat"
	}
	if !searcher.IsReady() {
		return nil, fmt.Errorf("vector index not ready")
	}

	// Resolve query vector
	var queryVector []float32
	if req.SourceDocID != "" {
		if req.SourceCollection == "" {
			return nil, fmt.Errorf("sourceCollection required when using sourceDocID")
		}
		rec, err := s.VectorStore.Get(req.SourceCollection, req.SourceDocID)
		if err != nil || rec == nil {
			return nil, fmt.Errorf("source document has no embedding: %s/%s", req.SourceCollection, req.SourceDocID)
		}
		queryVector = rec.Vector
	} else if req.Query != "" && s.Embedding != nil {
		embedCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var err error
		queryVector, err = s.Embedding.Embed(embedCtx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("embedding failed: %w", err)
		}
	} else {
		return nil, fmt.Errorf("one of query or sourceDocID is required")
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}
	metric := ResolveSimilarity(req.DistanceMetric)
	metricName := req.DistanceMetric
	if metricName == "" {
		metricName = "cosine"
	}

	searchTopK := topK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	// Search each target collection
	type taggedResult struct {
		collection string
		result     VectorResult
	}
	var allTagged []taggedResult

	for _, coll := range req.TargetCollections {
		var results []VectorResult
		if len(req.FilterMeta) > 0 {
			allowedIDs := s.getDocIDsByMeta(coll, req.FilterMeta)
			if len(allowedIDs) == 0 {
				continue
			}
			results = searcher.SearchWithFilter(coll, queryVector, searchTopK, req.Threshold, allowedIDs, metric)
		} else {
			results = searcher.Search(coll, queryVector, searchTopK, req.Threshold, metric)
		}
		results = DeduplicateChunkResults(results)
		for _, vr := range results {
			allTagged = append(allTagged, taggedResult{collection: coll, result: vr})
		}
	}

	sort.Slice(allTagged, func(i, j int) bool {
		return allTagged[i].result.Score > allTagged[j].result.Score
	})
	if len(allTagged) > topK {
		allTagged = allTagged[:topK]
	}

	// Load full documents
	items := make([]CrossSearchResultItem, 0, len(allTagged))
	_ = s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		for rank, tr := range allTagged {
			v := bDocs.Get(kDoc(tr.collection, tr.result.DocID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr
			if !req.IncludeContent {
				doc.ContentMD = ""
			}
			items = append(items, CrossSearchResultItem{
				Collection: tr.collection,
				Document:   doc,
				Score:      tr.result.Score,
				Rank:       rank + 1,
			})
		}
		return nil
	})

	return &MCPCrossSearchResponse{
		Results:           items,
		Total:             len(items),
		SourceCollection:  req.SourceCollection,
		SourceDocID:       req.SourceDocID,
		TargetCollections: req.TargetCollections,
		Algorithm:         algo,
		DistanceMetric:    metricName,
	}, nil
}

// --- Find Duplicates ---

// FindDuplicates finds duplicate documents via the direct client.
func (c *DirectClient) FindDuplicates(ctx context.Context, req *MCPFindDuplicatesRequest) (*MCPFindDuplicatesResponse, error) {
	httpReq := FindDuplicatesRequest{
		Collection:     req.Collection,
		Mode:           req.Mode,
		Threshold:      req.Threshold,
		MaxDocs:        req.MaxDocs,
		DistanceMetric: req.DistanceMetric,
		IncludeContent: req.IncludeContent,
	}
	return c.server.findDuplicates(httpReq)
}

// Ingest bulk-imports documents via the direct client.
func (c *DirectClient) Ingest(ctx context.Context, req *MCPIngestRequest) (*MCPIngestResponse, error) {
	docs := make([]IngestDocumentHTTP, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = IngestDocumentHTTP(d)
	}

	opts := IngestOptionsHTTP{
		SkipDuplicates:          req.Options.SkipDuplicates,
		SkipEmbeddings:          req.Options.SkipEmbeddings,
		SkipFTS:                 req.Options.SkipFTS,
		SkipWebhooks:            req.Options.SkipWebhooks,
		AutoConfigureCollection: req.Options.AutoConfigureCollection,
		SaveRevision:            req.Options.SaveRevision,
	}

	resp, err := c.server.ingestDocuments(ctx, req.Collection, docs, opts)
	if err != nil {
		return nil, err
	}

	return &MCPIngestResponse{
		Added:      resp.Added,
		Updated:    resp.Updated,
		Skipped:    resp.Skipped,
		Failed:     resp.Failed,
		Errors:     resp.Errors,
		Collection: resp.Collection,
		DurationMs: resp.DurationMs,
	}, nil
}

// Aggregate performs aggregation queries via the direct client.
func (c *DirectClient) Aggregate(ctx context.Context, req *AggregateRequest) (*AggregateResponse, error) {
	return c.server.aggregate(req)
}

// Close is a no-op for the direct client since the server manages its own lifecycle.
func (c *DirectClient) Close() error {
	// No-op — Server owns all resources.
	return nil
}

// --- helpers ---

// toProtoMeta converts map[string][]string to proto MetaValues map.
func toProtoMeta(meta map[string][]string) map[string]*proto.MetaValues {
	if meta == nil {
		return nil
	}
	result := make(map[string]*proto.MetaValues, len(meta))
	for k, v := range meta {
		result[k] = &proto.MetaValues{Values: v}
	}
	return result
}
