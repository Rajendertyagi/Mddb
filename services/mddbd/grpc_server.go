package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	proto "mddb/proto"
)

// GRPCServer implements the MDDB gRPC service
type GRPCServer struct {
	proto.UnimplementedMDDBServer
	server         *Server
	batchProcessor *BatchProcessor
	batchUpdater   *BatchUpdater
	batchDeleter   *BatchDeleter
}

// NewGRPCServer creates a new gRPC server wrapper
func NewGRPCServer(s *Server) *GRPCServer {
	// Use FinalBatchProcessor if extreme mode, otherwise standard
	var batchProc *BatchProcessor
	if s.UseExtreme {
		// In extreme mode, use wrapper that calls FinalBatchProcessor
		batchProc = &BatchProcessor{
			server:     s,
			maxWorkers: 8,
		}
		// Override with final processor
		finalProc := NewFinalBatchProcessor(s, 8)
		// Store final processor for use
		s.finalBatchProcessor = finalProc
	} else {
		batchProc = NewBatchProcessor(s, 8)
	}

	gs := &GRPCServer{
		server:         s,
		batchProcessor: batchProc,
		batchDeleter:   NewBatchDeleter(s, 8), // 8 parallel workers
		batchUpdater:   NewBatchUpdater(s, 8), // 8 parallel workers
	}
	// Worker pool will be initialized when needed
	return gs
}

// isReadOnly returns true if the gRPC server is in read-only mode,
// considering both the global server mode and the per-protocol gRPC mode override.
func (g *GRPCServer) isReadOnly() bool {
	mode := effectiveMode(g.server.Mode, g.server.Config.GRPC.Mode)
	return mode == ModeRead
}

// startGRPCServer starts the gRPC server on the specified address.
// addr may be a TCP host:port or a Unix Domain Socket (unix:/path/to/sock) —
// see openListener in listen_addr.go.
func startGRPCServer(s *Server, addr string, opts ...grpc.ServerOption) error {
	lis, err := openListener(addr)
	if err != nil {
		return err
	}
	defer func() { _ = closeListener(lis, addr) }()

	// Default options
	defaultOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB
		grpc.MaxSendMsgSize(10 * 1024 * 1024), // 10MB
	}

	// Merge with provided options
	allOpts := append(defaultOpts, opts...)

	grpcServer := grpc.NewServer(allOpts...)

	proto.RegisterMDDBServer(grpcServer, NewGRPCServer(s))

	// Register replication service (available on all nodes for status queries)
	if s.ReplicationRole == "leader" || s.ReplicationRole == "follower" {
		rs := NewReplicationServer(s)
		s.replServer = rs
		proto.RegisterMDDBReplicationServer(grpcServer, rs)
	}

	// Register reflection service for grpcurl
	reflection.Register(grpcServer)

	return grpcServer.Serve(lis)
}

// Add implements the Add RPC
func (g *GRPCServer) Add(ctx context.Context, req *proto.AddRequest) (*proto.Document, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	// Convert proto meta to internal format
	meta := make(map[string][]string)
	for k, v := range req.Meta {
		meta[k] = v.Values
	}

	// Schema validation (opt-in)
	if err := g.server.SchemaManager.Validate(req.Collection, meta); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	now := time.Now().Unix()
	docID := genID(req.Collection, req.Key, req.Lang)

	// Use KeyBuilder for efficient key construction
	var kb KeyBuilder

	var saved Doc
	var cachedBuf []byte
	var bo BinlogOps
	err := g.server.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(g.server.BucketNames.Docs)
		bRev := tx.Bucket(g.server.BucketNames.Rev)
		bByK := tx.Bucket(g.server.BucketNames.ByKey)

		// Load existing
		existing := Doc{}
		docKey := kb.BuildDocKey(req.Collection, docID)
		if v := bDocs.Get(docKey); v != nil {
			existingDoc, err := unmarshalDoc(v)
			if err != nil {
				return err
			}
			existing = *existingDoc
		}
		added := existing.AddedAt
		if added == 0 {
			added = now
		}

		doc := Doc{
			ID: docID, Key: req.Key, Lang: req.Lang, Meta: meta,
			ContentMD: req.ContentMd, AddedAt: added, UpdatedAt: now,
		}
		buf, err := marshalDoc(&doc)
		if err != nil {
			return err
		}
		cachedBuf = buf // Save for cache

		// Use KeyBuilder for all keys
		docKey = kb.BuildDocKey(req.Collection, docID)
		if err := bDocs.Put(docKey, buf); err != nil {
			return err
		}
		bo.Put("docs", docKey, buf)

		byKeyKey := kb.BuildByKey(req.Collection, req.Key, req.Lang)
		if err := bByK.Put(byKeyKey, []byte(docID)); err != nil {
			return err
		}
		bo.Put("bykey", byKeyKey, []byte(docID))

		// Lazy metadata indexing - queue for async processing
		if metadataChanged(existing.Meta, doc.Meta) {
			g.server.IndexQueue.Enqueue(&IndexJob{
				Collection: req.Collection,
				DocID:      docID,
				OldMeta:    existing.Meta,
				NewMeta:    doc.Meta,
			})
		}

		// Revision (optional - only if requested)
		if req.SaveRevision {
			revKey := kb.BuildRevKey(req.Collection, doc.ID, now)
			if err := bRev.Put(revKey, buf); err != nil {
				return err
			}
			bo.Put("rev", revKey, buf)
		}

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(g.server.Binlog)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Update cache (use lock-free cache if extreme mode)
	cacheKey := BuildCacheKey(req.Collection, req.Key, req.Lang)
	if g.server.UseExtreme {
		g.server.LockFreeCache.Set(cacheKey, cachedBuf)
	} else {
		g.server.Cache.Set(cacheKey, cachedBuf)
	}

	// Trigger async embedding
	if g.server.EmbeddingWorker != nil && saved.ContentMD != "" {
		g.server.EmbeddingWorker.Enqueue(EmbeddingJob{
			Collection: req.Collection,
			DocID:      saved.ID,
			ContentMD:  saved.ContentMD,
		})
	}

	// Automation triggers
	if g.server.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		triggerEvent := "update"
		if saved.AddedAt == saved.UpdatedAt {
			triggerEvent = "insert"
		}
		go g.server.AutomationManager.EvaluateTriggers(req.Collection, saved, triggerEvent)
	}

	return docToProto(&saved), nil
}

