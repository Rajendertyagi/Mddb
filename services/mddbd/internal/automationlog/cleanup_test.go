package automationlog

import (
	"testing"
	"time"
)

// TestStartCleanupRemovesExpired covers the background StartCleanup goroutine
// (which runs cleanup once immediately) and Stop's idempotency.
func TestStartCleanupRemovesExpired(t *testing.T) {
	// A 1ns TTL means anything logged is expired almost immediately.
	ls, done := setupLogStoreTest(t, time.Nanosecond)
	defer done()

	if err := ls.Log(Entry{RuleID: "r1", RuleType: "trigger", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond) // ensure now-ttl is past the entry timestamp

	ls.StartCleanup(time.Hour) // immediate cleanup pass in the goroutine

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := ls.Count("", ""); n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n, _ := ls.Count("", ""); n != 0 {
		t.Errorf("expected expired entry cleaned up, still have %d", n)
	}

	ls.Stop()
	ls.Stop() // second call must be a safe no-op (already-closed branch)
}

// TestCleanupKeepsFresh confirms cleanup leaves non-expired entries in place.
func TestCleanupKeepsFresh(t *testing.T) {
	ls, done := setupLogStoreTest(t, time.Hour)
	defer done()

	if err := ls.Log(Entry{RuleID: "r1", RuleType: "cron", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	ls.cleanup() // direct call: nothing is older than a 1h TTL

	if n, _ := ls.Count("", ""); n != 1 {
		t.Errorf("fresh entry must survive cleanup, got %d", n)
	}
}
