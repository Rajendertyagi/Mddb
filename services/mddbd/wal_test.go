package main

import (
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newTestWAL creates a WAL in a temp directory with the given sync policy.
func newTestWAL(t *testing.T, policy SyncPolicy) *WAL {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	wal, err := NewWAL(dbPath, policy)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}
	t.Cleanup(func() {
		_ = wal.Close()
	})
	return wal
}

// -----------------------------------------------------------------------
// NewWAL tests
// -----------------------------------------------------------------------

func TestNewWAL(t *testing.T) {
	wal := newTestWAL(t, SyncNever)
	if wal == nil {
		t.Fatal("NewWAL returned nil")
		return
	}
	if wal.file == nil {
		t.Error("WAL file is nil")
	}
}

func TestNewWAL_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	wal, err := NewWAL(dbPath, SyncNever)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}
	defer func() { _ = wal.Close() }()

	walPath := filepath.Join(dir, "mddb.wal")
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Error("WAL file was not created")
	}
	if wal.path != walPath {
		t.Errorf("WAL path: got %q, want %q", wal.path, walPath)
	}
}

func TestNewWAL_InvalidPath(t *testing.T) {
	_, err := NewWAL("/nonexistent/dir/path/test.db", SyncNever)
	if err == nil {
		t.Error("NewWAL with invalid path should return error")
	}
}

func TestNewWAL_SyncPolicies(t *testing.T) {
	policies := []struct {
		name   string
		policy SyncPolicy
	}{
		{"SyncAlways", SyncAlways},
		{"SyncPeriodic", SyncPeriodic},
		{"SyncBatch", SyncBatch},
		{"SyncNever", SyncNever},
	}

	for _, tc := range policies {
		t.Run(tc.name, func(t *testing.T) {
			wal := newTestWAL(t, tc.policy)
			if wal.syncPolicy != tc.policy {
				t.Errorf("expected policy %d, got %d", tc.policy, wal.syncPolicy)
			}
		})
	}
}

// -----------------------------------------------------------------------
// Write tests
// -----------------------------------------------------------------------

func TestWAL_WriteSingleEntry(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: time.Now().Unix(),
		Data:      []byte("test document data"),
	}

	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if entry.Checksum == 0 {
		t.Error("checksum should be non-zero after write")
	}

	entries, size := wal.Stats()
	if entries != 1 {
		t.Errorf("expected 1 entry, got %d", entries)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

func TestWAL_WriteAndRead(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: time.Now().Unix(),
		Data:      []byte(`{"id":"doc1","key":"test"}`),
	}

	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Read returned %d entries, want 1", len(entries))
	}

	got := entries[0]
	if got.Type != EntryTypeAdd {
		t.Errorf("Type = %d, want %d", got.Type, EntryTypeAdd)
	}
	if got.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp = %d, want %d", got.Timestamp, entry.Timestamp)
	}
	if string(got.Data) != string(entry.Data) {
		t.Errorf("Data = %q, want %q", got.Data, entry.Data)
	}
	if got.Checksum != crc32.ChecksumIEEE(entry.Data) {
		t.Errorf("Checksum mismatch")
	}
}

func TestWAL_WriteMultipleEntries(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	types := []EntryType{EntryTypeAdd, EntryTypeUpdate, EntryTypeDelete, EntryTypeCommit}
	for i, et := range types {
		entry := &WALEntry{
			Type:      et,
			Timestamp: int64(1700000000 + i),
			Data:      []byte("data"),
		}
		if err := wal.Write(entry); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("Read returned %d entries, want 4", len(entries))
	}

	for i, e := range entries {
		if e.Type != types[i] {
			t.Errorf("entry[%d].Type = %d, want %d", i, e.Type, types[i])
		}
	}
}

