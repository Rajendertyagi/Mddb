package main

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func setupAutomationTest(t *testing.T) (*AutomationManager, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	am := NewAutomationManager(db)
	if err := am.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	return am, func() { _ = db.Close() }
}

// createWebhook is a test helper that creates a webhook rule and returns it.
func createWebhook(t *testing.T, am *AutomationManager, name, url string) *AutomationRule {
	t.Helper()
	rule, err := am.Create(AutomationRule{
		Type:    "webhook",
		Name:    name,
		Enabled: true,
		URL:     url,
		Method:  "POST",
	})
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}
	return rule
}

// createTrigger is a test helper that creates a trigger rule and returns it.
func createTrigger(t *testing.T, am *AutomationManager, name, webhookID string) *AutomationRule {
	t.Helper()
	rule, err := am.Create(AutomationRule{
		Type:       "trigger",
		Name:       name,
		Enabled:    true,
		Collection: "blog",
		SearchType: "fts",
		Query:      "test query",
		Threshold:  50,
		WebhookID:  webhookID,
	})
	if err != nil {
		t.Fatalf("failed to create trigger: %v", err)
	}
	return rule
}

func TestAutomationCRUD(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Create a webhook
	wh := createWebhook(t, am, "My Webhook", "https://example.com/hook")
	if wh.ID == "" {
		t.Fatal("expected webhook to have an ID")
	}
	if wh.Type != "webhook" {
		t.Errorf("expected type webhook, got %s", wh.Type)
	}
	if wh.Method != "POST" {
		t.Errorf("expected default method POST, got %s", wh.Method)
	}
	if wh.CreatedAt == 0 {
		t.Error("expected createdAt to be set")
	}
	if wh.UpdatedAt == 0 {
		t.Error("expected updatedAt to be set")
	}

	// Create a trigger referencing the webhook
	tr := createTrigger(t, am, "My Trigger", wh.ID)
	if tr.ID == "" {
		t.Fatal("expected trigger to have an ID")
	}
	if tr.Type != "trigger" {
		t.Errorf("expected type trigger, got %s", tr.Type)
	}

	// Create a cron referencing the webhook
	cr, err := am.Create(AutomationRule{
		Type:      "cron",
		Name:      "My Cron",
		Enabled:   true,
		Schedule:  "0 9 * * *",
		WebhookID: wh.ID,
	})
	if err != nil {
		t.Fatalf("failed to create cron: %v", err)
	}
	if cr.ID == "" {
		t.Fatal("expected cron to have an ID")
	}
	if cr.Type != "cron" {
		t.Errorf("expected type cron, got %s", cr.Type)
	}

	// List all
	rules := am.List("")
	if len(rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(rules))
	}

	// Get by ID
	got := am.Get(wh.ID)
	if got == nil {
		t.Fatal("expected to find webhook by ID")
	}
	if got.Name != "My Webhook" {
		t.Errorf("expected name 'My Webhook', got %q", got.Name)
	}

	// Update webhook
	updated, err := am.Update(wh.ID, AutomationRule{
		Name: "Updated Webhook",
		URL:  "https://example.com/updated",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "Updated Webhook" {
		t.Errorf("expected updated name, got %q", updated.Name)
	}
	if updated.URL != "https://example.com/updated" {
		t.Errorf("expected updated URL, got %q", updated.URL)
	}

	// Delete cron first (depends on trigger)
	if err := am.Delete(cr.ID); err != nil {
		t.Fatalf("delete cron failed: %v", err)
	}

	// Delete trigger (depends on webhook)
	if err := am.Delete(tr.ID); err != nil {
		t.Fatalf("delete trigger failed: %v", err)
	}

	// Delete webhook
	if err := am.Delete(wh.ID); err != nil {
		t.Fatalf("delete webhook failed: %v", err)
	}

	// Verify all deleted
	rules = am.List("")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after deletion, got %d", len(rules))
	}
}

func TestAutomationValidation(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Missing name
	_, err := am.Create(AutomationRule{
		Type: "webhook",
		URL:  "https://example.com/hook",
	})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Invalid type
	_, err = am.Create(AutomationRule{
		Type: "invalid",
		Name: "bad type",
	})
	if err == nil {
		t.Error("expected error for invalid type")
	}

	// Empty type
	_, err = am.Create(AutomationRule{
		Name: "no type",
	})
	if err == nil {
		t.Error("expected error for empty type")
	}
}