// AddBatch implements the AddBatch RPC - adds multiple documents in a single transaction
// Uses parallel processing for preparation, then single transaction for commit
func (g *GRPCServer) AddBatch(ctx context.Context, req *proto.AddBatchRequest) (*proto.AddBatchResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if len(req.Documents) == 0 {
		return &proto.AddBatchResponse{}, nil
	}

	// Use final batch processor if extreme mode, otherwise standard
	var resp *proto.AddBatchResponse
	var err error

	if g.server.UseExtreme && g.server.finalBatchProcessor != nil {
		resp, err = g.server.finalBatchProcessor.ProcessBatch(ctx, req.Collection, req.Documents)
	} else {
		resp, err = g.batchProcessor.ProcessBatch(ctx, req.Collection, req.Documents)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return resp, nil
}

// Ingest implements the Ingest RPC — bulk ingest with URL key derivation, dedup, and auto-metadata.
func (g *GRPCServer) Ingest(ctx context.Context, req *proto.IngestRequest) (*proto.IngestResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if len(req.Documents) == 0 {
		return &proto.IngestResponse{Collection: req.Collection}, nil
	}

	resp, err := g.server.ingestDocumentsFromProto(ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return protoFromIngestResponse(resp), nil
}

// Get implements the Get RPC
func (g *GRPCServer) Get(ctx context.Context, req *proto.GetRequest) (*proto.Document, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// Check cache first (use lock-free cache if extreme mode)
	cacheKey := BuildCacheKey(req.Collection, req.Key, req.Lang)

	var cachedData []byte
	var found bool

	if g.server.UseExtreme {
		cachedData, found = g.server.LockFreeCache.Get(cacheKey)
	} else {
		cachedData, found = g.server.Cache.Get(cacheKey)
	}

	if found {
		docPtr, err := unmarshalDoc(cachedData)
		if err == nil {
			// Apply template variables if needed
			if len(req.Env) > 0 {
				docPtr.ContentMD = applyEnv(docPtr.ContentMD, req.Env)
			}
			return docToProto(docPtr), nil
		}
	}

	var doc Doc
	var docData []byte
	err := g.server.DB.View(func(tx *bolt.Tx) error {
		bByK := tx.Bucket(g.server.BucketNames.ByKey)
		bDocs := tx.Bucket(g.server.BucketNames.Docs)

		docID := bByK.Get(kByKey(req.Collection, req.Key, req.Lang))
		if docID == nil {
			return errors.New("not found")
		}

		v := bDocs.Get(kDoc(req.Collection, string(docID)))
		if v == nil {
			return errors.New("not found")
		}

		docData = make([]byte, len(v))
		copy(docData, v)

		docPtr, err := unmarshalDoc(v)
		if err != nil {
			return err
		}
		doc = *docPtr
		return nil
	})

	// Update cache (use lock-free cache if extreme mode)
	if err == nil && docData != nil {
		if g.server.UseExtreme {
			g.server.LockFreeCache.Set(cacheKey, docData)
		} else {
			g.server.Cache.Set(cacheKey, docData)
		}
	}

	if err != nil {
		if err.Error() == "not found" {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Apply template variables
	if len(req.Env) > 0 {
		doc.ContentMD = applyEnv(doc.ContentMD, req.Env)
	}

	return docToProto(&doc), nil
}

// Search implements the Search RPC
func (g *GRPCServer) Search(ctx context.Context, req *proto.SearchRequest) (*proto.SearchResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// Convert proto filter to internal format
	filterMeta := make(map[string][]string)
	for k, v := range req.FilterMeta {
		filterMeta[k] = v.Values
	}

	// Single transaction for both ID collection and document loading
	var docs []Doc
	var docIDs []string

	err := g.server.DB.View(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket(g.server.BucketNames.IdxMeta)
		bDocs := tx.Bucket(g.server.BucketNames.Docs)

		if len(filterMeta) == 0 {
			// No filter: scan all docs
			c := bDocs.Cursor()
			prefix := []byte("doc|" + req.Collection + "|")
			for k, _ := c.Seek(prefix); k != nil && BytesHasPrefix(k, prefix); k, _ = c.Next() {
				// Extract docID (3rd part) without string allocations
				if docID := ExtractPart(k, 2); docID != nil {
					docIDs = append(docIDs, string(docID))
				}
			}
		} else {
			// Filter by meta
			sets := [][]string{}
			for mk, mvs := range filterMeta {
				union := []string{}
				for _, mv := range mvs {
					c := bIdx.Cursor()
					prefix := kMetaKeyPrefix(req.Collection, mk, mv)
					for k, _ := c.Seek(prefix); k != nil && BytesHasPrefix(k, prefix); k, _ = c.Next() {
						// Extract docID (5th part) without string allocations
						if docID := ExtractPart(k, 4); docID != nil {
							union = append(union, string(docID))
						}
					}
				}
				sets = append(sets, unique(union))
			}
			docIDs = intersect(sets...)
		}

		// Load documents in the same transaction
		for _, id := range docIDs {
			v := bDocs.Get(kDoc(req.Collection, id))
			if v != nil {
				d, err := unmarshalDoc(v)
				if err == nil {
					docs = append(docs, *d)
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Sort
	sortField := req.Sort
	if sortField == "" {
		sortField = "updatedAt"
	}
	sortDocs(docs, sortField, req.Asc)

	// Pagination
	total := len(docs)
	offset := int(req.Offset)
	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	if offset > len(docs) {
		offset = len(docs)
	}
	end := offset + limit
	if end > len(docs) {
		end = len(docs)
	}
	docs = docs[offset:end]

	// Convert to proto
	protoDocs := make([]*proto.Document, len(docs))
	for i, doc := range docs {
		protoDocs[i] = docToProto(&doc)
	}

	return &proto.SearchResponse{
		Documents: protoDocs,
		Total:     safeInt32(total),
	}, nil
}

// Export implements the Export RPC (streaming)
func (g *GRPCServer) Export(req *proto.ExportRequest, stream proto.MDDB_ExportServer) error {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(stream.Context(), req.Collection, PermRead); err != nil {
			return status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// Similar to HTTP export but streaming chunks
	return status.Error(codes.Unimplemented, "export streaming not yet implemented")
}

// Backup implements the Backup RPC
func (g *GRPCServer) Backup(ctx context.Context, req *proto.BackupRequest) (*proto.BackupResponse, error) {
	// Check admin permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	filename := req.To
	if filename == "" {
		filename = fmt.Sprintf("backup-%d.db", time.Now().Unix())
	}

	err := g.server.DB.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(filename, 0600)
	})

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.BackupResponse{Backup: filename}, nil
}

// Restore implements the Restore RPC
func (g *GRPCServer) Restore(ctx context.Context, req *proto.RestoreRequest) (*proto.RestoreResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check admin permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.From == "" {
		return nil, status.Error(codes.InvalidArgument, "missing backup filename")
	}

	if err := copyFile(req.From, g.server.Path); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.RestoreResponse{Restored: req.From}, nil
}

// Truncate implements the Truncate RPC
func (g *GRPCServer) Truncate(ctx context.Context, req *proto.TruncateRequest) (*proto.TruncateResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	err := g.server.DB.Update(func(tx *bolt.Tx) error {
		bRev := tx.Bucket(g.server.BucketNames.Rev)
		bDocs := tx.Bucket(g.server.BucketNames.Docs)

		// Get all doc IDs in collection
		var docIDs []string
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			parts := strings.Split(string(k), "|")
			if len(parts) >= 3 {
				docIDs = append(docIDs, parts[2])
			}
		}

		// For each doc, keep only last N revisions
		for _, docID := range docIDs {
			var revKeys []string
			rc := bRev.Cursor()
			rprefix := kRevPrefix(req.Collection, docID)
			for k, _ := rc.Seek(rprefix); k != nil && strings.HasPrefix(string(k), string(rprefix)); k, _ = rc.Next() {
				revKeys = append(revKeys, string(k))
			}

			// Delete old revisions
			if len(revKeys) > int(req.KeepRevs) {
				toDelete := revKeys[:len(revKeys)-int(req.KeepRevs)]
				for _, k := range toDelete {
					_ = bRev.Delete([]byte(k))
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.TruncateResponse{Status: "truncated"}, nil
}

// Stats implements the Stats RPC
func (g *GRPCServer) Stats(ctx context.Context, req *proto.StatsRequest) (*proto.StatsResponse, error) {
	// Check admin permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	resp := &proto.StatsResponse{
		DatabasePath: g.server.Path,
		Mode:         string(g.server.Mode),
		Collections:  []*proto.CollectionStats{},
	}

	// Get database file size
	if info, err := os.Stat(g.server.Path); err == nil {
		resp.DatabaseSize = info.Size()
	}

	// Collect statistics
	collectionMap := make(map[string]*proto.CollectionStats)

	err := g.server.DB.View(func(tx *bolt.Tx) error {
		// Count documents
		bDocs := tx.Bucket(g.server.BucketNames.Docs)
		if bDocs != nil {
			c := bDocs.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &proto.CollectionStats{Name: coll}
					}
					collectionMap[coll].DocumentCount++
					resp.TotalDocuments++
				}
			}
		}

		// Count revisions
		bRev := tx.Bucket(g.server.BucketNames.Rev)
		if bRev != nil {
			c := bRev.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &proto.CollectionStats{Name: coll}
					}
					collectionMap[coll].RevisionCount++
					resp.TotalRevisions++
				}
			}
		}

		// Count meta indices
		bIdx := tx.Bucket(g.server.BucketNames.IdxMeta)
		if bIdx != nil {
			c := bIdx.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &proto.CollectionStats{Name: coll}
					}
					collectionMap[coll].MetaIndexCount++
					resp.TotalMetaIndices++
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert map to slice
	for _, cs := range collectionMap {
		resp.Collections = append(resp.Collections, cs)
	}

	return resp, nil
}

// VectorSearch implements the VectorSearch RPC
func (g *GRPCServer) VectorSearch(ctx context.Context, req *proto.VectorSearchRequest) (*proto.VectorSearchResponse, error) {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if req.Query == "" && len(req.QueryVector) == 0 {
		return nil, status.Error(codes.InvalidArgument, "either query or query_vector is required")
	}

	// Select algorithm
	algo := req.Algorithm
	if algo == "" {
		algo = "flat"
	}
	searcher, algoOk := g.server.VectorSearchers[algo]
	if !algoOk {
		return nil, status.Error(codes.InvalidArgument, "unknown algorithm: "+algo)
	}
	if !searcher.IsReady() {
		searcher = g.server.VectorSearchers["flat"]
		algo = "flat"
	}
	if !searcher.IsReady() {
		return nil, status.Error(codes.Unavailable, "vector index is loading")
	}

	// Get query vector
	var queryVector []float32
	if len(req.QueryVector) > 0 {
		queryVector = req.QueryVector
	} else if g.server.Embedding != nil {
		var err error
		queryVector, err = g.server.Embedding.Embed(ctx, req.Query)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to embed query: "+err.Error())
		}
	} else {
		return nil, status.Error(codes.FailedPrecondition, "no embedding provider configured")
	}

	topK := int(req.TopK)
	if topK <= 0 {
		topK = 5
	}

	// Convert proto filter
	filterMeta := make(map[string][]string)
	for k, v := range req.FilterMeta {
		filterMeta[k] = v.Values
	}

	// Oversample for chunk deduplication
	searchTopK := topK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	var results []VectorResult
	if len(filterMeta) > 0 {
		allowedIDs := g.server.getDocIDsByMeta(req.Collection, filterMeta)
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs, nil)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold, nil)
	}

	// Deduplicate chunk results
	results = DeduplicateChunkResults(results)
	if len(results) > topK {
		results = results[:topK]
	}

	// Load documents
	protoResults := make([]*proto.VectorSearchResult, 0, len(results))
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(g.server.BucketNames.Docs)
		for rank, vr := range results {
			v := bDocs.Get(kDoc(req.Collection, vr.DocID))
			if v == nil {
				continue
			}
			d, err := unmarshalDoc(v)
			if err != nil {
				continue
			}
			if !req.IncludeContent {
				d.ContentMD = ""
			}
			protoResults = append(protoResults, &proto.VectorSearchResult{
				Document: docToProto(d),
				Score:    vr.Score,
				Rank:     int32(rank + 1),
			})
		}
		return nil
	})

	resp := &proto.VectorSearchResponse{
		Results:   protoResults,
		Total:     safeInt32(len(protoResults)),
		Algorithm: algo,
	}
	if g.server.Embedding != nil {
		resp.Model = g.server.Embedding.Model()
		resp.Dimensions = safeInt32(g.server.Embedding.Dimensions())
	}

	return resp, nil
}

// VectorReindex implements the VectorReindex RPC
func (g *GRPCServer) VectorReindex(ctx context.Context, req *proto.VectorReindexRequest) (*proto.VectorReindexResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.Embedding == nil {
		return nil, status.Error(codes.FailedPrecondition, "no embedding provider configured")
	}

	// Collect documents
	type docEntry struct {
		ID        string
		ContentMD string
	}
	var docs []docEntry

	err := g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(g.server.BucketNames.Docs)
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			d, err := unmarshalDoc(v)
			if err != nil {
				continue
			}
			docs = append(docs, docEntry{ID: d.ID, ContentMD: d.ContentMD})
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var embedded, skipped, failed int32
	var errs []string

	for _, d := range docs {
		if d.ContentMD == "" {
			skipped++
			continue
		}
		if !req.Force {
			existing, err := g.server.VectorStore.Get(req.Collection, d.ID)
			if err == nil && existing != nil && existing.ContentHash == ContentHash(d.ContentMD) {
				skipped++
				continue
			}
		}

		vector, err := g.server.Embedding.Embed(ctx, d.ContentMD)
		if err != nil {
			failed++
			errs = append(errs, d.ID+": "+err.Error())
			continue
		}

		if err := g.server.VectorStore.Put(req.Collection, d.ID, vector, g.server.Embedding.Model(), ContentHash(d.ContentMD)); err != nil {
			failed++
			errs = append(errs, d.ID+": store: "+err.Error())
			continue
		}
		g.server.VectorIndex.Add(req.Collection, d.ID, vector)
		embedded++
	}

	return &proto.VectorReindexResponse{
		Embedded: embedded,
		Skipped:  skipped,
		Failed:   failed,
		Errors:   errs,
	}, nil
}

// VectorStats implements the VectorStats RPC
func (g *GRPCServer) VectorStats(ctx context.Context, req *proto.VectorStatsRequest) (*proto.VectorStatsResponse, error) {
	// Check admin permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	resp := &proto.VectorStatsResponse{
		Enabled:     g.server.Embedding != nil,
		Collections: make(map[string]*proto.VectorCollectionStats),
	}

	if g.server.Embedding != nil {
		resp.Provider = g.server.Embedding.Model()
		resp.Model = g.server.Embedding.Model()
		resp.Dimensions = safeInt32(g.server.Embedding.Dimensions())
	}

	vectorCounts, _ := g.server.VectorStore.CountByCollection()

	docCounts := make(map[string]int)
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(g.server.BucketNames.Docs)
		if bDocs == nil {
			return nil
		}
		c := bDocs.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			parts := strings.Split(string(k), "|")
			if len(parts) >= 2 {
				docCounts[parts[1]]++
			}
		}
		return nil
	})

	allColls := make(map[string]bool)
	for c := range docCounts {
		allColls[c] = true
	}
	for c := range vectorCounts {
		allColls[c] = true
	}

	for coll := range allColls {
		resp.Collections[coll] = &proto.VectorCollectionStats{
			TotalDocuments:    safeInt32(docCounts[coll]),
			EmbeddedDocuments: safeInt32(vectorCounts[coll]),
		}
	}

	return resp, nil
}

