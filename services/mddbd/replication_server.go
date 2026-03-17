package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	proto "mddb/proto"
)

const snapshotChunkSize = 1024 * 1024 // 1MB

// ReplicationServer implements the MDDBReplication gRPC service (leader-side).
type ReplicationServer struct {
	proto.UnimplementedMDDBReplicationServer
	server    *Server
	followers map[string]*FollowerState
	mu        sync.RWMutex
}

// FollowerState tracks a connected follower
type FollowerState struct {
	ID           string
	ConfirmedLSN uint64
	LastSeenAt   int64
	Address      string
}

// NewReplicationServer creates a new replication server
func NewReplicationServer(s *Server) *ReplicationServer {
	return &ReplicationServer{
		server:    s,
		followers: make(map[string]*FollowerState),
	}
}

// RequestSnapshot streams a full BoltDB snapshot to a follower
func (rs *ReplicationServer) RequestSnapshot(req *proto.SnapshotRequest, stream proto.MDDBReplication_RequestSnapshotServer) error {
	if rs.server.Binlog == nil {
		return status.Error(codes.FailedPrecondition, "binlog not enabled on this node")
	}

	followerID := req.FollowerId
	if followerID == "" {
		return status.Error(codes.InvalidArgument, "follower_id is required")
	}

	log.Printf("Replication: follower %s requested snapshot", followerID)

	// Record the LSN at snapshot time
	snapshotLSN := rs.server.Binlog.CurrentLSN()

	// Use BoltDB's read-only transaction to stream the database
	err := rs.server.DB.View(func(tx *bolt.Tx) error {
		pr, pw := io.Pipe()

		// Write snapshot to pipe in background
		go func() {
			_, err := tx.WriteTo(pw)
			pw.CloseWithError(err)
		}()

		buf := make([]byte, snapshotChunkSize)
		var offset uint64
		totalSize := uint64(tx.Size()) // #nosec G115 -- db size always non-negative

		for {
			n, err := pr.Read(buf)
			if n > 0 {
				chunk := &proto.SnapshotChunk{
					Data:        buf[:n],
					Offset:      offset,
					TotalSize:   totalSize,
					IsLast:      err == io.EOF,
					SnapshotLsn: snapshotLSN,
				}
				if sendErr := stream.Send(chunk); sendErr != nil {
					_ = pr.Close()
					return fmt.Errorf("failed to send snapshot chunk: %w", sendErr)
				}
				offset += uint64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read snapshot: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Replication: snapshot to %s failed: %v", followerID, err)
		return status.Error(codes.Internal, err.Error())
	}

	log.Printf("Replication: snapshot to %s completed (LSN=%d)", followerID, snapshotLSN)
	return nil
}

// StreamBinlog streams binlog entries from a given LSN. The stream stays open for continuous tailing.
func (rs *ReplicationServer) StreamBinlog(req *proto.StreamBinlogRequest, stream proto.MDDBReplication_StreamBinlogServer) error {
	if rs.server.Binlog == nil {
		return status.Error(codes.FailedPrecondition, "binlog not enabled on this node")
	}

	followerID := req.FollowerId
	if followerID == "" {
		return status.Error(codes.InvalidArgument, "follower_id is required")
	}

	fromLSN := req.FromLsn

	// Track follower
	addr := ""
	if p, ok := peer.FromContext(stream.Context()); ok {
		addr = p.Addr.String()
	}
	rs.mu.Lock()
	rs.followers[followerID] = &FollowerState{
		ID:           followerID,
		ConfirmedLSN: fromLSN,
		LastSeenAt:   time.Now().Unix(),
		Address:      addr,
	}
	rs.mu.Unlock()

	defer func() {
		rs.mu.Lock()
		delete(rs.followers, followerID)
		rs.mu.Unlock()
		log.Printf("Replication: follower %s disconnected from binlog stream", followerID)
	}()

	log.Printf("Replication: follower %s streaming binlog from LSN=%d", followerID, fromLSN)

	// 1. Send historical entries from binlog file
	entries, err := rs.server.Binlog.ReadFrom(fromLSN)
	if err != nil {
		if err == ErrBinlogLSNTooOld {
			return status.Error(codes.FailedPrecondition, "LSN too old, snapshot required")
		}
		return status.Error(codes.Internal, err.Error())
	}

	for _, entry := range entries {
		if err := stream.Send(entryToProto(entry)); err != nil {
			return err
		}
	}

	// 2. Subscribe to real-time entries and tail the binlog
	ch := rs.server.Binlog.Subscribe(followerID)
	defer rs.server.Binlog.Unsubscribe(followerID)

	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return nil // channel closed (binlog shutting down)
			}
			if err := stream.Send(entryToProto(entry)); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// ReplicationStatus returns the current replication state
func (rs *ReplicationServer) ReplicationStatus(_ context.Context, _ *proto.ReplicationStatusRequest) (*proto.ReplicationStatusResponse, error) {
	resp := &proto.ReplicationStatusResponse{
		NodeId: rs.server.NodeID,
		Role:   rs.server.ReplicationRole,
	}

	if rs.server.Binlog != nil {
		stats := rs.server.Binlog.Stats()
		resp.CurrentLsn = stats.CurrentLSN
		resp.BinlogOldestLsn = stats.OldestLSN
		resp.BinlogSizeBytes = stats.FileSize
	}

	// Add follower info
	rs.mu.RLock()
	for _, fs := range rs.followers {
		lagMs := int64(0)
		if rs.server.Binlog != nil {
			currentLSN := rs.server.Binlog.CurrentLSN()
			if currentLSN > fs.ConfirmedLSN {
				lagMs = int64(currentLSN-fs.ConfirmedLSN) * 10 // #nosec G115 -- rough estimate, LSN diff within int64 range
			}
		}
		resp.Followers = append(resp.Followers, &proto.FollowerInfo{
			FollowerId:   fs.ID,
			ConfirmedLsn: fs.ConfirmedLSN,
			LastSeenAt:   fs.LastSeenAt,
			Address:      fs.Address,
			LagMs:        lagMs,
		})
	}
	rs.mu.RUnlock()

	return resp, nil
}

// AcknowledgeLSN processes LSN acknowledgment from a follower
func (rs *ReplicationServer) AcknowledgeLSN(_ context.Context, req *proto.AcknowledgeLSNRequest) (*proto.AcknowledgeLSNResponse, error) {
	rs.mu.Lock()
	if fs, ok := rs.followers[req.FollowerId]; ok {
		fs.ConfirmedLSN = req.ConfirmedLsn
		fs.LastSeenAt = time.Now().Unix()
	}
	rs.mu.Unlock()

	leaderLSN := uint64(0)
	if rs.server.Binlog != nil {
		leaderLSN = rs.server.Binlog.CurrentLSN()
	}

	return &proto.AcknowledgeLSNResponse{
		Ok:        true,
		LeaderLsn: leaderLSN,
	}, nil
}

// entryToProto converts a BinlogEntry to the protobuf representation
func entryToProto(e *BinlogEntry) *proto.BinlogEntryProto {
	return &proto.BinlogEntryProto{
		Lsn:        e.LSN,
		Type:       uint32(e.Type),
		Timestamp:  e.Timestamp,
		BucketName: e.BucketName,
		Key:        e.Key,
		Value:      e.Value,
		Checksum:   e.Checksum,
	}
}

// protoToEntry converts a protobuf BinlogEntryProto to internal BinlogEntry
func protoToEntry(p *proto.BinlogEntryProto) *BinlogEntry {
	return &BinlogEntry{
		LSN:        p.Lsn,
		Type:       BinlogEntryType(p.Type), // #nosec G115 -- entry type always within byte range
		Timestamp:  p.Timestamp,
		BucketName: p.BucketName,
		Key:        p.Key,
		Value:      p.Value,
		Checksum:   p.Checksum,
	}
}
