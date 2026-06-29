package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"mddb/internal/vector"
	proto "mddb/proto"
)

// ReplicationClient manages the follower's connection to the leader.
type ReplicationClient struct {
	server        *Server
	leaderAddr    string
	followerID    string
	applier       *ReplicationApplier
	retryInterval time.Duration
	ackInterval   time.Duration
	maxLag        time.Duration

	conn   *grpc.ClientConn
	client proto.MDDBReplicationClient

	lastAppliedAt atomic.Int64
	lagMs         atomic.Int64
	running       atomic.Bool
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// ReplicationClientConfig holds configuration for the replication client
type ReplicationClientConfig struct {
	LeaderAddr    string
	FollowerID    string
	RetryInterval time.Duration
	AckInterval   time.Duration
	MaxLag        time.Duration
}

// NewReplicationClient creates a new replication client
func NewReplicationClient(s *Server, cfg ReplicationClientConfig) *ReplicationClient {
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 5 * time.Second
	}
	if cfg.AckInterval <= 0 {
		cfg.AckInterval = 10 * time.Second
	}
	if cfg.MaxLag <= 0 {
		cfg.MaxLag = 30 * time.Second
	}

	return &ReplicationClient{
		server:        s,
		leaderAddr:    cfg.LeaderAddr,
		followerID:    cfg.FollowerID,
		applier:       NewReplicationApplier(s),
		retryInterval: cfg.RetryInterval,
		ackInterval:   cfg.AckInterval,
		maxLag:        cfg.MaxLag,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the replication loop in a background goroutine
func (rc *ReplicationClient) Start() {
	if rc.running.Swap(true) {
		return // already running
	}
	rc.wg.Add(1)
	go rc.loop()
	log.Printf("Replication client started: leader=%s follower=%s", rc.leaderAddr, rc.followerID)
}

// Stop gracefully stops the replication client
func (rc *ReplicationClient) Stop() {
	if !rc.running.Swap(false) {
		return
	}
	close(rc.stopCh)
	rc.wg.Wait()
	if rc.conn != nil {
		_ = rc.conn.Close()
	}
	log.Println("Replication client stopped")
}

// LagMs returns the current replication lag in milliseconds
func (rc *ReplicationClient) LagMs() int64 {
	return rc.lagMs.Load()
}

// LastAppliedAt returns the unix timestamp of the last applied entry
func (rc *ReplicationClient) LastAppliedAt() int64 {
	return rc.lastAppliedAt.Load()
}

// IsHealthy returns true if the replication lag is within acceptable bounds
func (rc *ReplicationClient) IsHealthy() bool {
	lastApplied := rc.lastAppliedAt.Load()
	if lastApplied == 0 {
		return false // never applied anything
	}
	return time.Since(time.Unix(lastApplied, 0)) < rc.maxLag
}

// loop is the main replication loop with reconnection logic
func (rc *ReplicationClient) loop() {
	defer rc.wg.Done()

	for rc.running.Load() {
		if err := rc.connect(); err != nil {
			log.Printf("Replication: failed to connect to leader %s: %v", rc.leaderAddr, err)
			rc.waitRetry()
			continue
		}

		if err := rc.replicate(); err != nil {
			log.Printf("Replication: stream error: %v", err)
			rc.disconnect()
			rc.waitRetry()
			continue
		}
	}
}

// connect establishes a gRPC connection to the leader
func (rc *ReplicationClient) connect() error {
	if rc.conn != nil {
		_ = rc.conn.Close()
	}

	conn, err := grpc.NewClient(rc.leaderAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)), // 64MB for snapshots
	)
	if err != nil {
		return fmt.Errorf("failed to dial leader: %w", err)
	}

	rc.conn = conn
	rc.client = proto.NewMDDBReplicationClient(conn)
	log.Printf("Replication: connected to leader %s", rc.leaderAddr)
	return nil
}

