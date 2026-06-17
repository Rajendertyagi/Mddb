package indexqueue

import (
	"errors"
	"testing"
	"time"

	"mddb/internal/binlog"

	bolt "go.etcd.io/bbolt"
)

// failStore is a Store whose writes always fail, to drive the worker's
// failure-counting branch.
type failStore struct{}

func (failStore) DBUpdate(func(*bolt.Tx) error) error { return errors.New("db down") }
func (failStore) IdxMetaBucket() []byte               { return []byte("idxmeta") }
func (failStore) Binlog() *binlog.Binlog              { return nil }

// waitFor polls until cond() or the deadline; fails the test on timeout.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

// TestSetStoreWiresPersistence covers SetStore: the queue is built before the
// store exists (as in main), wired afterwards, then processes a job.
func TestSetStoreWiresPersistence(t *testing.T) {
	st, done := newTestStore(t)
	defer done()

	iq := NewIndexQueue(nil, 1) // store wired below, before any Enqueue
	defer iq.Shutdown()
	iq.SetStore(st)

	if err := iq.Enqueue(&IndexJob{Collection: "c", DocID: "d", NewMeta: map[string][]string{"k": {"v"}}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { p, _, _, _ := iq.Stats(); return p == 1 }, "job not processed after SetStore")
}

// TestWorkerCountsFailures covers the worker's error branch: a failing store
// makes processJob error, which must increment the failed counter.
func TestWorkerCountsFailures(t *testing.T) {
	iq := NewIndexQueue(failStore{}, 1)
	defer iq.Shutdown()

	if err := iq.Enqueue(&IndexJob{Collection: "c", DocID: "d", NewMeta: map[string][]string{"k": {"v"}}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { _, f, _, _ := iq.Stats(); return f == 1 }, "worker did not count the failure")
}