func TestWAL_Write50Entries(t *testing.T) {
	wal := newTestWAL(t, SyncNever)

	for i := 0; i < 50; i++ {
		entry := &WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: time.Now().Unix(),
			Data:      []byte("entry data"),
		}
		if err := wal.Write(entry); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	entries, size := wal.Stats()
	if entries != 50 {
		t.Errorf("expected 50 entries, got %d", entries)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

func TestWAL_WriteEmptyData(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	entry := &WALEntry{
		Type:      EntryTypeDelete,
		Timestamp: time.Now().Unix(),
		Data:      []byte{},
	}

	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write with empty data: %v", err)
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if len(entries[0].Data) != 0 {
		t.Errorf("data len = %d, want 0", len(entries[0].Data))
	}
}

func TestWAL_WriteLargeEntry(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	largeData := make([]byte, 100*1024) // 100KB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: time.Now().Unix(),
		Data:      largeData,
	}

	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write large entry: %v", err)
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if len(entries[0].Data) != len(largeData) {
		t.Errorf("data len = %d, want %d", len(entries[0].Data), len(largeData))
	}
}

// -----------------------------------------------------------------------
// Read tests
// -----------------------------------------------------------------------

func TestWAL_ReadEmpty(t *testing.T) {
	wal := newTestWAL(t, SyncNever)

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read empty: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

func TestWAL_ReadIntegrity(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	// Write 100 entries with known data
	for i := 0; i < 100; i++ {
		data := make([]byte, 64)
		for j := range data {
			data[j] = byte(i + j)
		}
		entry := &WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: int64(1000 + i),
			Data:      data,
		}
		if err := wal.Write(entry); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(entries))
	}

	for i, entry := range entries {
		if entry.Timestamp != int64(1000+i) {
			t.Errorf("entry %d: timestamp mismatch: got %d, want %d", i, entry.Timestamp, 1000+i)
		}
		expectedData := make([]byte, 64)
		for j := range expectedData {
			expectedData[j] = byte(i + j)
		}
		if string(entry.Data) != string(expectedData) {
			t.Errorf("entry %d: data mismatch", i)
		}
	}
}

func TestWAL_ReadBackAllEntryTypes(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	testData := []struct {
		entryType EntryType
		data      string
	}{
		{EntryTypeAdd, "document1"},
		{EntryTypeUpdate, "document2-updated"},
		{EntryTypeDelete, "document3-deleted"},
		{EntryTypeCommit, "commit-marker"},
	}

	now := time.Now().Unix()
	for _, td := range testData {
		entry := &WALEntry{
			Type:      td.entryType,
			Timestamp: now,
			Data:      []byte(td.data),
		}
		if err := wal.Write(entry); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != len(testData) {
		t.Fatalf("expected %d entries, got %d", len(testData), len(entries))
	}

	for i, td := range testData {
		if entries[i].Type != td.entryType {
			t.Errorf("entry %d: type got %d, want %d", i, entries[i].Type, td.entryType)
		}
		if string(entries[i].Data) != td.data {
			t.Errorf("entry %d: data got %q, want %q", i, string(entries[i].Data), td.data)
		}
	}
}

// -----------------------------------------------------------------------
// Truncate tests
// -----------------------------------------------------------------------

func TestWAL_Truncate(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	for i := 0; i < 5; i++ {
		_ = wal.Write(&WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: time.Now().Unix(),
			Data:      []byte("data"),
		})
	}

	if err := wal.Truncate(); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read after Truncate: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries after Truncate = %d, want 0", len(entries))
	}

	entryCount, size := wal.Stats()
	if entryCount != 0 {
		t.Errorf("entry count after Truncate = %d, want 0", entryCount)
	}
	if size != 0 {
		t.Errorf("size after Truncate = %d, want 0", size)
	}
}

func TestWAL_TruncateAndRewrite(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	_ = wal.Write(&WALEntry{Type: EntryTypeAdd, Timestamp: 1, Data: []byte("old")})
	_ = wal.Truncate()

	_ = wal.Write(&WALEntry{Type: EntryTypeUpdate, Timestamp: 2, Data: []byte("new")})

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Type != EntryTypeUpdate {
		t.Errorf("Type = %d, want %d", entries[0].Type, EntryTypeUpdate)
	}
	if string(entries[0].Data) != "new" {
		t.Errorf("Data = %q, want %q", string(entries[0].Data), "new")
	}
}