func TestAutomationWebhookValidation(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Missing URL
	_, err := am.Create(AutomationRule{
		Type: "webhook",
		Name: "Missing URL",
	})
	if err == nil {
		t.Error("expected error for webhook without URL")
	}

	// Whitespace-only URL
	_, err = am.Create(AutomationRule{
		Type: "webhook",
		Name: "Whitespace URL",
		URL:  "   ",
	})
	if err == nil {
		t.Error("expected error for webhook with whitespace URL")
	}

	// Valid webhook
	_, err = am.Create(AutomationRule{
		Type: "webhook",
		Name: "Valid Webhook",
		URL:  "https://example.com/hook",
	})
	if err != nil {
		t.Errorf("expected no error for valid webhook, got: %v", err)
	}
}

func TestAutomationTriggerValidation(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")

	// Missing collection
	_, err := am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		SearchType: "fts",
		Query:      "test",
		WebhookID:  wh.ID,
	})
	if err == nil {
		t.Error("expected error for trigger without collection")
	}

	// Missing searchType when query is set
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		Query:      "test",
		WebhookID:  wh.ID,
	})
	if err == nil {
		t.Error("expected error for trigger with query but without searchType")
	}

	// Invalid searchType
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		SearchType: "invalid",
		Query:      "test",
		WebhookID:  wh.ID,
	})
	if err == nil {
		t.Error("expected error for trigger with invalid searchType")
	}

	// Empty query is valid (fires unconditionally)
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "No Query Trigger",
		Collection: "blog",
		WebhookID:  wh.ID,
	})
	if err != nil {
		t.Errorf("expected no error for trigger without query (unconditional), got: %v", err)
	}

	// Missing webhookId
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		SearchType: "fts",
		Query:      "test",
	})
	if err == nil {
		t.Error("expected error for trigger without webhookId")
	}

	// Non-existent webhookId
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		SearchType: "fts",
		Query:      "test",
		WebhookID:  "nonexistent",
	})
	if err == nil {
		t.Error("expected error for trigger with non-existent webhookId")
	}

	// Invalid threshold (negative)
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		SearchType: "fts",
		Query:      "test",
		Threshold:  -1,
		WebhookID:  wh.ID,
	})
	if err == nil {
		t.Error("expected error for trigger with negative threshold")
	}

	// Invalid threshold (over 100)
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Bad Trigger",
		Collection: "blog",
		SearchType: "fts",
		Query:      "test",
		Threshold:  101,
		WebhookID:  wh.ID,
	})
	if err == nil {
		t.Error("expected error for trigger with threshold > 100")
	}

	// Valid trigger with each search type
	for _, st := range []string{"fts", "vector", "hybrid"} {
		_, err = am.Create(AutomationRule{
			Type:       "trigger",
			Name:       "Trigger " + st,
			Collection: "blog",
			SearchType: st,
			Query:      "test",
			Threshold:  50,
			WebhookID:  wh.ID,
		})
		if err != nil {
			t.Errorf("expected no error for valid trigger with searchType=%s, got: %v", st, err)
		}
	}
}

func TestAutomationTriggerSentimentValidation(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")

	// 1. Valid sentiment-only trigger (no query) should succeed
	_, err := am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Sentiment Only",
		Enabled:          true,
		Collection:       "blog",
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     -0.5,
		SentimentMax:     0.5,
	})
	if err != nil {
		t.Errorf("expected no error for valid sentiment-only trigger, got: %v", err)
	}

	// 2. sentimentMin > sentimentMax should fail
	_, err = am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Bad Sentiment Range",
		Enabled:          true,
		Collection:       "blog",
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     0.5,
		SentimentMax:     -0.5,
	})
	if err == nil {
		t.Error("expected error for sentimentMin > sentimentMax")
	}

	// 3. sentimentMin < -1.0 should fail
	_, err = am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Min Too Low",
		Enabled:          true,
		Collection:       "blog",
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     -1.5,
		SentimentMax:     0.5,
	})
	if err == nil {
		t.Error("expected error for sentimentMin < -1.0")
	}

	// 4. sentimentMax > 1.0 should fail
	_, err = am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Max Too High",
		Enabled:          true,
		Collection:       "blog",
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     -0.5,
		SentimentMax:     1.5,
	})
	if err == nil {
		t.Error("expected error for sentimentMax > 1.0")
	}

	// 5. Invalid conditionLogic "xor" should fail
	_, err = am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Bad Logic",
		Enabled:          true,
		Collection:       "blog",
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     -0.5,
		SentimentMax:     0.5,
		ConditionLogic:   "xor",
	})
	if err == nil {
		t.Error("expected error for invalid conditionLogic \"xor\"")
	}

	// 6. Valid conditionLogic "or" with both search + sentiment should succeed
	_, err = am.Create(AutomationRule{
		Type:             "trigger",
		Name:             "Search And Sentiment",
		Enabled:          true,
		Collection:       "blog",
		SearchType:       "fts",
		Query:            "test query",
		Threshold:        50,
		WebhookID:        wh.ID,
		SentimentEnabled: true,
		SentimentMin:     -0.3,
		SentimentMax:     0.8,
		ConditionLogic:   "or",
	})
	if err != nil {
		t.Errorf("expected no error for valid trigger with search + sentiment + conditionLogic \"or\", got: %v", err)
	}
}

