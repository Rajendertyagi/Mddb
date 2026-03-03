package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Binlog implements a binary replication log for leader-follower replication.
// The leader appends all BoltDB mutations (Put/Delete per bucket) to the binlog.
// Followers stream entries from a given LSN to replicate state.
type Binlog struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
	path   string

	lsn       atomic.Uint64
	oldestLSN uint64 // oldest LSN still in the file
	fileSize  int64

	// Retention
	maxSize int64         // max binlog file size (default 256MB)
	maxAge  time.Duration // max retention (default 24h)

	// Real-time subscribers (followers tailing the log)
	subscribers map[string]chan *BinlogEntry
	subMu       sync.RWMutex

	// Periodic flush
	flusher chan struct{}
	done    chan struct{}
}

// BinlogConfig holds configuration for the binlog
type BinlogConfig struct {
	Path    string        // file path (default: alongside DB file)
	MaxSize int64         // max segment size in bytes (default: 256MB)
	MaxAge  time.Duration // max retention time (default: 24h)
}

const (
	defaultBinlogMaxSize = 256 * 1024 * 1024 // 256MB
	defaultBinlogMaxAge  = 24 * time.Hour
	binlogBufferSize     = 256 * 1024 // 256KB write buffer
	binlogSubscriberCap  = 4096       // channel buffer for subscribers
	binlogFlushInterval  = 100 * time.Millisecond
)

// NewBinlog creates a new binlog at the given path.
// If dbPath is provided and config.Path is empty, the binlog is placed alongside the DB file.
func NewBinlog(dbPath string, cfg BinlogConfig) (*Binlog, error) {
	binlogPath := cfg.Path
	if binlogPath == "" {
		binlogPath = filepath.Join(filepath.Dir(dbPath), "mddb.binlog")
	}

	file, err := os.OpenFile(binlogPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open binlog: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to stat binlog: %w", err)
	}

	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = defaultBinlogMaxSize
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = defaultBinlogMaxAge
	}

	b := &Binlog{
		file:        file,
		writer:      bufio.NewWriterSize(file, binlogBufferSize),
		path:        binlogPath,
		fileSize:    stat.Size(),
		maxSize:     maxSize,
		maxAge:      maxAge,
		subscribers: make(map[string]chan *BinlogEntry),
		flusher:     make(chan struct{}, 1),
		done:        make(chan struct{}),
	}

	// Recover LSN from existing data
	if stat.Size() > 0 {
		if err := b.recoverLSN(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("failed to recover binlog LSN: %w", err)
		}
	} else {
		b.oldestLSN = 0
	}

	// Start periodic flusher
	go b.periodicFlusher()

	return b, nil
}

// recoverLSN scans the binlog file to find the oldest and latest LSN
func (b *Binlog) recoverLSN() error {
	if _, err := b.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReaderSize(b.file, binlogBufferSize)
	var firstLSN, lastLSN uint64
	first := true

	for {
		// Try to read the first 8 bytes (LSN) to peek
		lsnBytes := make([]byte, 8)
		_, err := io.ReadFull(reader, lsnBytes)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading binlog LSN: %w", err)
		}

		lsn := binary.BigEndian.Uint64(lsnBytes)
		if first {
			firstLSN = lsn
			first = false //nolint:ineffassign // guard boolean read in subsequent iterations
		}
		lastLSN = lsn

		// Read type(1) + timestamp(8) + bucketNameLen(2)
		header := make([]byte, 11)
		if _, err := io.ReadFull(reader, header); err != nil {
			return fmt.Errorf("error reading binlog entry header: %w", err)
		}

		bucketNameLen := int(binary.BigEndian.Uint16(header[9:11]))
		// Skip bucket name
		if _, err := reader.Discard(bucketNameLen); err != nil {
			return err
		}

		// Read keyLen(4), skip key
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return err
		}
		keyLen := int(binary.BigEndian.Uint32(lenBuf))
		if _, err := reader.Discard(keyLen); err != nil {
			return err
		}

		// Read valueLen(4), skip value
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return err
		}
		valueLen := int(binary.BigEndian.Uint32(lenBuf))
		if _, err := reader.Discard(valueLen); err != nil {
			return err
		}

		// Skip checksum(4)
		if _, err := reader.Discard(4); err != nil {
			return err
		}

		first = false
	}

	if !first || lastLSN > 0 {
		b.oldestLSN = firstLSN
		b.lsn.Store(lastLSN)
	}

	// Seek back to end for appending
	if _, err := b.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	return nil
}