func TestWAL_MultipleTruncates(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	for cycle := 0; cycle < 3; cycle++ {
		// Write some entries
		for i := 0; i < 5; i++ {
			_ = wal.Write(&WALEntry{
				Type:      EntryTypeAdd,
				Timestamp: int64(cycle*100 + i),
				Data:      []byte("cycle data"),
			})
		}

		// Truncate
		if err := wal.Truncate(); err != nil {
			t.Fatalf("Truncate cycle %d: %v", cycle, err)
		}

		// Verify empty
		entries, err := wal.Read()
		if err != nil {
			t.Fatalf("Read cycle %d: %v", cycle, err)
		}
		if len(entries) != 0 {
			t.Errorf("cycle %d: expected 0 entries after truncate, got %d", cycle, len(entries))
		}
	}
}

// -----------------------------------------------------------------------
// Close and recovery tests
// -----------------------------------------------------------------------

func TestWAL_Close(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	wal, err := NewWAL(dbPath, SyncNever)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}

	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: time.Now().Unix(),
		Data:      []byte("data before close"),
	}
	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify data was flushed by reopening
	wal2, err := NewWAL(dbPath, SyncNever)
	if err != nil {
		t.Fatalf("NewWAL (reopen) failed: %v", err)
	}
	defer func() { _ = wal2.Close() }()

	entries, err := wal2.Read()
	if err != nil {
		t.Fatalf("Read after reopen failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after reopen, got %d", len(entries))
	}
	if string(entries[0].Data) != "data before close" {
		t.Errorf("data mismatch: got %q", string(entries[0].Data))
	}
}

func TestWAL_Recovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Write entries with first WAL instance
	wal1, err := NewWAL(dbPath, SyncAlways)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		entry := &WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: int64(100 + i),
			Data:      []byte("recovery-test-data"),
		}
		if err := wal1.Write(entry); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	if err := wal1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen (simulates recovery after restart)
	wal2, err := NewWAL(dbPath, SyncAlways)
	if err != nil {
		t.Fatalf("NewWAL (recovery) failed: %v", err)
	}
	defer func() { _ = wal2.Close() }()

	entries, err := wal2.Read()
	if err != nil {
		t.Fatalf("Read after recovery failed: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries after recovery, got %d", len(entries))
	}

	for i, entry := range entries {
		if entry.Timestamp != int64(100+i) {
			t.Errorf("entry %d: timestamp got %d, want %d", i, entry.Timestamp, 100+i)
		}
		if string(entry.Data) != "recovery-test-data" {
			t.Errorf("entry %d: data mismatch", i)
		}
	}
}

func TestWAL_RecoveryPreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Write first batch
	wal1, err := NewWAL(dbPath, SyncAlways)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		_ = wal1.Write(&WALEntry{Type: EntryTypeAdd, Timestamp: int64(i), Data: []byte("batch1")})
	}
	if err := wal1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen and write more
	wal2, err := NewWAL(dbPath, SyncAlways)
	if err != nil {
		t.Fatalf("NewWAL (reopen) failed: %v", err)
	}
	for i := 0; i < 2; i++ {
		_ = wal2.Write(&WALEntry{Type: EntryTypeUpdate, Timestamp: int64(100 + i), Data: []byte("batch2")})
	}

	entries, err := wal2.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if err := wal2.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries (3+2), got %d", len(entries))
	}

	for i := 0; i < 3; i++ {
		if string(entries[i].Data) != "batch1" {
			t.Errorf("entry %d: expected 'batch1', got %q", i, string(entries[i].Data))
		}
		if entries[i].Type != EntryTypeAdd {
			t.Errorf("entry %d: expected EntryTypeAdd, got %d", i, entries[i].Type)
		}
	}

	for i := 3; i < 5; i++ {
		if string(entries[i].Data) != "batch2" {
			t.Errorf("entry %d: expected 'batch2', got %q", i, string(entries[i].Data))
		}
		if entries[i].Type != EntryTypeUpdate {
			t.Errorf("entry %d: expected EntryTypeUpdate, got %d", i, entries[i].Type)
		}
	}
}