// Helper: convert internal Doc to proto Document
func docToProto(doc *Doc) *proto.Document {
	protoMeta := make(map[string]*proto.MetaValues)
	for k, v := range doc.Meta {
		protoMeta[k] = &proto.MetaValues{Values: v}
	}

	return &proto.Document{
		Id:        doc.ID,
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      protoMeta,
		ContentMd: doc.ContentMD,
		AddedAt:   doc.AddedAt,
		UpdatedAt: doc.UpdatedAt,
		ExpiresAt: doc.ExpiresAt,
	}
}

// DeleteBatch implements the DeleteBatch RPC - deletes multiple documents in a single transaction
func (g *GRPCServer) DeleteBatch(ctx context.Context, req *proto.DeleteBatchRequest) (*proto.DeleteBatchResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	resp, err := g.batchDeleter.ProcessBatchDelete(ctx, req.Collection, req.Documents)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return resp, nil
}

// UpdateBatch implements the UpdateBatch RPC - updates multiple documents in a single transaction
func (g *GRPCServer) UpdateBatch(ctx context.Context, req *proto.UpdateBatchRequest) (*proto.UpdateBatchResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	resp, err := g.batchUpdater.ProcessBatchUpdate(ctx, req.Collection, req.Documents)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return resp, nil
}

// ImportURL implements the ImportURL RPC
func (g *GRPCServer) ImportURL(ctx context.Context, req *proto.ImportURLRequest) (*proto.Document, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Url == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, url, lang")
	}

	key := req.Key
	if key == "" {
		key = deriveKeyFromURL(req.Url)
		if key == "" {
			return nil, status.Error(codes.InvalidArgument, "cannot derive key from URL; provide key explicitly")
		}
	}

	content, err := fetchURL(req.Url)
	if err != nil {
		return nil, status.Error(codes.Internal, "fetch failed: "+err.Error())
	}

	fmMeta, body := parseFrontmatter(content)
	mergedMeta := fmMeta
	if mergedMeta == nil {
		mergedMeta = make(map[string][]string)
	}
	for k, v := range req.Meta {
		mergedMeta[k] = v.Values
	}

	saved, _, err := g.server.addDocument(req.Collection, key, req.Lang, mergedMeta, body, req.Ttl)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return docToProto(&saved), nil
}