// disconnect closes the gRPC connection
func (rc *ReplicationClient) disconnect() {
	if rc.conn != nil {
		_ = rc.conn.Close()
		rc.conn = nil
		rc.client = nil
	}
}

// withReplicationSecret attaches MDDB_REPLICATION_SECRET to the outgoing gRPC
// metadata so the leader's authorizeReplication accepts this follower (SEC-001).
// No-op when unset (mTLS / main-auth deployments don't need it).
func withReplicationSecret(ctx context.Context) context.Context {
	if secret := os.Getenv("MDDB_REPLICATION_SECRET"); secret != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-mddb-replication-secret", secret)
	}
	return ctx
}

// replicate starts the binlog stream and applies entries
func (rc *ReplicationClient) replicate() error {
	fromLSN := rc.applier.LastAppliedLSN()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = withReplicationSecret(ctx)

	// Start binlog stream
	stream, err := rc.client.StreamBinlog(ctx, &proto.StreamBinlogRequest{
		FollowerId: rc.followerID,
		FromLsn:    fromLSN,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.FailedPrecondition {
			// LSN too old, need full snapshot
			log.Println("Replication: LSN too old, requesting full snapshot")
			if err := rc.requestSnapshot(ctx); err != nil {
				return fmt.Errorf("snapshot failed: %w", err)
			}
			// Retry binlog stream after snapshot
			return nil
		}
		return err
	}

	// Start ack goroutine
	go rc.ackLoop(ctx)

	// Receive and apply entries
	for {
		select {
		case <-rc.stopCh:
			return nil
		default:
		}

		entry, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stream recv error: %w", err)
		}

		// Apply entry
		binlogEntry := protoToEntry(entry)
		if err := rc.applier.Apply(binlogEntry); err != nil {
			log.Printf("Replication: failed to apply entry LSN=%d: %v", entry.Lsn, err)
			continue // skip and continue (eventual consistency)
		}

		rc.lastAppliedAt.Store(time.Now().Unix())

		// Update lag estimate
		entryTime := time.Unix(0, entry.Timestamp)
		rc.lagMs.Store(time.Since(entryTime).Milliseconds())
	}
}

// requestSnapshot downloads a full database snapshot from the leader
func (rc *ReplicationClient) requestSnapshot(ctx context.Context) error {
	ctx = withReplicationSecret(ctx)
	stream, err := rc.client.RequestSnapshot(ctx, &proto.SnapshotRequest{
		FollowerId: rc.followerID,
		CurrentLsn: rc.applier.LastAppliedLSN(),
	})
	if err != nil {
		return fmt.Errorf("failed to request snapshot: %w", err)
	}

	// Write to temp file
	tmpPath := rc.server.Path + ".snapshot.tmp"
	// #nosec G304 -- Extension is hardcoded and safe
	tmpFile, err := os.Create(filepath.Clean(tmpPath))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	var snapshotLSN uint64
	var totalReceived uint64

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to receive snapshot chunk: %w", err)
		}

		if _, err := tmpFile.Write(chunk.Data); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to write snapshot chunk: %w", err)
		}

		totalReceived += uint64(len(chunk.Data))
		snapshotLSN = chunk.SnapshotLsn

		if chunk.IsLast {
			break
		}
	}

	_ = tmpFile.Close()
	log.Printf("Replication: received snapshot (%d bytes, LSN=%d)", totalReceived, snapshotLSN)

	// Replace the database and rebuild in-memory state atomically with respect
	// to live readers (GO-004). The restore write lock drains in-flight
	// DBView/DBUpdate calls and blocks new ones, so no handler observes a
	// closed or half-swapped *bolt.DB.
	if err := rc.server.withRestoreLock(func() error {
		if err := rc.replaceDatabase(tmpPath); err != nil {
			return err
		}
		rc.rebuildInMemoryState()
		return nil
	}); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace database: %w", err)
	}

	// Set the applier's LSN to the snapshot LSN (atomic, lock-free).
	rc.applier.SetLastAppliedLSN(snapshotLSN)
	rc.lastAppliedAt.Store(time.Now().Unix())

	log.Printf("Replication: snapshot applied, now at LSN=%d", snapshotLSN)
	return nil
}

