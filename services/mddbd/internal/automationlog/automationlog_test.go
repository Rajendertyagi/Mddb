package automationlog

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func setupLogStoreTest(t *testing.T, ttl time.Duration) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	ls := NewStore(db, ttl)
	if err := ls.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	return ls, func() { _ = db.Close() }
}

func TestAutomationLogStore_EnsureBucket(t *testing.T) {
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	ls := NewStore(db, 24*time.Hour)

	// Bucket should not exist yet
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("automation_log"))
		if b != nil {
			t.Fatal("bucket should not exist before EnsureBucket")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the bucket
	if err := ls.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Bucket should now exist
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("automation_log"))
		if b == nil {
			t.Fatal("bucket should exist after EnsureBucket")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Calling EnsureBucket again should be idempotent
	if err := ls.EnsureBucket(); err != nil {
		t.Fatalf("second EnsureBucket call failed: %v", err)
	}
}

func TestAutomationLogStore_LogAndList(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 24*time.Hour)
	defer cleanup()

	// Write 3 entries with small delays to ensure ordering
	entries := []Entry{
		{RuleID: "rule1", RuleName: "Rule One", RuleType: "trigger", Status: "success", HTTPStatus: 200, DurationMs: 50, Attempt: 1},
		{RuleID: "rule2", RuleName: "Rule Two", RuleType: "cron", Status: "error", HTTPStatus: 500, DurationMs: 120, Error: "timeout", Attempt: 1},
		{RuleID: "rule3", RuleName: "Rule Three", RuleType: "trigger", Status: "skipped", HTTPStatus: 0, DurationMs: 0, Attempt: 1},
	}

	for i, e := range entries {
		time.Sleep(1 * time.Millisecond)
		if err := ls.Log(e); err != nil {
			t.Fatalf("failed to log entry %d: %v", i, err)
		}
	}

	// List all entries
	result, nextCursor, err := ls.List(50, "", "", "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("expected no next cursor, got %q", nextCursor)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	// Verify newest-first order: entry 3 should be first, entry 1 last
	if result[0].RuleID != "rule3" {
		t.Errorf("expected first entry to be rule3, got %s", result[0].RuleID)
	}
	if result[1].RuleID != "rule2" {
		t.Errorf("expected second entry to be rule2, got %s", result[1].RuleID)
	}
	if result[2].RuleID != "rule1" {
		t.Errorf("expected third entry to be rule1, got %s", result[2].RuleID)
	}

	// Verify fields are preserved
	if result[0].RuleName != "Rule Three" {
		t.Errorf("expected RuleName 'Rule Three', got %q", result[0].RuleName)
	}
	if result[1].Status != "error" {
		t.Errorf("expected Status 'error', got %q", result[1].Status)
	}
	if result[1].Error != "timeout" {
		t.Errorf("expected Error 'timeout', got %q", result[1].Error)
	}
	if result[1].HTTPStatus != 500 {
		t.Errorf("expected HTTPStatus 500, got %d", result[1].HTTPStatus)
	}

	// Verify IDs were auto-generated
	for i, e := range result {
		if e.ID == "" {
			t.Errorf("entry %d has empty ID", i)
		}
		if e.Timestamp == 0 {
			t.Errorf("entry %d has zero Timestamp", i)
		}
	}
}

func TestAutomationLogStore_Pagination(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 24*time.Hour)
	defer cleanup()

	// Write 10 entries with small delays
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Millisecond)
		entry := Entry{
			RuleID:   fmt.Sprintf("rule-%d", i),
			RuleName: fmt.Sprintf("Rule %d", i),
			RuleType: "trigger",
			Status:   "success",
			Attempt:  1,
		}
		if err := ls.Log(entry); err != nil {
			t.Fatalf("failed to log entry %d: %v", i, err)
		}
	}

	// Paginate with limit=3
	var allEntries []Entry
	cursor := ""
	pages := 0

	for {
		entries, nextCursor, err := ls.List(3, cursor, "", "")
		if err != nil {
			t.Fatalf("List page %d failed: %v", pages, err)
		}

		if len(entries) == 0 && pages == 0 {
			t.Fatal("first page should not be empty")
		}

		allEntries = append(allEntries, entries...)
		pages++

		if nextCursor == "" {
			break
		}
		cursor = nextCursor

		// Safety valve to prevent infinite loops
		if pages > 10 {
			t.Fatal("too many pages, possible infinite loop")
		}
	}

	if len(allEntries) != 10 {
		t.Fatalf("expected 10 total entries across all pages, got %d", len(allEntries))
	}

	// Verify newest-first order across all pages
	for i := 0; i < len(allEntries)-1; i++ {
		if allEntries[i].ID <= allEntries[i+1].ID {
			t.Errorf("entries not in newest-first order at index %d: %s <= %s", i, allEntries[i].ID, allEntries[i+1].ID)
		}
	}

	// Verify we got all rule IDs (9 down to 0, newest first)
	if allEntries[0].RuleID != "rule-9" {
		t.Errorf("expected first entry to be rule-9, got %s", allEntries[0].RuleID)
	}
	if allEntries[9].RuleID != "rule-0" {
		t.Errorf("expected last entry to be rule-0, got %s", allEntries[9].RuleID)
	}
}