// -----------------------------------------------------------------------
// Stats tests
// -----------------------------------------------------------------------

func TestWAL_StatsInitial(t *testing.T) {
	wal := newTestWAL(t, SyncNever)

	entryCount, size := wal.Stats()
	if entryCount != 0 || size != 0 {
		t.Errorf("initial stats: entries=%d size=%d, want 0,0", entryCount, size)
	}
}

func TestWAL_StatsIncrement(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	_ = wal.Write(&WALEntry{Type: EntryTypeAdd, Timestamp: 1, Data: []byte("hello")})

	entryCount, size := wal.Stats()
	if entryCount != 1 {
		t.Errorf("entries = %d, want 1", entryCount)
	}
	if size <= 0 {
		t.Errorf("size = %d, want > 0", size)
	}
}

func TestWAL_StatsProgressiveIncrement(t *testing.T) {
	wal := newTestWAL(t, SyncNever)

	var prevSize int64
	for i := 0; i < 5; i++ {
		_ = wal.Write(&WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: time.Now().Unix(),
			Data:      []byte("data"),
		})

		entries, size := wal.Stats()
		if entries != uint64(i+1) {
			t.Errorf("after write %d: expected %d entries, got %d", i, i+1, entries)
		}
		if size <= prevSize {
			t.Errorf("after write %d: size %d should be > previous size %d", i, size, prevSize)
		}
		prevSize = size
	}
}

// -----------------------------------------------------------------------
// Sync policy tests
// -----------------------------------------------------------------------

func TestWAL_SyncAlwaysPolicy(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: time.Now().Unix(),
		Data:      []byte("synced immediately"),
	}
	if err := wal.Write(entry); err != nil {
		t.Fatalf("Write with SyncAlways failed: %v", err)
	}

	// Data should be readable immediately
	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry with SyncAlways, got %d", len(entries))
	}
}

func TestWAL_SyncBatchPolicy(t *testing.T) {
	wal := newTestWAL(t, SyncBatch)

	// SyncBatch syncs every 100 entries
	for i := 0; i < 100; i++ {
		if err := wal.Write(&WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: int64(i),
			Data:      []byte("batch-data"),
		}); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 100 {
		t.Errorf("entries = %d, want 100", len(entries))
	}
}

func TestWAL_SyncPeriodicPolicy(t *testing.T) {
	wal := newTestWAL(t, SyncPeriodic)

	for i := 0; i < 10; i++ {
		if err := wal.Write(&WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: int64(i),
			Data:      []byte("periodic-data"),
		}); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}

	// Allow periodic flusher time to run
	time.Sleep(200 * time.Millisecond)

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("entries = %d, want 10", len(entries))
	}
}

// -----------------------------------------------------------------------
// Concurrent access tests
// -----------------------------------------------------------------------

func TestWAL_ConcurrentWrites(t *testing.T) {
	wal := newTestWAL(t, SyncNever)

	var wg sync.WaitGroup
	numWriters := 10
	writesPerWriter := 20

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < writesPerWriter; i++ {
				entry := &WALEntry{
					Type:      EntryTypeAdd,
					Timestamp: time.Now().Unix(),
					Data:      []byte("concurrent data"),
				}
				if err := wal.Write(entry); err != nil {
					t.Errorf("writer %d, entry %d: Write failed: %v", writerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()

	totalExpected := uint64(numWriters * writesPerWriter)
	entries, _ := wal.Stats()
	if entries != totalExpected {
		t.Errorf("expected %d entries from concurrent writes, got %d", totalExpected, entries)
	}
}

func TestWAL_ConcurrentWriteAndRead(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	// Write entries first
	for i := 0; i < 20; i++ {
		_ = wal.Write(&WALEntry{
			Type:      EntryTypeAdd,
			Timestamp: int64(i),
			Data:      []byte("pre-written"),
		})
	}

	var wg sync.WaitGroup

	// Concurrent writers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = wal.Write(&WALEntry{
				Type:      EntryTypeUpdate,
				Timestamp: int64(100 + i),
				Data:      []byte("concurrent-write"),
			})
		}
	}()

	// Concurrent readers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_, err := wal.Read()
			if err != nil {
				t.Errorf("concurrent Read failed: %v", err)
			}
		}
	}()

	wg.Wait()
}

