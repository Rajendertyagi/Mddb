package audit

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newAuditTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "audit_cov_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); _ = os.Remove(f.Name()) })
	return db
}

func TestAuditQueryAndPurge(t *testing.T) {
	db := newAuditTestDB(t)
	am := NewAuditManager(db, true, 30)
	if err := am.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	if !am.Enabled() {
		t.Error("expected Enabled() true")
	}
	_ = am.Dropped()

	am.Start()
	now := time.Now().UnixNano()
	for i := 0; i < 5; i++ {
		am.Record(AuditEvent{Timestamp: now + int64(i), Actor: "admin", Action: "write", Result: "ok"})
	}
	am.Record(AuditEvent{Timestamp: now + 10, Actor: "user", Action: "read", Result: "denied"})
	am.Stop() // drains and flushes the writer

	all, err := am.Query(QueryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(all) == 0 {
		t.Error("expected recorded events to be queryable")
	}
	// Filtered queries exercise the predicate branches.
	if _, err := am.Query(QueryFilter{Actor: "admin", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := am.Query(QueryFilter{Action: "read", Result: "denied", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := am.Query(QueryFilter{FromNanos: now, ToNanos: now + 100, Limit: 10}); err != nil {
		t.Fatal(err)
	}

	// PurgeOlderThan removes everything before the cutoff.
	if err := am.PurgeOlderThan(now + 1000); err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	_ = am.Dropped()
}

func TestAuditDisabledAndQueryLimit(t *testing.T) {
	db := newAuditTestDB(t)
	// Disabled manager + zero retention exercises the constructor defaults and
	// the Record no-op path.
	am := NewAuditManager(db, false, 0)
	if err := am.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	if am.Enabled() {
		t.Error("expected Enabled() false")
	}
	am.Start()
	am.Record(AuditEvent{Action: "ignored"}) // no-op while disabled
	am.Stop()

	// Enabled manager with many events -> Query limit-break + no-match filter.
	am2 := NewAuditManager(db, true, 30)
	if err := am2.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	am2.Start()
	now := time.Now().UnixNano()
	for i := 0; i < 20; i++ {
		am2.Record(AuditEvent{Timestamp: now + int64(i), Actor: "a", Action: "act", Result: "ok"})
	}
	am2.Stop()

	got, err := am2.Query(QueryFilter{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 5 {
		t.Errorf("Limit not respected: got %d", len(got))
	}
	if _, err := am2.Query(QueryFilter{Actor: "nobody", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	// Partial purge: keep events at/after the midpoint.
	if err := am2.PurgeOlderThan(now + 10); err != nil {
		t.Fatalf("PurgeOlderThan(partial): %v", err)
	}
}

func TestAuditNilAndQueryBranches(t *testing.T) {
	var nilAM *AuditManager
	if nilAM.Dropped() != 0 {
		t.Error("nil Dropped() != 0")
	}
	if got, _ := nilAM.Query(QueryFilter{}); got != nil {
		t.Error("nil Query() != nil")
	}

	db := newAuditTestDB(t)
	disabled := NewAuditManager(db, false, 0)
	if err := disabled.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	if got, _ := disabled.Query(QueryFilter{}); got != nil {
		t.Error("disabled Query() != nil")
	}

	am := NewAuditManager(db, true, 30)
	if err := am.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	am.Start()
	now := time.Now().UnixNano()
	for i := 0; i < 6; i++ {
		am.Record(AuditEvent{Timestamp: now + int64(i*1000), Actor: "a", Action: "x", Result: "ok"})
	}
	am.Stop()
	if _, err := am.Query(QueryFilter{}); err != nil { // Limit defaulting
		t.Fatal(err)
	}
	// From-break (oldest) + To-continue (newest) boundary branches.
	if _, err := am.Query(QueryFilter{FromNanos: now + 2000, ToNanos: now + 4000, Limit: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestExporterCoreAndDrain(t *testing.T) {
	// bufSize <= 0 defaults to 1024.
	c := newExporterCore("t", "target", 0)
	if c.bufSize != 1024 {
		t.Errorf("default bufSize = %d, want 1024", c.bufSize)
	}
	c.pushOrDrop(AuditEvent{Action: "x"})

	// Both drainAndClose branches.
	drainAndClose(nil)
	drainAndClose(io.NopCloser(strings.NewReader("body")))
}