func TestAutomationLogStore_FilterByRuleID(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 24*time.Hour)
	defer cleanup()

	// Write entries with different ruleIDs
	ruleIDs := []string{"alpha", "beta", "alpha", "gamma", "alpha"}
	for i, rid := range ruleIDs {
		time.Sleep(1 * time.Millisecond)
		entry := Entry{
			RuleID:   rid,
			RuleName: fmt.Sprintf("Rule %d", i),
			RuleType: "trigger",
			Status:   "success",
			Attempt:  1,
		}
		if err := ls.Log(entry); err != nil {
			t.Fatalf("failed to log entry %d: %v", i, err)
		}
	}

	// Filter by ruleID "alpha" - should get 3 entries
	entries, _, err := ls.List(50, "", "alpha", "")
	if err != nil {
		t.Fatalf("List with ruleID filter failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries for ruleID=alpha, got %d", len(entries))
	}
	for _, e := range entries {
		if e.RuleID != "alpha" {
			t.Errorf("expected ruleID=alpha, got %s", e.RuleID)
		}
	}

	// Filter by ruleID "beta" - should get 1 entry
	entries, _, err = ls.List(50, "", "beta", "")
	if err != nil {
		t.Fatalf("List with ruleID=beta filter failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for ruleID=beta, got %d", len(entries))
	}

	// Filter by ruleID "nonexistent" - should get 0 entries
	entries, _, err = ls.List(50, "", "nonexistent", "")
	if err != nil {
		t.Fatalf("List with nonexistent ruleID filter failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for ruleID=nonexistent, got %d", len(entries))
	}
}

func TestAutomationLogStore_FilterByStatus(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 24*time.Hour)
	defer cleanup()

	// Write entries with different statuses
	statuses := []string{"success", "error", "success", "skipped", "error", "success"}
	for i, s := range statuses {
		time.Sleep(1 * time.Millisecond)
		entry := Entry{
			RuleID:   fmt.Sprintf("rule-%d", i),
			RuleName: fmt.Sprintf("Rule %d", i),
			RuleType: "trigger",
			Status:   s,
			Attempt:  1,
		}
		if err := ls.Log(entry); err != nil {
			t.Fatalf("failed to log entry %d: %v", i, err)
		}
	}

	// Filter by status "success" - should get 3 entries
	entries, _, err := ls.List(50, "", "", "success")
	if err != nil {
		t.Fatalf("List with status=success filter failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries for status=success, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Status != "success" {
			t.Errorf("expected status=success, got %s", e.Status)
		}
	}

	// Filter by status "error" - should get 2 entries
	entries, _, err = ls.List(50, "", "", "error")
	if err != nil {
		t.Fatalf("List with status=error filter failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for status=error, got %d", len(entries))
	}

	// Filter by status "skipped" - should get 1 entry
	entries, _, err = ls.List(50, "", "", "skipped")
	if err != nil {
		t.Fatalf("List with status=skipped filter failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for status=skipped, got %d", len(entries))
	}
}

func TestAutomationLogStore_Count(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 24*time.Hour)
	defer cleanup()

	// Empty store should have count 0
	count, err := ls.Count("", "")
	if err != nil {
		t.Fatalf("Count on empty store failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0 on empty store, got %d", count)
	}

	// Write entries with various ruleIDs and statuses
	testData := []struct {
		ruleID string
		status string
	}{
		{"rule-a", "success"},
		{"rule-a", "error"},
		{"rule-b", "success"},
		{"rule-b", "success"},
		{"rule-c", "skipped"},
	}
	for i, td := range testData {
		time.Sleep(1 * time.Millisecond)
		entry := Entry{
			RuleID:   td.ruleID,
			RuleName: fmt.Sprintf("Rule %d", i),
			RuleType: "trigger",
			Status:   td.status,
			Attempt:  1,
		}
		if err := ls.Log(entry); err != nil {
			t.Fatalf("failed to log entry %d: %v", i, err)
		}
	}

	// Total count (no filters)
	count, err = ls.Count("", "")
	if err != nil {
		t.Fatalf("Count total failed: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected total count 5, got %d", count)
	}

	// Count by ruleID
	count, err = ls.Count("rule-a", "")
	if err != nil {
		t.Fatalf("Count by ruleID failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2 for rule-a, got %d", count)
	}

	count, err = ls.Count("rule-b", "")
	if err != nil {
		t.Fatalf("Count by ruleID failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2 for rule-b, got %d", count)
	}

	count, err = ls.Count("rule-c", "")
	if err != nil {
		t.Fatalf("Count by ruleID failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 for rule-c, got %d", count)
	}

	// Count by status
	count, err = ls.Count("", "success")
	if err != nil {
		t.Fatalf("Count by status failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3 for success, got %d", count)
	}

	count, err = ls.Count("", "error")
	if err != nil {
		t.Fatalf("Count by status failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 for error, got %d", count)
	}

	count, err = ls.Count("", "skipped")
	if err != nil {
		t.Fatalf("Count by status failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 for skipped, got %d", count)
	}

	// Count by ruleID + status combined
	count, err = ls.Count("rule-a", "success")
	if err != nil {
		t.Fatalf("Count by ruleID+status failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 for rule-a+success, got %d", count)
	}

	// Count with nonexistent filters
	count, err = ls.Count("nonexistent", "")
	if err != nil {
		t.Fatalf("Count with nonexistent ruleID failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0 for nonexistent ruleID, got %d", count)
	}
}