// -----------------------------------------------------------------------
// Checksum tests
// -----------------------------------------------------------------------

func TestWAL_ChecksumVerification(t *testing.T) {
	wal := newTestWAL(t, SyncAlways)

	data := []byte("checksum test data")
	entry := &WALEntry{
		Type:      EntryTypeAdd,
		Timestamp: 1700000000,
		Data:      data,
	}

	_ = wal.Write(entry)

	entries, _ := wal.Read()
	if len(entries) != 1 {
		t.Fatal("expected 1 entry")
	}

	expectedChecksum := crc32.ChecksumIEEE(data)
	if entries[0].Checksum != expectedChecksum {
		t.Errorf("checksum = %d, want %d", entries[0].Checksum, expectedChecksum)
	}
}

// -----------------------------------------------------------------------
// Constant value tests
// -----------------------------------------------------------------------

func TestEntryTypeConstants(t *testing.T) {
	if EntryTypeAdd != 1 {
		t.Errorf("EntryTypeAdd = %d, want 1", EntryTypeAdd)
	}
	if EntryTypeUpdate != 2 {
		t.Errorf("EntryTypeUpdate = %d, want 2", EntryTypeUpdate)
	}
	if EntryTypeDelete != 3 {
		t.Errorf("EntryTypeDelete = %d, want 3", EntryTypeDelete)
	}
	if EntryTypeCommit != 4 {
		t.Errorf("EntryTypeCommit = %d, want 4", EntryTypeCommit)
	}
}

func TestSyncPolicyConstants(t *testing.T) {
	if SyncAlways != 0 {
		t.Errorf("SyncAlways = %d, want 0", SyncAlways)
	}
	if SyncPeriodic != 1 {
		t.Errorf("SyncPeriodic = %d, want 1", SyncPeriodic)
	}
	if SyncBatch != 2 {
		t.Errorf("SyncBatch = %d, want 2", SyncBatch)
	}
	if SyncNever != 3 {
		t.Errorf("SyncNever = %d, want 3", SyncNever)
	}
}

// -----------------------------------------------------------------------
// Table-driven tests
// -----------------------------------------------------------------------

func TestWAL_EntryTypesTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		entryType EntryType
		data      string
	}{
		{"add entry", EntryTypeAdd, "add data"},
		{"update entry", EntryTypeUpdate, "update data"},
		{"delete entry", EntryTypeDelete, "delete data"},
		{"commit entry", EntryTypeCommit, "commit marker"},
	}

	wal := newTestWAL(t, SyncAlways)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &WALEntry{
				Type:      tt.entryType,
				Timestamp: time.Now().Unix(),
				Data:      []byte(tt.data),
			}
			if err := wal.Write(entry); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		})
	}

	entries, err := wal.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != len(tests) {
		t.Fatalf("expected %d entries, got %d", len(tests), len(entries))
	}

	for i, tt := range tests {
		if entries[i].Type != tt.entryType {
			t.Errorf("%s: type got %d, want %d", tt.name, entries[i].Type, tt.entryType)
		}
		if string(entries[i].Data) != tt.data {
			t.Errorf("%s: data got %q, want %q", tt.name, string(entries[i].Data), tt.data)
		}
	}
}