// Append writes a new entry to the binlog. The LSN is assigned automatically.
// This should be called AFTER a successful BoltDB commit.
func (b *Binlog) Append(entry *BinlogEntry) error {
	b.mu.Lock()

	// Assign LSN
	newLSN := b.lsn.Add(1)
	entry.LSN = newLSN
	entry.Timestamp = time.Now().UnixNano()

	// Serialize
	data := MarshalBinlogEntry(entry)

	// Write to buffer
	n, err := b.writer.Write(data)
	if err != nil {
		b.mu.Unlock()
		return fmt.Errorf("failed to write binlog entry: %w", err)
	}
	b.fileSize += int64(n)

	if b.oldestLSN == 0 {
		b.oldestLSN = newLSN
	}

	// Trigger async flush
	select {
	case b.flusher <- struct{}{}:
	default:
	}

	b.mu.Unlock()

	// Notify subscribers (outside lock)
	b.notifySubscribers(entry)

	return nil
}

// AppendBatch writes multiple entries in a single lock acquisition.
// All entries get sequential LSNs.
func (b *Binlog) AppendBatch(entries []*BinlogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	b.mu.Lock()

	now := time.Now().UnixNano()
	for _, entry := range entries {
		newLSN := b.lsn.Add(1)
		entry.LSN = newLSN
		entry.Timestamp = now

		data := MarshalBinlogEntry(entry)
		n, err := b.writer.Write(data)
		if err != nil {
			b.mu.Unlock()
			return fmt.Errorf("failed to write binlog batch entry: %w", err)
		}
		b.fileSize += int64(n)

		if b.oldestLSN == 0 {
			b.oldestLSN = newLSN
		}
	}

	// Trigger async flush
	select {
	case b.flusher <- struct{}{}:
	default:
	}

	b.mu.Unlock()

	// Notify subscribers
	for _, entry := range entries {
		b.notifySubscribers(entry)
	}

	return nil
}

// ReadFrom reads all entries with LSN > fromLSN.
// Returns ErrBinlogLSNTooOld if the requested LSN is no longer in the binlog.
func (b *Binlog) ReadFrom(fromLSN uint64) ([]*BinlogEntry, error) {
	b.mu.Lock()
	// Flush pending writes first so they can be read
	_ = b.flush()
	b.mu.Unlock()

	f, err := os.Open(b.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open binlog for reading: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(bufio.NewReaderSize(f, binlogBufferSize))
	if err != nil {
		return nil, err
	}

	var entries []*BinlogEntry
	pos := 0
	for pos < len(data) {
		entry, n, err := UnmarshalBinlogEntry(data[pos:])
		if err != nil {
			break // possibly truncated entry at end
		}
		pos += n
		if entry.LSN > fromLSN {
			entries = append(entries, entry)
		}
	}

	if fromLSN > 0 && b.oldestLSN > 0 && fromLSN < b.oldestLSN {
		return nil, ErrBinlogLSNTooOld
	}

	return entries, nil
}

// Subscribe creates a channel that receives new binlog entries in real-time.
// Used by followers to tail the binlog.
func (b *Binlog) Subscribe(id string) <-chan *BinlogEntry {
	b.subMu.Lock()
	defer b.subMu.Unlock()

	ch := make(chan *BinlogEntry, binlogSubscriberCap)
	b.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber.
func (b *Binlog) Unsubscribe(id string) {
	b.subMu.Lock()
	defer b.subMu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}
}

// CurrentLSN returns the current (latest) LSN
func (b *Binlog) CurrentLSN() uint64 {
	return b.lsn.Load()
}

// OldestLSN returns the oldest LSN still in the binlog
func (b *Binlog) OldestLSN() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.oldestLSN
}