// SetTTL implements the SetTTL RPC
func (g *GRPCServer) SetTTL(ctx context.Context, req *proto.SetTTLRequest) (*proto.Document, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	docID := genID(req.Collection, req.Key, req.Lang)
	now := time.Now().Unix()
	var expiresAt int64
	if req.Ttl > 0 {
		expiresAt = now + req.Ttl
	}

	var updated Doc
	err := g.server.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		v := bDocs.Get(kDoc(req.Collection, docID))
		if v == nil {
			return fmt.Errorf("not found")
		}
		d, err := unmarshalDoc(v)
		if err != nil {
			return err
		}
		d.ExpiresAt = expiresAt
		buf, err := marshalDoc(d)
		if err != nil {
			return err
		}
		updated = *d
		return bDocs.Put(kDoc(req.Collection, docID), buf)
	})
	if err != nil {
		if err.Error() == "not found" {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if g.server.TTLManager != nil {
		if expiresAt > 0 {
			_ = g.server.TTLManager.Set(req.Collection, docID, expiresAt)
		} else {
			_ = g.server.TTLManager.Remove(req.Collection, docID)
		}
	}

	return docToProto(&updated), nil
}

// FTS implements the FTS RPC
func (g *GRPCServer) FTS(ctx context.Context, req *proto.FTSRequest) (*proto.FTSResponse, error) {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, query")
	}
	if g.server.FTSIndex == nil {
		return nil, status.Error(codes.FailedPrecondition, "full-text search not initialized")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	algo := req.Algorithm
	if algo == "" {
		algo = "tfidf"
	}

	fuzzy := int(req.Fuzzy)
	if fuzzy < 0 {
		fuzzy = 0
	}
	if fuzzy > 2 {
		fuzzy = 2
	}

	// Convert proto filter_meta to internal format
	var allowed map[string]bool
	if len(req.FilterMeta) > 0 {
		filterMeta := make(map[string][]string)
		for k, v := range req.FilterMeta {
			filterMeta[k] = v.Values
		}
		allowed = g.server.getDocIDsByMeta(req.Collection, filterMeta)
		if len(allowed) == 0 {
			return &proto.FTSResponse{
				Results:   nil,
				Total:     0,
				Algorithm: algo,
				Fuzzy:     int32(fuzzy),
			}, nil
		}
	}

	// Language-aware tokenization
	queryLang := req.Lang
	tokens := g.server.FTSIndex.TokenizeQueryLang(req.Collection, req.Query, queryLang)

	var results []FTSResult
	var err error
	switch algo {
	case "bm25f":
		if fuzzy > 0 {
			results, err = g.server.FTSIndex.SearchBM25FFuzzy(req.Collection, tokens, limit, fuzzy, nil)
		} else {
			results, err = g.server.FTSIndex.SearchBM25F(req.Collection, tokens, limit, nil)
		}
	case "bm25":
		if fuzzy > 0 {
			results, err = g.server.FTSIndex.SearchBM25Fuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = g.server.FTSIndex.SearchBM25(req.Collection, req.Query, limit)
		}
	case "tfidf":
		if fuzzy > 0 {
			results, err = g.server.FTSIndex.SearchFuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = g.server.FTSIndex.Search(req.Collection, req.Query, limit)
		}
	case "pmisparse":
		if fuzzy > 0 {
			results, err = g.server.FTSIndex.SearchPMISparseFuzzy(req.Collection, req.Query, limit, fuzzy)
		} else {
			results, err = g.server.FTSIndex.SearchPMISparse(req.Collection, req.Query, limit)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown algorithm: "+algo+", available: tfidf, bm25, bm25f, pmisparse")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Apply metadata filter if provided
	if allowed != nil {
		filtered := results[:0]
		for _, r := range results {
			if allowed[r.DocID] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Apply per-query boosts/demotions from request.
	results = g.server.applyBoostFTS(req.Collection, results, req.Boost)

	var protoResults []*proto.FTSResult
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		for _, res := range results {
			v := bDocs.Get(kDoc(req.Collection, res.DocID))
			if v == nil {
				continue
			}
			d, err := unmarshalDoc(v)
			if err != nil {
				continue
			}
			if d.ExpiresAt > 0 && d.ExpiresAt < time.Now().Unix() {
				continue
			}
			protoResults = append(protoResults, &proto.FTSResult{
				Document:     docToProto(d),
				Score:        res.Score,
				MatchedTerms: res.MatchedTerms,
			})
		}
		return nil
	})

	return &proto.FTSResponse{
		Results:   protoResults,
		Total:     safeInt32(len(protoResults)),
		Algorithm: algo,
		Fuzzy:     int32(fuzzy),
		Lang:      queryLang,
	}, nil
}

// FTSReindex implements the FTSReindex RPC — re-indexes all documents using their lang field.
func (g *GRPCServer) FTSReindex(ctx context.Context, req *proto.FTSReindexRequest) (*proto.FTSReindexResponse, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required field: collection")
	}
	if g.server.FTSIndex == nil {
		return nil, status.Error(codes.FailedPrecondition, "full-text search not initialized")
	}

	// Collect docs first (read tx), then index outside to avoid deadlock
	type reindexDoc struct {
		ID, ContentMD, Lang string
		Meta                map[string][]string
	}
	var docs []reindexDoc
	var skipped int
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		prefix := []byte("doc|" + req.Collection + "|")
		c := bDocs.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
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
		_ = g.server.FTSIndex.IndexWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)
		_ = g.server.FTSIndex.IndexPositionsWithLang(req.Collection, d.ID, d.ContentMD, d.Lang)
		fields := map[string]string{"content": d.ContentMD}
		for mk, vals := range d.Meta {
			if len(vals) > 0 {
				fields["meta."+mk] = strings.Join(vals, " ")
			}
		}
		_ = g.server.FTSIndex.IndexFieldsWithLang(req.Collection, d.ID, fields, d.Lang)
		reindexed++
	}

	return &proto.FTSReindexResponse{
		Status:    "ok",
		Reindexed: safeInt32(reindexed),
		Skipped:   safeInt32(skipped),
	}, nil
}

// FTSLanguages implements the FTSLanguages RPC — returns supported languages.
func (g *GRPCServer) FTSLanguages(ctx context.Context, _ *proto.FTSLanguagesRequest) (*proto.FTSLanguagesResponse, error) {
	if g.server.FTSIndex == nil || g.server.FTSIndex.langRegistry == nil {
		return &proto.FTSLanguagesResponse{}, nil
	}

	var langs []*proto.FTSLanguageInfo
	for _, code := range g.server.FTSIndex.langRegistry.Languages() {
		cfg := g.server.FTSIndex.langRegistry.Resolve(code)
		name := code
		if cfg != nil {
			name = cfg.Name
		}
		langs = append(langs, &proto.FTSLanguageInfo{Code: code, Name: name})
	}

	return &proto.FTSLanguagesResponse{
		Languages:   langs,
		DefaultLang: g.server.FTSIndex.langRegistry.DefaultLang(),
	}, nil
}

// HybridSearch implements the HybridSearch RPC - combines FTS and vector search
func (g *GRPCServer) HybridSearch(ctx context.Context, req *proto.HybridSearchRequest) (*proto.HybridSearchResponse, error) {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, query")
	}

	// Defaults
	topK := int(req.TopK)
	if topK <= 0 {
		topK = 10
	}
	algo := req.Algorithm
	if algo == "" {
		algo = "bm25"
	}
	vectorAlgo := req.VectorAlgorithm
	if vectorAlgo == "" {
		vectorAlgo = "flat"
	}
	strategy := req.Strategy
	if strategy == "" {
		strategy = "alpha"
	}
	alpha := req.Alpha
	if strategy == "alpha" && alpha == 0 {
		alpha = 0.5
	}
	rrfK := int(req.RrfK)
	if rrfK <= 0 {
		rrfK = 60
	}
	fuzzy := int(req.Fuzzy)
	if fuzzy < 0 {
		fuzzy = 0
	}
	if fuzzy > 2 {
		fuzzy = 2
	}

	// Validate strategy
	if strategy != "alpha" && strategy != "rrf" {
		return nil, status.Error(codes.InvalidArgument, "unknown strategy: "+strategy+", available: alpha, rrf")
	}

	// Convert proto filter_meta to internal format
	filterMeta := make(map[string][]string)
	for k, v := range req.FilterMeta {
		filterMeta[k] = v.Values
	}

	// Convert proto field_weights to internal format
	var fieldWeights map[string]float64
	if len(req.FieldWeights) > 0 {
		fieldWeights = req.FieldWeights
	}

	// Build internal hybrid search request
	hybridReq := HybridSearchRequest{
		Collection:      req.Collection,
		Query:           req.Query,
		TopK:            topK,
		Algorithm:       algo,
		VectorAlgorithm: vectorAlgo,
		Alpha:           alpha,
		Strategy:        strategy,
		RRFK:            rrfK,
		Fuzzy:           fuzzy,
		Threshold:       req.Threshold,
		FilterMeta:      filterMeta,
		IncludeContent:  req.IncludeContent,
		FieldWeights:    fieldWeights,
		Lang:            req.Lang,
		Boost:           req.Boost,
	}

	// Step 1: Run FTS search
	ftsResults, err := g.server.runFTSSearch(hybridReq)
	if err != nil {
		return nil, status.Error(codes.Internal, "FTS search failed: "+err.Error())
	}

	// Step 2: Run vector search
	vectorResults, err := g.server.runVectorSearch(ctx, hybridReq)
	if err != nil {
		return nil, status.Error(codes.Internal, "vector search failed: "+err.Error())
	}

	// Step 3: Merge results
	var merged []HybridSearchResultItem
	switch strategy {
	case "rrf":
		merged = mergeRRF(ftsResults, vectorResults, rrfK, topK)
	default: // "alpha"
		merged = mergeAlpha(ftsResults, vectorResults, alpha, topK)
	}

	// Apply per-query boosts/demotions (metadata-based score multipliers).
	merged = g.server.applyBoostHybrid(req.Collection, merged, req.Boost)
	if len(merged) > topK {
		merged = merged[:topK]
	}

	// Step 4: Load full documents
	items := g.server.loadHybridDocs(req.Collection, merged, req.IncludeContent)

	// Convert to proto response
	protoResults := make([]*proto.HybridSearchResult, len(items))
	for i, item := range items {
		protoResults[i] = &proto.HybridSearchResult{
			Document:      docToProto(&item.Document),
			CombinedScore: item.CombinedScore,
			FtsScore:      item.FTSScore,
			VectorScore:   item.VectorScore,
			MatchedTerms:  item.MatchedTerms,
			Rank:          safeInt32(item.Rank),
		}
	}

	return &proto.HybridSearchResponse{
		Results:         protoResults,
		Total:           safeInt32(len(protoResults)),
		Strategy:        strategy,
		FtsAlgorithm:    algo,
		VectorAlgorithm: vectorAlgo,
	}, nil
}