// replaceDatabase swaps the current database with the snapshot
func (rc *ReplicationClient) replaceDatabase(snapshotPath string) error {
	// Close current database
	if err := rc.server.DB.Close(); err != nil {
		return fmt.Errorf("failed to close current database: %w", err)
	}

	// Rename snapshot to database path
	if err := os.Rename(snapshotPath, rc.server.Path); err != nil {
		return fmt.Errorf("failed to rename snapshot: %w", err)
	}

	// Reopen database
	db, err := bolt.Open(rc.server.Path, 0600, &bolt.Options{
		NoFreelistSync: true,
		FreelistType:   bolt.FreelistMapType,
	})
	if err != nil {
		return fmt.Errorf("failed to reopen database: %w", err)
	}

	rc.server.DB = db
	return nil
}

// rebuildInMemoryState reloads all in-memory state from the new database.
// MUST be called while holding the restore write lock (see requestSnapshot):
// it reads the freshly swapped rc.server.DB and re-points the caches/managers.
// The managers and cache are reloaded IN PLACE (same pointers) so concurrent
// readers of Server.WebhookManager / schema.SchemaManager / Cache never see a swapped
// field (GO-004). The manager reload helpers use their own (lowercase) db
// handle, not DBView/DBUpdate, so they don't re-enter the restore lock.
func (rc *ReplicationClient) rebuildInMemoryState() {
	// Reload vector index. The store wraps the new DB; the in-memory index is
	// rebuilt asynchronously (loadVectorIndex acquires the restore read lock via
	// DBView, so it waits until this restore releases the write lock).
	if rc.server.VectorIndex != nil && rc.server.VectorStore != nil {
		rc.server.VectorStore = vector.NewVectorStore(rc.server.DB)
		go rc.server.loadVectorIndex()
	}

	// Reload webhooks in place.
	if rc.server.WebhookManager != nil {
		if err := rc.server.WebhookManager.Reload(rc.server.DB); err != nil {
			log.Printf("Replication: webhook reload after snapshot failed: %v", err)
		}
	}

	// Reload schemas in place.
	if rc.server.SchemaManager != nil {
		if err := rc.server.SchemaManager.Reload(rc.server.DB); err != nil {
			log.Printf("Replication: schema reload after snapshot failed: %v", err)
		}
	}

	// Reset the document cache in place — same cache.DocumentCache (and its single
	// cleanup goroutine), contents cleared. Avoids both the Server.Cache pointer
	// race and the per-restore goroutine leak of allocating a fresh cache.
	if rc.server.Cache != nil {
		rc.server.Cache.Clear()
	}

	log.Println("Replication: in-memory state rebuilt after snapshot")
}

// ackLoop periodically sends LSN acknowledgments to the leader
func (rc *ReplicationClient) ackLoop(ctx context.Context) {
	ticker := time.NewTicker(rc.ackInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if rc.client == nil {
				continue
			}
			lsn := rc.applier.LastAppliedLSN()
			resp, err := rc.client.AcknowledgeLSN(ctx, &proto.AcknowledgeLSNRequest{
				FollowerId:   rc.followerID,
				ConfirmedLsn: lsn,
			})
			if err != nil {
				continue
			}
			// Update lag based on leader's current LSN
			if resp.LeaderLsn > lsn {
				rc.lagMs.Store(int64(resp.LeaderLsn-lsn) * 10) // #nosec G115 -- LSN diff within int64 range
			} else {
				rc.lagMs.Store(0)
			}
		case <-ctx.Done():
			return
		case <-rc.stopCh:
			return
		}
	}
}

// waitRetry waits for the retry interval or until stopped
func (rc *ReplicationClient) waitRetry() {
	select {
	case <-time.After(rc.retryInterval):
	case <-rc.stopCh:
	}
}