func TestAutomationCronValidation(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")

	// Missing schedule
	_, err := am.Create(AutomationRule{
		Type:      "cron",
		Name:      "Bad Cron",
		WebhookID: wh.ID,
	})
	if err == nil {
		t.Error("expected error for cron without schedule")
	}

	// Missing webhookId
	_, err = am.Create(AutomationRule{
		Type:     "cron",
		Name:     "Bad Cron",
		Schedule: "0 9 * * *",
	})
	if err == nil {
		t.Error("expected error for cron without webhookId")
	}

	// Non-existent webhookId
	_, err = am.Create(AutomationRule{
		Type:      "cron",
		Name:      "Bad Cron",
		Schedule:  "0 9 * * *",
		WebhookID: "nonexistent",
	})
	if err == nil {
		t.Error("expected error for cron with non-existent webhookId")
	}

	// Valid cron
	_, err = am.Create(AutomationRule{
		Type:      "cron",
		Name:      "Valid Cron",
		Schedule:  "0 9 * * *",
		WebhookID: wh.ID,
	})
	if err != nil {
		t.Errorf("expected no error for valid cron, got: %v", err)
	}
}

func TestAutomationList(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Create multiple rules of different types
	wh1 := createWebhook(t, am, "Webhook 1", "https://example.com/hook1")
	createWebhook(t, am, "Webhook 2", "https://example.com/hook2")
	createTrigger(t, am, "Trigger 1", wh1.ID)
	createTrigger(t, am, "Trigger 2", wh1.ID)

	_, err := am.Create(AutomationRule{
		Type:      "cron",
		Name:      "Cron 1",
		Enabled:   true,
		Schedule:  "0 9 * * *",
		WebhookID: wh1.ID,
	})
	if err != nil {
		t.Fatalf("failed to create cron: %v", err)
	}

	// List all
	all := am.List("")
	if len(all) != 5 {
		t.Errorf("expected 5 rules, got %d", len(all))
	}

	// Filter by webhook
	webhooks := am.List("webhook")
	if len(webhooks) != 2 {
		t.Errorf("expected 2 webhooks, got %d", len(webhooks))
	}
	for _, r := range webhooks {
		if r.Type != "webhook" {
			t.Errorf("expected type webhook, got %s", r.Type)
		}
	}

	// Filter by trigger
	triggers := am.List("trigger")
	if len(triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d", len(triggers))
	}
	for _, r := range triggers {
		if r.Type != "trigger" {
			t.Errorf("expected type trigger, got %s", r.Type)
		}
	}

	// Filter by cron
	crons := am.List("cron")
	if len(crons) != 1 {
		t.Errorf("expected 1 cron, got %d", len(crons))
	}
	for _, r := range crons {
		if r.Type != "cron" {
			t.Errorf("expected type cron, got %s", r.Type)
		}
	}

	// Filter by non-existent type returns empty
	empty := am.List("nonexistent")
	if len(empty) != 0 {
		t.Errorf("expected 0 rules for nonexistent type, got %d", len(empty))
	}
}