// RegisterWebhook implements the RegisterWebhook RPC
func (g *GRPCServer) RegisterWebhook(ctx context.Context, req *proto.RegisterWebhookRequest) (*proto.WebhookProto, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if g.server.WebhookManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "webhooks not initialized")
	}

	wh, err := g.server.WebhookManager.Register(req.Url, req.Events, req.Collection)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &proto.WebhookProto{
		Id:         wh.ID,
		Url:        wh.URL,
		Events:     wh.Events,
		Collection: wh.Collection,
		CreatedAt:  wh.CreatedAt,
	}, nil
}

// ListWebhooks implements the ListWebhooks RPC
func (g *GRPCServer) ListWebhooks(ctx context.Context, req *proto.ListWebhooksRequest) (*proto.ListWebhooksResponse, error) {
	// Check admin permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if g.server.WebhookManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "webhooks not initialized")
	}

	hooks := g.server.WebhookManager.List()
	protoHooks := make([]*proto.WebhookProto, len(hooks))
	for i, h := range hooks {
		protoHooks[i] = &proto.WebhookProto{
			Id:         h.ID,
			Url:        h.URL,
			Events:     h.Events,
			Collection: h.Collection,
			CreatedAt:  h.CreatedAt,
		}
	}

	return &proto.ListWebhooksResponse{Webhooks: protoHooks}, nil
}

// DeleteWebhook implements the DeleteWebhook RPC
func (g *GRPCServer) DeleteWebhook(ctx context.Context, req *proto.DeleteWebhookRequest) (*proto.DeleteWebhookResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission (using "*" since we need to find the webhook's collection)
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if g.server.WebhookManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "webhooks not initialized")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "missing id")
	}

	if err := g.server.WebhookManager.Delete(req.Id); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.DeleteWebhookResponse{Status: "deleted", Id: req.Id}, nil
}

// SetSchema implements the SetSchema RPC
func (g *GRPCServer) SetSchema(ctx context.Context, req *proto.SetSchemaRequest) (*proto.SetSchemaResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if req.Schema == "" {
		return nil, status.Error(codes.InvalidArgument, "missing schema")
	}
	if err := g.server.SchemaManager.Set(req.Collection, req.Schema); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &proto.SetSchemaResponse{Status: "ok"}, nil
}

// GetSchema implements the GetSchema RPC
func (g *GRPCServer) GetSchema(ctx context.Context, req *proto.GetSchemaRequest) (*proto.GetSchemaResponse, error) {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	raw, found := g.server.SchemaManager.Get(req.Collection)
	return &proto.GetSchemaResponse{
		Collection: req.Collection,
		Schema:     raw,
		Enabled:    found,
	}, nil
}

// DeleteSchema implements the DeleteSchema RPC
func (g *GRPCServer) DeleteSchema(ctx context.Context, req *proto.DeleteSchemaRequest) (*proto.DeleteSchemaResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	// Check write permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if err := g.server.SchemaManager.Delete(req.Collection); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.DeleteSchemaResponse{Status: "ok"}, nil
}

// ListSchemas implements the ListSchemas RPC
func (g *GRPCServer) ListSchemas(ctx context.Context, req *proto.ListSchemasRequest) (*proto.ListSchemasResponse, error) {
	// Check read permission for listing schemas (using "*" since it's a global operation)
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	schemas := g.server.SchemaManager.List()
	var result []*proto.SchemaInfo
	for col, raw := range schemas {
		result = append(result, &proto.SchemaInfo{
			Collection: col,
			Schema:     raw,
		})
	}
	return &proto.ListSchemasResponse{Schemas: result}, nil
}

// ValidateDocument implements the ValidateDocument RPC
func (g *GRPCServer) ValidateDocument(ctx context.Context, req *proto.ValidateDocumentRequest) (*proto.ValidateDocumentResponse, error) {
	// Check read permission
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	meta := make(map[string][]string)
	for k, v := range req.Meta {
		meta[k] = v.Values
	}
	err := g.server.SchemaManager.Validate(req.Collection, meta)
	if err != nil {
		parts := strings.SplitAfter(err.Error(), ": ")
		var errMsgs []string
		if len(parts) > 1 {
			errMsgs = strings.Split(parts[len(parts)-1], "; ")
		} else {
			errMsgs = []string{err.Error()}
		}
		return &proto.ValidateDocumentResponse{Valid: false, Errors: errMsgs}, nil
	}
	return &proto.ValidateDocumentResponse{Valid: true}, nil
}

// UpdateDocument implements the UpdateDocument RPC - partial document update
func (g *GRPCServer) UpdateDocument(ctx context.Context, req *proto.UpdateDocumentRequest) (*proto.Document, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}

	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, key, lang")
	}

	if !req.UpdateMeta && !req.UpdateContent && !req.UpdateTtl {
		return nil, status.Error(codes.InvalidArgument, "no fields to update")
	}

	// Convert proto meta
	var newMeta map[string][]string
	if req.UpdateMeta {
		newMeta = make(map[string][]string)
		for k, v := range req.Meta {
			newMeta[k] = v.Values
		}
		if err := g.server.SchemaManager.Validate(req.Collection, newMeta); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	now := time.Now().Unix()
	var saved Doc
	var metaDidChange bool
	var bo BinlogOps

	err := g.server.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

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

		if req.UpdateMeta {
			metaDidChange = metadataChanged(doc.Meta, newMeta)
			doc.Meta = newMeta
		}
		if req.UpdateContent {
			doc.ContentMD = req.ContentMd
		}
		if req.UpdateTtl {
			if req.Ttl > 0 {
				doc.ExpiresAt = now + req.Ttl
			} else {
				doc.ExpiresAt = 0
			}
		}

		buf, err := marshalDoc(&doc)
		if err != nil {
			return err
		}

		docKey := kDoc(req.Collection, doc.ID)
		if err := bDocs.Put(docKey, buf); err != nil {
			return err
		}
		bo.Put("docs", docKey, buf)

		if metaDidChange {
			if existing.Meta != nil {
				for mk, vals := range existing.Meta {
					for _, mv := range vals {
						mkey := append(kMetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
						_ = bIdx.Delete(mkey)
						bo.Delete("idxmeta", mkey)
					}
				}
			}
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Put(mkey, []byte("1")); err != nil {
						return err
					}
					bo.Put("idxmeta", mkey, []byte("1"))
				}
			}
		}

		rkey := append(kRevPrefix(req.Collection, doc.ID), []byte(fmt.Sprintf("%020d", now))...)
		if err := bRev.Put(rkey, buf); err != nil {
			return err
		}
		bo.Put("rev", rkey, buf)

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(g.server.Binlog)
	}
	if err != nil {
		if err.Error() == "not found" {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Post-update hooks
	if req.UpdateContent && g.server.EmbeddingWorker != nil && saved.ContentMD != "" {
		g.server.EmbeddingWorker.Enqueue(EmbeddingJob{
			Collection: req.Collection,
			DocID:      saved.ID,
			ContentMD:  saved.ContentMD,
		})
	}

	if g.server.TTLManager != nil {
		if saved.ExpiresAt > 0 {
			_ = g.server.TTLManager.Set(req.Collection, saved.ID, saved.ExpiresAt)
		} else if req.UpdateTtl {
			_ = g.server.TTLManager.Remove(req.Collection, saved.ID)
		}
	}

	if g.server.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		go g.server.AutomationManager.EvaluateTriggers(req.Collection, saved, "update")
	}

	return docToProto(&saved), nil
}

