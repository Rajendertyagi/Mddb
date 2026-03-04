package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
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

// startGRPCServer starts the gRPC server on the specified address
func startGRPCServer(s *Server, addr string, opts ...grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

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
	if g.server.Mode == ModeRead {
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

	return docToProto(&saved), nil
}

// AddBatch implements the AddBatch RPC - adds multiple documents in a single transaction
// Uses parallel processing for preparation, then single transaction for commit
func (g *GRPCServer) AddBatch(ctx context.Context, req *proto.AddBatchRequest) (*proto.AddBatchResponse, error) {
	if g.server.Mode == ModeRead {
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
		Total:     int32(total),
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold)
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
		Total:     int32(len(protoResults)),
		Algorithm: algo,
	}
	if g.server.Embedding != nil {
		resp.Model = g.server.Embedding.Model()
		resp.Dimensions = int32(g.server.Embedding.Dimensions())
	}

	return resp, nil
}

// VectorReindex implements the VectorReindex RPC
func (g *GRPCServer) VectorReindex(ctx context.Context, req *proto.VectorReindexRequest) (*proto.VectorReindexResponse, error) {
	if g.server.Mode == ModeRead {
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
		resp.Dimensions = int32(g.server.Embedding.Dimensions())
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
			TotalDocuments:    int32(docCounts[coll]),
			EmbeddedDocuments: int32(vectorCounts[coll]),
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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

	var results []FTSResult
	var err error
	switch algo {
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
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown algorithm: "+algo+", available: tfidf, bm25")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

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
		Total:     int32(len(protoResults)),
		Algorithm: algo,
		Fuzzy:     int32(fuzzy),
	}, nil
}

// RegisterWebhook implements the RegisterWebhook RPC
func (g *GRPCServer) RegisterWebhook(ctx context.Context, req *proto.RegisterWebhookRequest) (*proto.WebhookProto, error) {
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
	if g.server.Mode == ModeRead {
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