// Rotate truncates the binlog, keeping only entries from keepFromLSN onwards.
// If keepFromLSN is 0, truncates everything.
func (b *Binlog) Rotate(keepFromLSN uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Flush first
	if err := b.flush(); err != nil {
		return err
	}

	if keepFromLSN == 0 {
		// Full truncate
		return b.truncate()
	}

	// Read entries to keep
	f, err := os.Open(b.path)
	if err != nil {
		return err
	}

	allData, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return err
	}

	// Find entries to keep
	var keepData []byte
	pos := 0
	for pos < len(allData) {
		entry, n, err := UnmarshalBinlogEntry(allData[pos:])
		if err != nil {
			break
		}
		if entry.LSN >= keepFromLSN {
			if keepData == nil {
				keepData = allData[pos:]
				break
			}
		}
		pos += n
	}

	// Rewrite file
	if err := b.file.Close(); err != nil {
		return err
	}

	file, err := os.OpenFile(b.path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	b.file = file
	b.writer = bufio.NewWriterSize(file, binlogBufferSize)

	if len(keepData) > 0 {
		n, err := b.writer.Write(keepData)
		if err != nil {
			return err
		}
		b.fileSize = int64(n)
		b.oldestLSN = keepFromLSN
	} else {
		b.fileSize = 0
		b.oldestLSN = 0
	}

	return b.flush()
}

// Close flushes and closes the binlog
func (b *Binlog) Close() error {
	close(b.done)

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.flush(); err != nil {
		return err
	}

	// Close all subscriber channels
	b.subMu.Lock()
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
	b.subMu.Unlock()

	return b.file.Close()
}

// Stats returns binlog statistics
func (b *Binlog) Stats() BinlogStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subMu.RLock()
	subCount := len(b.subscribers)
	b.subMu.RUnlock()

	return BinlogStats{
		CurrentLSN:  b.lsn.Load(),
		OldestLSN:   b.oldestLSN,
		FileSize:    b.fileSize,
		Subscribers: subCount,
		Path:        b.path,
	}
}

// BinlogStats contains binlog statistics
type BinlogStats struct {
	CurrentLSN  uint64 `json:"current_lsn"`
	OldestLSN   uint64 `json:"oldest_lsn"`
	FileSize    int64  `json:"file_size"`
	Subscribers int    `json:"subscribers"`
	Path        string `json:"path"`
}

// flush flushes the write buffer and syncs to disk
func (b *Binlog) flush() error {
	if err := b.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush binlog: %w", err)
	}
	if err := b.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync binlog: %w", err)
	}
	return nil
}

// truncate clears the entire binlog
func (b *Binlog) truncate() error {
	if err := b.file.Close(); err != nil {
		return err
	}

	file, err := os.OpenFile(b.path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to truncate binlog: %w", err)
	}

	b.file = file
	b.writer = bufio.NewWriterSize(file, binlogBufferSize)
	b.fileSize = 0
	b.oldestLSN = 0

	return nil
}

// periodicFlusher flushes the binlog periodically
func (b *Binlog) periodicFlusher() {
	ticker := time.NewTicker(binlogFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.mu.Lock()
			_ = b.flush()
			b.mu.Unlock()
		case <-b.flusher:
			b.mu.Lock()
			_ = b.flush()
			b.mu.Unlock()
		case <-b.done:
			return
		}
	}
}

// notifySubscribers sends an entry to all active subscribers
func (b *Binlog) notifySubscribers(entry *BinlogEntry) {
	b.subMu.RLock()
	defer b.subMu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
			// Subscriber is too slow, drop entry.
			// The follower will need to re-sync from file.
		}
	}
}

// ErrBinlogLSNTooOld is returned when the requested LSN is no longer in the binlog
var ErrBinlogLSNTooOld = fmt.Errorf("requested LSN is older than the binlog's oldest entry; full snapshot required")