// GetDocumentMeta implements the GetDocumentMeta RPC - returns metadata only
func (g *GRPCServer) GetDocumentMeta(ctx context.Context, req *proto.GetDocumentMetaRequest) (*proto.GetDocumentMetaResponse, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, key, lang")
	}

	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	var doc Doc
	err := g.server.DB.View(func(tx *bolt.Tx) error {
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
		if err.Error() == "not found" {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		return nil, status.Error(codes.NotFound, "document not found")
	}

	protoMeta := make(map[string]*proto.MetaValues)
	for k, v := range doc.Meta {
		protoMeta[k] = &proto.MetaValues{Values: v}
	}

	return &proto.GetDocumentMetaResponse{
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      protoMeta,
		AddedAt:   doc.AddedAt,
		UpdatedAt: doc.UpdatedAt,
		ExpiresAt: doc.ExpiresAt,
	}, nil
}

// Classify implements the Classify RPC — zero-shot document classification.
func (g *GRPCServer) Classify(ctx context.Context, req *proto.ClassifyRequest) (*proto.ClassifyResponse, error) {
	if g.server.AuthManager != nil && req.Collection != "" {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	if len(req.Labels) == 0 {
		return nil, status.Error(codes.InvalidArgument, "labels are required")
	}

	resp, err := g.server.classifyDocument(ctx, req.Collection, req.Key, req.Lang, req.Text, req.Labels, int(req.TopK), req.Multi, req.Threshold)
	if err != nil {
		if err.Error() == "no embedding provider configured" {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if err.Error() == "not found" {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	protoResults := make([]*proto.ClassifyLabelScore, len(resp.Results))
	for i, r := range resp.Results {
		protoResults[i] = &proto.ClassifyLabelScore{
			Label: r.Label,
			Score: r.Score,
		}
	}

	return &proto.ClassifyResponse{
		Results:    protoResults,
		Model:      resp.Model,
		Dimensions: safeInt32(resp.Dimensions),
	}, nil
}

// DeleteDocument implements the DeleteDocument RPC — deletes a single document.
func (g *GRPCServer) DeleteDocument(ctx context.Context, req *proto.DeleteDocumentRequest) (*proto.DeleteDocumentResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, key, lang")
	}
	if err := g.server.deleteDocumentInternal(req.Collection, req.Key, req.Lang); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.DeleteDocumentResponse{
		Status:     "deleted",
		Collection: req.Collection,
		Key:        req.Key,
		Lang:       req.Lang,
	}, nil
}

// DeleteCollection implements the DeleteCollection RPC — deletes all documents in a collection.
func (g *GRPCServer) DeleteCollection(ctx context.Context, req *proto.DeleteCollectionRequest) (*proto.DeleteCollectionResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}

	var deletedCount int
	var bo BinlogOps

	err := g.server.DB.Update(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr

			if err := bDocs.Delete(k); err != nil {
				return err
			}
			bo.Delete("docs", k)

			bykKey := kByKey(req.Collection, doc.Key, doc.Lang)
			if err := bByK.Delete(bykKey); err != nil {
				return err
			}
			bo.Delete("bykey", bykKey)

			rc := bRev.Cursor()
			rp := kRevPrefix(req.Collection, doc.ID)
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				if err := bRev.Delete(rk); err != nil {
					return err
				}
				bo.Delete("rev", rk)
			}

			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
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
		bo.FlushTo(g.server.Binlog)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if g.server.CollectionManager != nil {
		_ = g.server.CollectionManager.Delete(req.Collection)
	}

	return &proto.DeleteCollectionResponse{
		Status:       "deleted",
		Collection:   req.Collection,
		DeletedCount: safeInt32(deletedCount),
	}, nil
}

// ListSynonyms implements the ListSynonyms RPC.
func (g *GRPCServer) ListSynonyms(ctx context.Context, req *proto.ListSynonymsRequest) (*proto.ListSynonymsResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.SynonymManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "synonym manager not initialized")
	}

	synonyms := g.server.SynonymManager.List(req.Collection)
	entries := make([]*proto.SynonymEntry, 0, len(synonyms))
	for term, syns := range synonyms {
		entries = append(entries, &proto.SynonymEntry{
			Term:     term,
			Synonyms: syns,
		})
	}

	return &proto.ListSynonymsResponse{
		Collection: req.Collection,
		Entries:    entries,
		Total:      safeInt32(len(entries)),
	}, nil
}

// AddSynonym implements the AddSynonym RPC.
func (g *GRPCServer) AddSynonym(ctx context.Context, req *proto.AddSynonymRequest) (*proto.AddSynonymResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" || req.Term == "" || len(req.Synonyms) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, term, synonyms")
	}
	if g.server.SynonymManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "synonym manager not initialized")
	}
	if err := g.server.SynonymManager.Set(req.Collection, req.Term, req.Synonyms); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.AddSynonymResponse{Status: "ok"}, nil
}

// DeleteSynonym implements the DeleteSynonym RPC.
func (g *GRPCServer) DeleteSynonym(ctx context.Context, req *proto.DeleteSynonymRequest) (*proto.DeleteSynonymResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" || req.Term == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, term")
	}
	if g.server.SynonymManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "synonym manager not initialized")
	}
	if err := g.server.SynonymManager.Delete(req.Collection, req.Term); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.DeleteSynonymResponse{Status: "ok"}, nil
}

// ListStopwords implements the ListStopwords RPC.
func (g *GRPCServer) ListStopwords(ctx context.Context, req *proto.ListStopwordsRequest) (*proto.ListStopwordsResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.StopWordManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "stopword manager not initialized")
	}

	defaults, custom := g.server.StopWordManager.List(req.Collection)
	entries := make([]*proto.StopwordEntry, 0, len(defaults)+len(custom))
	for _, w := range defaults {
		entries = append(entries, &proto.StopwordEntry{Word: w, IsDefault: true})
	}
	for _, w := range custom {
		entries = append(entries, &proto.StopwordEntry{Word: w, IsDefault: false})
	}

	return &proto.ListStopwordsResponse{
		Collection: req.Collection,
		Entries:    entries,
		Total:      safeInt32(len(entries)),
		Defaults:   safeInt32(len(defaults)),
		Custom:     safeInt32(len(custom)),
	}, nil
}

// AddStopwords implements the AddStopwords RPC.
func (g *GRPCServer) AddStopwords(ctx context.Context, req *proto.AddStopwordsRequest) (*proto.AddStopwordsResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" || len(req.Words) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, words")
	}
	if g.server.StopWordManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "stopword manager not initialized")
	}
	if err := g.server.StopWordManager.Add(req.Collection, req.Words); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.AddStopwordsResponse{Status: "ok", Added: safeInt32(len(req.Words))}, nil
}

// DeleteStopwords implements the DeleteStopwords RPC.
func (g *GRPCServer) DeleteStopwords(ctx context.Context, req *proto.DeleteStopwordsRequest) (*proto.DeleteStopwordsResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" || len(req.Words) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, words")
	}
	if g.server.StopWordManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "stopword manager not initialized")
	}
	var deleted int32
	var errs []string
	for _, w := range req.Words {
		if err := g.server.StopWordManager.Delete(req.Collection, w); err != nil {
			errs = append(errs, err.Error())
		} else {
			deleted++
		}
	}
	return &proto.DeleteStopwordsResponse{Status: "ok", Deleted: deleted, Errors: errs}, nil
}

// GetMetaKeys implements the GetMetaKeys RPC.
func (g *GRPCServer) GetMetaKeys(ctx context.Context, req *proto.GetMetaKeysRequest) (*proto.GetMetaKeysResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	meta := make(map[string][]string)

	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx == nil {
			return nil
		}
		prefix := []byte("meta|" + req.Collection + "|")
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

	protoMeta := make(map[string]*proto.MetaValues)
	for k, v := range meta {
		protoMeta[k] = &proto.MetaValues{Values: v}
	}
	return &proto.GetMetaKeysResponse{Meta: protoMeta}, nil
}

// GetChecksum implements the GetChecksum RPC.
func (g *GRPCServer) GetChecksum(ctx context.Context, req *proto.GetChecksumRequest) (*proto.GetChecksumResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	checksum, count := g.server.collectionChecksum(req.Collection)
	return &proto.GetChecksumResponse{
		Collection:    req.Collection,
		Checksum:      checksum,
		DocumentCount: safeInt32(count),
	}, nil
}

// automationRuleToProto converts internal AutomationRule to proto.
func automationRuleToProto(r *AutomationRule) *proto.AutomationRuleProto {
	p := &proto.AutomationRuleProto{
		Id:               r.ID,
		Type:             r.Type,
		Name:             r.Name,
		Enabled:          r.Enabled,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		Url:              r.URL,
		Method:           r.Method,
		Headers:          r.Headers,
		Collection:       r.Collection,
		SearchType:       r.SearchType,
		Query:            r.Query,
		Threshold:        r.Threshold,
		WebhookId:        r.WebhookID,
		Events:           r.Events,
		SentimentEnabled: r.SentimentEnabled,
		SentimentMin:     r.SentimentMin,
		SentimentMax:     r.SentimentMax,
		ConditionLogic:   r.ConditionLogic,
		Schedule:         r.Schedule,
		TriggerId:        r.TriggerID,
		LastRun:          r.LastRun,
		NextRun:          r.NextRun,
	}
	if r.SearchParams != nil {
		if b, err := json.Marshal(r.SearchParams); err == nil {
			p.SearchParamsJson = string(b)
		}
	}
	return p
}