func TestAutomationUpdate(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Original Name", "https://example.com/original")

	// Update should preserve ID and type
	updated, err := am.Update(wh.ID, AutomationRule{
		Name: "New Name",
		URL:  "https://example.com/updated",
		// Try to change type (should be ignored)
		Type: "trigger",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if updated.ID != wh.ID {
		t.Errorf("expected ID to be preserved: want %s, got %s", wh.ID, updated.ID)
	}
	if updated.Type != "webhook" {
		t.Errorf("expected type to be preserved as webhook, got %s", updated.Type)
	}
	if updated.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", updated.Name)
	}
	if updated.URL != "https://example.com/updated" {
		t.Errorf("expected updated URL, got %q", updated.URL)
	}
	if updated.CreatedAt != wh.CreatedAt {
		t.Errorf("expected createdAt to be preserved: want %d, got %d", wh.CreatedAt, updated.CreatedAt)
	}
	if updated.UpdatedAt < wh.UpdatedAt {
		t.Errorf("expected updatedAt >= createdAt: want >= %d, got %d", wh.UpdatedAt, updated.UpdatedAt)
	}

	// Verify Get reflects the update
	got := am.Get(wh.ID)
	if got == nil {
		t.Fatal("expected to find updated rule")
	}
	if got.Name != "New Name" {
		t.Errorf("expected Get to return updated name, got %q", got.Name)
	}

	// Update non-existent rule
	_, err = am.Update("nonexistent", AutomationRule{
		Name: "Doesn't Matter",
		URL:  "https://example.com",
	})
	if err == nil {
		t.Error("expected error when updating non-existent rule")
	}
}

func TestAutomationDelete(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "To Delete", "https://example.com/hook")

	// Verify it exists
	if got := am.Get(wh.ID); got == nil {
		t.Fatal("expected webhook to exist before delete")
	}

	// Delete
	if err := am.Delete(wh.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// Verify it's gone from Get
	if got := am.Get(wh.ID); got != nil {
		t.Error("expected webhook to be nil after delete")
	}

	// Verify it's gone from List
	rules := am.List("")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}

	// Verify it's gone from GetWebhook
	if got := am.GetWebhook(wh.ID); got != nil {
		t.Error("expected GetWebhook to return nil after delete")
	}
}

