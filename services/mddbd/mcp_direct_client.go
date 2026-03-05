package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
	proto "mddb/proto"
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

func (c *DirectClient) Health(ctx context.Context) (*MCPHealth, error) {
	err := c.server.DB.View(func(tx *bolt.Tx) error { return nil })
	if err != nil {
		return &MCPHealth{Status: "unhealthy", Mode: string(c.server.Mode)}, err
	}
	return &MCPHealth{Status: "healthy", Mode: string(c.server.Mode)}, nil
}

func (c *DirectClient) Stats(ctx context.Context) (*MCPStats, error) {
	stats := &MCPStats{
		DatabasePath: c.server.Path,
		Mode:         string(c.server.Mode),
	}

	if info, err := os.Stat(c.server.Path); err == nil {
		stats.DatabaseSize = info.Size()
	}

	collectionMap := make(map[string]*MCPCollectionStats)

	err := c.server.DB.View(func(tx *bolt.Tx) error {
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

func (c *DirectClient) Add(ctx context.Context, req *MCPAddRequest) (*MCPDocument, error) {
	saved, _, err := c.server.addDocument(req.Collection, req.Key, req.Lang, req.Meta, req.ContentMD, 0)
	if err != nil {
		return nil, err
	}
	doc := docToMCPDocument(saved)
	return &doc, nil
}

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

func (c *DirectClient) Get(ctx context.Context, req *MCPGetRequest) (*MCPDocument, error) {
	var doc Doc
	err := c.server.DB.View(func(tx *bolt.Tx) error {
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

func (c *DirectClient) Search(ctx context.Context, req *MCPSearchRequest) (*MCPSearchResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 50
	}

	type row struct{ Doc Doc }
	var rows []row

	err := c.server.DB.View(func(tx *bolt.Tx) error {
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

func (c *DirectClient) Delete(ctx context.Context, req *MCPDeleteRequest) error {
	return c.server.deleteDocumentInternal(req.Collection, req.Key, req.Lang)
}

func (c *DirectClient) DeleteCollection(ctx context.Context, req *MCPDeleteCollectionRequest) (*MCPDeleteCollectionResponse, error) {
	var deletedCount int

	err := c.server.DB.Update(func(tx *bolt.Tx) error {
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

func (c *DirectClient) Export(ctx context.Context, req *MCPExportRequest) (io.ReadCloser, error) {
	// Collect matching documents
	var docs []Doc

	err := c.server.DB.View(func(tx *bolt.Tx) error {
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

func (c *DirectClient) Backup(ctx context.Context, req *MCPBackupRequest) (*MCPBackupResponse, error) {
	dst := req.To
	if dst == "" {
		dst = fmt.Sprintf("backup-%d.db", time.Now().Unix())
	}
	if err := copyFile(c.server.Path, dst); err != nil {
		return nil, err
	}
	return &MCPBackupResponse{Backup: dst}, nil
}

func (c *DirectClient) Restore(ctx context.Context, req *MCPRestoreRequest) (*MCPRestoreResponse, error) {
	if req.From == "" {
		return nil, errors.New("missing from")
	}
	_ = c.server.DB.Close()
	if err := copyFile(req.From, c.server.Path); err != nil {
		return nil, err
	}
	db, err := bolt.Open(c.server.Path, 0600, getOptimizedBoltOptions())
	if err != nil {
		return nil, err
	}
	c.server.DB = db
	return &MCPRestoreResponse{Restored: req.From}, nil
}

func (c *DirectClient) Truncate(ctx context.Context, req *MCPTruncateRequest) (*MCPTruncateResponse, error) {
	err := c.server.DB.Update(func(tx *bolt.Tx) error {
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

	var results []VectorResult
	if len(req.FilterMeta) > 0 {
		allowedIDs := s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowedIDs) == 0 {
			resp := &MCPVectorSearchResponse{
				Results:   []MCPVectorSearchResult{},
				Algorithm: algo,
			}
			if s.Embedding != nil {
				resp.Model = s.Embedding.Model()
				resp.Dimensions = s.Embedding.Dimensions()
			}
			return resp, nil
		}
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold)
	}

	// Deduplicate chunk results: group by base docID, take max score
	results = DeduplicateChunkResults(results)
	if len(results) > topK {
		results = results[:topK]
	}

	items := make([]MCPVectorSearchResult, 0, len(results))
	_ = s.DB.View(func(tx *bolt.Tx) error {
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

	err := s.DB.View(func(tx *bolt.Tx) error {
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
	_ = s.DB.View(func(tx *bolt.Tx) error {
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

	saved, _, err := c.server.addDocument(req.Collection, key, req.Lang, mergedMeta, body, req.TTL)
	if err != nil {
		return nil, err
	}

	doc := docToMCPDocument(saved)
	return &doc, nil
}

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
	err := c.server.DB.Update(func(tx *bolt.Tx) error {
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

	resp := &MCPFTSSearchResponse{
		Algorithm: algo,
		Fuzzy:     fuzzy,
		Results:   make([]MCPFTSResult, 0, len(results)),
	}

	_ = c.server.DB.View(func(tx *bolt.Tx) error {
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
		FilterMeta:      req.FilterMeta,
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

	items := c.server.loadHybridDocs(req.Collection, merged, true)

	resp := &MCPHybridSearchResponse{
		Strategy:        httpReq.Strategy,
		FTSAlgorithm:    httpReq.Algorithm,
		VectorAlgorithm: httpReq.VectorAlgorithm,
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

func (c *DirectClient) DeleteWebhook(ctx context.Context, req *MCPDeleteWebhookRequest) error {
	if c.server.WebhookManager == nil {
		return errors.New("webhooks not initialized")
	}
	return c.server.WebhookManager.Delete(req.ID)
}

func (c *DirectClient) SetSchema(ctx context.Context, req *MCPSetSchemaRequest) error {
	if c.server.SchemaManager == nil {
		return errors.New("schema manager not initialized")
	}
	return c.server.SchemaManager.Set(req.Collection, req.Schema)
}

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

func (c *DirectClient) DeleteSchema(ctx context.Context, collection string) error {
	if c.server.SchemaManager == nil {
		return errors.New("schema manager not initialized")
	}
	return c.server.SchemaManager.Delete(collection)
}

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