// protoToAutomationRule converts proto to internal AutomationRule.
func protoToAutomationRule(p *proto.AutomationRuleProto) AutomationRule {
	r := AutomationRule{
		ID:               p.Id,
		Type:             p.Type,
		Name:             p.Name,
		Enabled:          p.Enabled,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
		URL:              p.Url,
		Method:           p.Method,
		Headers:          p.Headers,
		Collection:       p.Collection,
		SearchType:       p.SearchType,
		Query:            p.Query,
		Threshold:        p.Threshold,
		WebhookID:        p.WebhookId,
		Events:           p.Events,
		SentimentEnabled: p.SentimentEnabled,
		SentimentMin:     p.SentimentMin,
		SentimentMax:     p.SentimentMax,
		ConditionLogic:   p.ConditionLogic,
		Schedule:         p.Schedule,
		TriggerID:        p.TriggerId,
		LastRun:          p.LastRun,
		NextRun:          p.NextRun,
	}
	if p.SearchParamsJson != "" {
		var sp map[string]interface{}
		if err := json.Unmarshal([]byte(p.SearchParamsJson), &sp); err == nil {
			r.SearchParams = sp
		}
	}
	return r
}

// ListAutomation implements the ListAutomation RPC.
func (g *GRPCServer) ListAutomation(ctx context.Context, req *proto.ListAutomationRequest) (*proto.ListAutomationResponse, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	rules := g.server.AutomationManager.List(req.Type)
	protoRules := make([]*proto.AutomationRuleProto, len(rules))
	for i := range rules {
		protoRules[i] = automationRuleToProto(&rules[i])
	}
	return &proto.ListAutomationResponse{Rules: protoRules, Total: safeInt32(len(protoRules))}, nil
}

// CreateAutomation implements the CreateAutomation RPC.
func (g *GRPCServer) CreateAutomation(ctx context.Context, req *proto.CreateAutomationRequest) (*proto.AutomationRuleProto, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	if req.Rule == nil {
		return nil, status.Error(codes.InvalidArgument, "missing rule")
	}
	rule := protoToAutomationRule(req.Rule)
	created, err := g.server.AutomationManager.Create(rule)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if created.Type == "cron" && g.server.CronScheduler != nil {
		g.server.CronScheduler.Reload()
	}
	return automationRuleToProto(created), nil
}

// GetAutomation implements the GetAutomation RPC.
func (g *GRPCServer) GetAutomation(ctx context.Context, req *proto.GetAutomationRequest) (*proto.AutomationRuleProto, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "missing id")
	}
	rule := g.server.AutomationManager.Get(req.Id)
	if rule == nil {
		return nil, status.Error(codes.NotFound, "automation rule not found")
	}
	return automationRuleToProto(rule), nil
}

// UpdateAutomation implements the UpdateAutomation RPC.
func (g *GRPCServer) UpdateAutomation(ctx context.Context, req *proto.UpdateAutomationRequest) (*proto.AutomationRuleProto, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	if req.Id == "" || req.Rule == nil {
		return nil, status.Error(codes.InvalidArgument, "missing id or rule")
	}
	update := protoToAutomationRule(req.Rule)
	updated, err := g.server.AutomationManager.Update(req.Id, update)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if updated.Type == "cron" && g.server.CronScheduler != nil {
		g.server.CronScheduler.Reload()
	}
	return automationRuleToProto(updated), nil
}

// DeleteAutomation implements the DeleteAutomation RPC.
func (g *GRPCServer) DeleteAutomation(ctx context.Context, req *proto.DeleteAutomationRequest) (*proto.DeleteAutomationResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "missing id")
	}
	// Check if cron type for scheduler reload
	existing := g.server.AutomationManager.Get(req.Id)
	if err := g.server.AutomationManager.Delete(req.Id); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if existing != nil && existing.Type == "cron" && g.server.CronScheduler != nil {
		g.server.CronScheduler.Reload()
	}
	return &proto.DeleteAutomationResponse{Status: "deleted", Id: req.Id}, nil
}

// TestAutomation implements the TestAutomation RPC — dry run of a trigger.
func (g *GRPCServer) TestAutomation(ctx context.Context, req *proto.TestAutomationRequest) (*proto.TestAutomationResponse, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation not initialized")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "missing id")
	}
	rule := g.server.AutomationManager.Get(req.Id)
	if rule == nil {
		return nil, status.Error(codes.NotFound, "automation rule not found")
	}
	if rule.Type != "trigger" {
		return nil, status.Error(codes.InvalidArgument, "only trigger rules can be tested")
	}

	matches, err := g.server.AutomationManager.RunTrigger(rule)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Load matched documents
	var protoDocs []*proto.Document
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		for _, m := range matches {
			v := bDocs.Get(kDoc(m.Collection, m.DocID))
			if v == nil {
				continue
			}
			d, err := unmarshalDoc(v)
			if err != nil {
				continue
			}
			protoDocs = append(protoDocs, docToProto(d))
		}
		return nil
	})

	return &proto.TestAutomationResponse{
		Trigger: automationRuleToProto(rule),
		Matches: protoDocs,
		Total:   safeInt32(len(protoDocs)),
	}, nil
}

// GetAutomationLogs implements the GetAutomationLogs RPC.
func (g *GRPCServer) GetAutomationLogs(ctx context.Context, req *proto.GetAutomationLogsRequest) (*proto.GetAutomationLogsResponse, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermAdmin); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.AutomationLogStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "automation logs not initialized")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	logs, nextCursor, err := g.server.AutomationLogStore.List(limit, req.Cursor, req.RuleId, req.Status)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	total, _ := g.server.AutomationLogStore.Count(req.RuleId, req.Status)

	protoLogs := make([]*proto.AutomationLogEntryProto, len(logs))
	for i, l := range logs {
		protoLogs[i] = &proto.AutomationLogEntryProto{
			Id:         l.ID,
			Timestamp:  l.Timestamp,
			RuleId:     l.RuleID,
			RuleName:   l.RuleName,
			RuleType:   l.RuleType,
			WebhookId:  l.WebhookID,
			WebhookUrl: l.WebhookURL,
			Status:     l.Status,
			HttpStatus: safeInt32(l.HTTPStatus),
			DurationMs: l.DurationMs,
			Error:      l.Error,
			Attempt:    safeInt32(l.Attempt),
		}
	}

	return &proto.GetAutomationLogsResponse{
		Logs:       protoLogs,
		Total:      safeInt32(total),
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}

// GetCollectionConfig implements the GetCollectionConfig RPC.
func (g *GRPCServer) GetCollectionConfig(ctx context.Context, req *proto.GetCollectionConfigRequest) (*proto.GetCollectionConfigResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.CollectionManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "collection manager not initialized")
	}
	cfg, found := g.server.CollectionManager.Get(req.Collection)
	resp := &proto.GetCollectionConfigResponse{
		Collection: req.Collection,
		Configured: found,
	}
	if found && cfg != nil {
		resp.Config = &proto.CollectionConfigProto{
			Type:        cfg.Type,
			Description: cfg.Description,
			Icon:        cfg.Icon,
			Color:       cfg.Color,
			CustomMeta:  cfg.CustomMeta,
		}
	}
	return resp, nil
}

// SetCollectionConfig implements the SetCollectionConfig RPC.
func (g *GRPCServer) SetCollectionConfig(ctx context.Context, req *proto.SetCollectionConfigRequest) (*proto.SetCollectionConfigResponse, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.CollectionManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "collection manager not initialized")
	}
	cfg := &CollectionConfig{
		Type:        req.Type,
		Description: req.Description,
		Icon:        req.Icon,
		Color:       req.Color,
		CustomMeta:  req.CustomMeta,
	}
	if err := g.server.CollectionManager.Set(req.Collection, cfg); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.SetCollectionConfigResponse{Status: "ok", Collection: req.Collection}, nil
}

// ListCollectionConfigs implements the ListCollectionConfigs RPC.
func (g *GRPCServer) ListCollectionConfigs(ctx context.Context, req *proto.ListCollectionConfigsRequest) (*proto.ListCollectionConfigsResponse, error) {
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, "*", PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	if g.server.CollectionManager == nil {
		return nil, status.Error(codes.FailedPrecondition, "collection manager not initialized")
	}
	all := g.server.CollectionManager.ListAll()
	entries := make([]*proto.CollectionConfigEntry, 0, len(all))
	for coll, cfg := range all {
		entries = append(entries, &proto.CollectionConfigEntry{
			Collection: coll,
			Config: &proto.CollectionConfigProto{
				Type:        cfg.Type,
				Description: cfg.Description,
				Icon:        cfg.Icon,
				Color:       cfg.Color,
				CustomMeta:  cfg.CustomMeta,
			},
		})
	}
	return &proto.ListCollectionConfigsResponse{Configs: entries, Total: safeInt32(len(entries))}, nil
}