func TestAutomationLogStore_Cleanup(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 1*time.Millisecond)
	defer cleanup()

	// Write an entry
	entry := Entry{
		RuleID:   "rule1",
		RuleName: "Test Rule",
		RuleType: "trigger",
		Status:   "success",
		Attempt:  1,
	}
	if err := ls.Log(entry); err != nil {
		t.Fatalf("failed to log entry: %v", err)
	}

	// Verify entry exists
	count, err := ls.Count("", "")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 entry before cleanup, got %d", count)
	}

	// Wait for the entry to expire (TTL is 1ms)
	time.Sleep(5 * time.Millisecond)

	// Run cleanup
	ls.cleanup()

	// Verify entry was removed
	count, err = ls.Count("", "")
	if err != nil {
		t.Fatalf("Count after cleanup failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 entries after cleanup, got %d", count)
	}
}

func TestAutomationLogStore_CleanupPreservesFresh(t *testing.T) {
	ls, cleanup := setupLogStoreTest(t, 1*time.Hour)
	defer cleanup()

	// Write an entry
	entry := Entry{
		RuleID:   "rule1",
		RuleName: "Test Rule",
		RuleType: "trigger",
		Status:   "success",
		Attempt:  1,
	}
	if err := ls.Log(entry); err != nil {
		t.Fatalf("failed to log entry: %v", err)
	}

	// Run cleanup immediately (entry is far from expiring with 1h TTL)
	ls.cleanup()

	// Verify entry survives
	count, err := ls.Count("", "")
	if err != nil {
		t.Fatalf("Count after cleanup failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 entry to survive cleanup, got %d", count)
	}

	// Also verify via List
	entries, _, err := ls.List(50, "", "", "")
	if err != nil {
		t.Fatalf("List after cleanup failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in list after cleanup, got %d", len(entries))
	}
	if entries[0].RuleID != "rule1" {
		t.Errorf("expected ruleID=rule1, got %s", entries[0].RuleID)
	}
}

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"7d", 168 * time.Hour},
		{"1d", 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"1h30m", 90 * time.Minute},
		{"30d", 720 * time.Hour},
		{"0d", 0},
		{"500ms", 500 * time.Millisecond},
		{"2h", 2 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseDurationString(tt.input)
			if err != nil {
				t.Fatalf("ParseDurationString(%q) returned error: %v", tt.input, err)
			}
			if d != tt.expected {
				t.Errorf("ParseDurationString(%q) = %v, want %v", tt.input, d, tt.expected)
			}
		})
	}
}

func TestParseDurationString_Invalid(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"", "empty string"},
		{"abc", "non-numeric string"},
		{"d", "day suffix only, no number"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, err := ParseDurationString(tt.input)
			if err == nil {
				t.Fatalf("ParseDurationString(%q) should return error for %s", tt.input, tt.desc)
			}
		})
	}
}

func TestAutomationLogID(t *testing.T) {
	// Generate an ID and verify its format
	id := automationLogID()

	// Should contain a pipe separator
	parts := strings.SplitN(id, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("expected ID to have format 'nanos|hex', got %q", id)
	}

	// First part should be a 20-digit zero-padded number
	nanoPart := parts[0]
	if len(nanoPart) != 20 {
		t.Errorf("expected nano part to be 20 chars, got %d: %q", len(nanoPart), nanoPart)
	}
	// Should be all digits
	for _, c := range nanoPart {
		if c < '0' || c > '9' {
			t.Errorf("nano part contains non-digit character: %c in %q", c, nanoPart)
			break
		}
	}

	// Second part should be a hex string (16 chars = 8 bytes hex-encoded)
	hexPart := parts[1]
	if len(hexPart) != 16 {
		t.Errorf("expected hex part to be 16 chars, got %d: %q", len(hexPart), hexPart)
	}
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("hex part contains non-hex character: %c in %q", c, hexPart)
			break
		}
	}

	// Generate multiple IDs and verify uniqueness
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := automationLogID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}