func TestAutomationEnableDisable(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Create enabled webhook
	wh, err := am.Create(AutomationRule{
		Type:    "webhook",
		Name:    "Toggle Test",
		Enabled: true,
		URL:     "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !wh.Enabled {
		t.Error("expected webhook to be enabled after creation")
	}

	// Disable via update
	updated, err := am.Update(wh.ID, AutomationRule{
		Name:    "Toggle Test",
		Enabled: false,
		URL:     "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("update (disable) failed: %v", err)
	}
	if updated.Enabled {
		t.Error("expected webhook to be disabled after update")
	}

	// Verify via Get
	got := am.Get(wh.ID)
	if got == nil {
		t.Fatal("expected to find rule after disable")
	}
	if got.Enabled {
		t.Error("expected Get to return disabled webhook")
	}

	// Re-enable
	updated, err = am.Update(wh.ID, AutomationRule{
		Name:    "Toggle Test",
		Enabled: true,
		URL:     "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("update (enable) failed: %v", err)
	}
	if !updated.Enabled {
		t.Error("expected webhook to be re-enabled after update")
	}
}

func TestAutomationGetWebhookAndGetTrigger(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")
	tr := createTrigger(t, am, "Trigger", wh.ID)

	// GetWebhook returns webhook
	got := am.GetWebhook(wh.ID)
	if got == nil {
		t.Fatal("expected GetWebhook to find webhook")
	}
	if got.Type != "webhook" {
		t.Errorf("expected type webhook, got %s", got.Type)
	}

	// GetWebhook does not return trigger
	got = am.GetWebhook(tr.ID)
	if got != nil {
		t.Error("expected GetWebhook to return nil for trigger ID")
	}

	// GetTrigger returns trigger
	gotTr := am.GetTrigger(tr.ID)
	if gotTr == nil {
		t.Fatal("expected GetTrigger to find trigger")
	}
	if gotTr.Type != "trigger" {
		t.Errorf("expected type trigger, got %s", gotTr.Type)
	}

	// GetTrigger does not return webhook
	gotTr = am.GetTrigger(wh.ID)
	if gotTr != nil {
		t.Error("expected GetTrigger to return nil for webhook ID")
	}

	// Non-existent ID
	if am.GetWebhook("nonexistent") != nil {
		t.Error("expected GetWebhook to return nil for nonexistent ID")
	}
	if am.GetTrigger("nonexistent") != nil {
		t.Error("expected GetTrigger to return nil for nonexistent ID")
	}
}

func TestAutomationEnabledTriggersForEvent(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")

	// Create two enabled triggers for "blog"
	_, err := am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Blog Trigger 1",
		Enabled:    true,
		Collection: "blog",
		SearchType: "fts",
		Query:      "golang",
		Threshold:  50,
		WebhookID:  wh.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Blog Trigger 2",
		Enabled:    true,
		Collection: "blog",
		SearchType: "vector",
		Query:      "machine learning",
		Threshold:  70,
		WebhookID:  wh.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a disabled trigger for "blog"
	disabledTrigger, err := am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Disabled Trigger",
		Enabled:    false,
		Collection: "blog",
		SearchType: "fts",
		Query:      "disabled",
		Threshold:  50,
		WebhookID:  wh.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = disabledTrigger

	// Create a trigger for a different collection
	_, err = am.Create(AutomationRule{
		Type:       "trigger",
		Name:       "Docs Trigger",
		Enabled:    true,
		Collection: "docs",
		SearchType: "fts",
		Query:      "api",
		Threshold:  30,
		WebhookID:  wh.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only enabled triggers for "blog" on insert should be returned (default events: insert+update)
	blogTriggers := am.EnabledTriggersForEvent("blog", "insert")
	if len(blogTriggers) != 2 {
		t.Errorf("expected 2 enabled blog triggers, got %d", len(blogTriggers))
	}

	// Only enabled triggers for "docs"
	docsTriggers := am.EnabledTriggersForEvent("docs", "insert")
	if len(docsTriggers) != 1 {
		t.Errorf("expected 1 enabled docs trigger, got %d", len(docsTriggers))
	}

	// No triggers for non-existent collection
	empty := am.EnabledTriggersForEvent("nonexistent", "insert")
	if len(empty) != 0 {
		t.Errorf("expected 0 triggers for nonexistent collection, got %d", len(empty))
	}

	// No triggers for "delete" event (default events are insert+update only)
	deleteTriggers := am.EnabledTriggersForEvent("blog", "delete")
	if len(deleteTriggers) != 0 {
		t.Errorf("expected 0 triggers for delete event, got %d", len(deleteTriggers))
	}
}

func TestAutomationLoadAll(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	am := NewAutomationManager(db)
	if err := am.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	// Create some rules
	wh := createWebhook(t, am, "Hook", "https://example.com/hook")
	createTrigger(t, am, "Trigger", wh.ID)

	// Create a new manager with the same DB (simulates restart)
	am2 := NewAutomationManager(db)
	if err := am2.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	rules := am2.List("")
	if len(rules) != 2 {
		t.Errorf("expected 2 rules after LoadAll, got %d", len(rules))
	}

	// GetWebhook should work after reload
	got := am2.GetWebhook(wh.ID)
	if got == nil {
		t.Error("expected GetWebhook to work after LoadAll")
	}

	_ = db.Close()
}

func TestAutomationDefaultMethod(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	// Webhook without explicit method should default to POST
	wh, err := am.Create(AutomationRule{
		Type: "webhook",
		Name: "No Method",
		URL:  "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if wh.Method != "POST" {
		t.Errorf("expected default method POST, got %q", wh.Method)
	}

	// Webhook with explicit method should keep it
	wh2, err := am.Create(AutomationRule{
		Type:   "webhook",
		Name:   "GET Hook",
		URL:    "https://example.com/hook",
		Method: "GET",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if wh2.Method != "GET" {
		t.Errorf("expected method GET, got %q", wh2.Method)
	}
}

func TestAutomationUpdateLastRun(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	wh := createWebhook(t, am, "Hook", "https://example.com/hook")

	cr, err := am.Create(AutomationRule{
		Type:      "cron",
		Name:      "Cron",
		Enabled:   true,
		Schedule:  "0 9 * * *",
		WebhookID: wh.ID,
	})
	if err != nil {
		t.Fatalf("create cron failed: %v", err)
	}

	// Update lastRun
	var ts int64 = 1700000000
	am.UpdateLastRun(cr.ID, ts)

	// Give the async goroutine a moment to persist
	// (UpdateLastRun fires a goroutine for DB persistence)

	got := am.Get(cr.ID)
	if got == nil {
		t.Fatal("expected to find cron after UpdateLastRun")
	}
	if got.LastRun != ts {
		t.Errorf("expected lastRun=%d, got %d", ts, got.LastRun)
	}
	if got.UpdatedAt != ts {
		t.Errorf("expected updatedAt=%d, got %d", ts, got.UpdatedAt)
	}
}

func TestAutomationUniqueIDs(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	ids := make(map[string]bool)
	for i := 0; i < 20; i++ {
		wh, err := am.Create(AutomationRule{
			Type: "webhook",
			Name: "Hook",
			URL:  "https://example.com/hook",
		})
		if err != nil {
			t.Fatalf("create #%d failed: %v", i, err)
		}
		if ids[wh.ID] {
			t.Fatalf("duplicate ID generated: %s", wh.ID)
		}
		ids[wh.ID] = true
	}
}