// CrossSearch implements the CrossSearch RPC — cross-collection vector search.
func (g *GRPCServer) CrossSearch(ctx context.Context, req *proto.CrossSearchRequest) (*proto.CrossSearchResponse, error) {
	if len(req.TargetCollections) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing target_collections")
	}
	if req.Query == "" && len(req.QueryVector) == 0 && req.SourceDocId == "" {
		return nil, status.Error(codes.InvalidArgument, "one of query, query_vector, or source_doc_id is required")
	}

	// Check read permission on all collections
	if g.server.AuthManager != nil {
		for _, coll := range req.TargetCollections {
			if err := g.server.AuthManager.CheckPermission(ctx, coll, PermRead); err != nil {
				return nil, status.Error(codes.PermissionDenied, err.Error())
			}
		}
		if req.SourceCollection != "" {
			if err := g.server.AuthManager.CheckPermission(ctx, req.SourceCollection, PermRead); err != nil {
				return nil, status.Error(codes.PermissionDenied, err.Error())
			}
		}
	}

	// Select algorithm
	algo := req.Algorithm
	if algo == "" {
		algo = "flat"
	}
	searcher, algoOk := g.server.VectorSearchers[algo]
	if !algoOk {
		return nil, status.Error(codes.InvalidArgument, "unknown algorithm: "+algo)
	}
	if !searcher.IsReady() {
		searcher = g.server.VectorSearchers["flat"]
		algo = "flat"
	}
	if !searcher.IsReady() {
		return nil, status.Error(codes.Unavailable, "vector index is loading")
	}

	// Resolve query vector
	var queryVector []float32
	if len(req.QueryVector) > 0 {
		queryVector = req.QueryVector
	} else if req.SourceDocId != "" {
		if req.SourceCollection == "" {
			return nil, status.Error(codes.InvalidArgument, "source_collection required when using source_doc_id")
		}
		rec, err := g.server.VectorStore.Get(req.SourceCollection, req.SourceDocId)
		if err != nil || rec == nil {
			return nil, status.Error(codes.NotFound, "source document has no embedding")
		}
		queryVector = rec.Vector
	} else if g.server.Embedding != nil {
		var err error
		queryVector, err = g.server.Embedding.Embed(ctx, req.Query)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to embed query: "+err.Error())
		}
	} else {
		return nil, status.Error(codes.FailedPrecondition, "no embedding provider configured")
	}

	topK := int(req.TopK)
	if topK <= 0 {
		topK = 10
	}

	metric := ResolveSimilarity(req.DistanceMetric)
	metricName := req.DistanceMetric
	if metricName == "" {
		metricName = "cosine"
	}

	// Convert proto filter_meta
	filterMeta := make(map[string][]string)
	for k, v := range req.FilterMeta {
		filterMeta[k] = v.Values
	}

	searchTopK := topK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	type taggedResult struct {
		collection string
		result     VectorResult
	}
	var allTagged []taggedResult

	for _, coll := range req.TargetCollections {
		var results []VectorResult
		if len(filterMeta) > 0 {
			allowedIDs := g.server.getDocIDsByMeta(coll, filterMeta)
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
	protoResults := make([]*proto.CrossSearchResultItem, 0, len(allTagged))
	_ = g.server.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		for rank, tr := range allTagged {
			v := bDocs.Get(kDoc(tr.collection, tr.result.DocID))
			if v == nil {
				continue
			}
			d, err := unmarshalDoc(v)
			if err != nil {
				continue
			}
			if !req.IncludeContent {
				d.ContentMD = ""
			}
			protoResults = append(protoResults, &proto.CrossSearchResultItem{
				Collection: tr.collection,
				Document:   docToProto(d),
				Score:      tr.result.Score,
				Rank:       int32(rank + 1),
			})
		}
		return nil
	})

	return &proto.CrossSearchResponse{
		Results:           protoResults,
		Total:             safeInt32(len(protoResults)),
		SourceCollection:  req.SourceCollection,
		SourceDocId:       req.SourceDocId,
		TargetCollections: req.TargetCollections,
		Algorithm:         algo,
		DistanceMetric:    metricName,
	}, nil
}

// FindDuplicates implements the FindDuplicates RPC.
func (g *GRPCServer) FindDuplicates(ctx context.Context, req *proto.FindDuplicatesRequest) (*proto.FindDuplicatesResponse, error) {
	if req.Collection == "" {
		return nil, status.Error(codes.InvalidArgument, "missing collection")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	mode := req.Mode
	if mode == "" {
		mode = "both"
	}
	if mode != "exact" && mode != "similar" && mode != "both" {
		return nil, status.Error(codes.InvalidArgument, "mode must be 'exact', 'similar', or 'both'")
	}
	threshold := req.Threshold
	if threshold <= 0 {
		threshold = 0.9
	}
	maxDocs := int(req.MaxDocs)
	if maxDocs <= 0 {
		maxDocs = 5000
	}

	internalReq := FindDuplicatesRequest{
		Collection:     req.Collection,
		Mode:           mode,
		Threshold:      threshold,
		MaxDocs:        maxDocs,
		DistanceMetric: req.DistanceMetric,
		IncludeContent: req.IncludeContent,
	}

	resp, err := g.server.findDuplicates(internalReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	convertGroup := func(groups []DuplicateGroup) []*proto.DuplicateGroupProto {
		result := make([]*proto.DuplicateGroupProto, len(groups))
		for i, g := range groups {
			docs := make([]*proto.DuplicateDocInfoProto, len(g.Documents))
			for j, d := range g.Documents {
				docs[j] = &proto.DuplicateDocInfoProto{
					DocId:       d.DocID,
					Key:         d.Key,
					ContentHash: d.ContentHash,
					ContentMd:   d.ContentMD,
					Score:       d.Score,
				}
			}
			result[i] = &proto.DuplicateGroupProto{
				GroupId:   safeInt32(g.GroupID),
				Type:      g.Type,
				Documents: docs,
				Score:     g.Score,
			}
		}
		return result
	}

	return &proto.FindDuplicatesResponse{
		Collection:      resp.Collection,
		Mode:            resp.Mode,
		Threshold:       resp.Threshold,
		DistanceMetric:  resp.DistanceMetric,
		TotalDocuments:  safeInt32(resp.TotalDocuments),
		TotalEmbedded:   safeInt32(resp.TotalEmbedded),
		ExactGroups:     convertGroup(resp.ExactGroups),
		SimilarGroups:   convertGroup(resp.SimilarGroups),
		ExactDuplicates: safeInt32(resp.ExactDuplicates),
		SimilarPairs:    safeInt32(resp.SimilarPairs),
	}, nil
}

// ListRevisions implements the ListRevisions RPC — list revision history for a document.
func (g *GRPCServer) ListRevisions(ctx context.Context, req *proto.ListRevisionsRequest) (*proto.ListRevisionsResponse, error) {
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, key, lang")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermRead); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	docID := genID(req.Collection, req.Key, req.Lang)

	var revisions []*proto.RevisionEntryProto
	err := g.server.DB.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		if bRev == nil {
			return nil
		}
		prefix := kRevPrefix(req.Collection, docID)
		c := bRev.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			keyStr := string(k)
			lastPipe := strings.LastIndexByte(keyStr, '|')
			if lastPipe < 0 || lastPipe >= len(keyStr)-1 {
				continue
			}
			ts, err := strconv.ParseInt(keyStr[lastPipe+1:], 10, 64)
			if err != nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			protoMeta := make(map[string]*proto.MetaValues)
			for mk, mv := range docPtr.Meta {
				protoMeta[mk] = &proto.MetaValues{Values: mv}
			}
			revisions = append(revisions, &proto.RevisionEntryProto{
				Timestamp: ts,
				UpdatedAt: docPtr.UpdatedAt,
				ContentMd: docPtr.ContentMD,
				Meta:      protoMeta,
			})
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Sort newest first
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Timestamp > revisions[j].Timestamp
	})

	return &proto.ListRevisionsResponse{
		Collection: req.Collection,
		Key:        req.Key,
		Lang:       req.Lang,
		Revisions:  revisions,
		Total:      safeInt32(len(revisions)),
	}, nil
}

// RestoreRevision implements the RestoreRevision RPC — restore a document from a specific revision.
func (g *GRPCServer) RestoreRevision(ctx context.Context, req *proto.RestoreRevisionRequest) (*proto.Document, error) {
	if g.isReadOnly() {
		return nil, status.Error(codes.PermissionDenied, "read-only mode")
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" || req.Timestamp == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields: collection, key, lang, timestamp")
	}
	if g.server.AuthManager != nil {
		if err := g.server.AuthManager.CheckPermission(ctx, req.Collection, PermWrite); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	docID := genID(req.Collection, req.Key, req.Lang)
	tsKey := fmt.Sprintf("%020d", req.Timestamp)
	revKey := append(kRevPrefix(req.Collection, docID), []byte(tsKey)...)

	var revDoc *Doc
	err := g.server.DB.View(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		if bRev == nil {
			return errors.New("revision not found")
		}
		v := bRev.Get(revKey)
		if v == nil {
			return fmt.Errorf("revision not found for timestamp %d", req.Timestamp)
		}
		var err error
		revDoc, err = loadDoc(v)
		return err
	})
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	doc, _, err := g.server.addDocument(req.Collection, req.Key, req.Lang, revDoc.Meta, revDoc.ContentMD, 0)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return docToProto(&doc), nil
}
