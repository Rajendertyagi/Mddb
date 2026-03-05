package main

import (
	"testing"
)

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]string
		expected string
	}{
		{
			name:     "simple replacement",
			template: "hello {{name}}",
			vars:     map[string]string{"name": "world"},
			expected: "hello world",
		},
		{
			name:     "multiple variables",
			template: "{{a}} and {{b}}",
			vars:     map[string]string{"a": "foo", "b": "bar"},
			expected: "foo and bar",
		},
		{
			name:     "unknown variable left as-is",
			template: "hello {{unknown}}",
			vars:     map[string]string{"name": "world"},
			expected: "hello {{unknown}}",
		},
		{
			name:     "empty vars",
			template: "hello {{name}}",
			vars:     map[string]string{},
			expected: "hello {{name}}",
		},
		{
			name:     "empty template",
			template: "",
			vars:     map[string]string{"name": "world"},
			expected: "",
		},
		{
			name:     "multiple occurrences",
			template: "{{x}}-{{x}}-{{x}}",
			vars:     map[string]string{"x": "a"},
			expected: "a-a-a",
		},
		{
			name:     "no placeholders",
			template: "plain text",
			vars:     map[string]string{"name": "world"},
			expected: "plain text",
		},
		{
			name:     "dotted variable names",
			template: "id={{doc.id}}&key={{doc.key}}",
			vars:     map[string]string{"doc.id": "123", "doc.key": "homepage"},
			expected: "id=123&key=homepage",
		},
		{
			name:     "meta field variable",
			template: "category={{doc.meta.category}}",
			vars:     map[string]string{"doc.meta.category": "tech"},
			expected: "category=tech",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandTemplate(tt.template, tt.vars)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildTriggerVars(t *testing.T) {
	webhook := &AutomationRule{ID: "wh1"}
	trigger := &AutomationRule{ID: "tr1", Name: "My Trigger"}

	t.Run("with document", func(t *testing.T) {
		doc := &Doc{
			ID:        "doc123",
			Key:       "test-doc",
			Lang:      "en",
			AddedAt:   1000,
			UpdatedAt: 2000,
			Meta: map[string][]string{
				"title":    {"Hello World"},
				"category": {"tech", "news"},
			},
		}

		vars := BuildTriggerVars(webhook, trigger, doc, "blog", 85.5, 0.65)

		expected := map[string]string{
			"doc.id":             "doc123",
			"doc.key":            "test-doc",
			"doc.lang":           "en",
			"doc.addedAt":        "1000",
			"doc.updatedAt":      "2000",
			"doc.meta.title":     "Hello World",
			"doc.meta.category":  "tech",
			"collection":         "blog",
			"event":              "trigger.matched",
			"trigger.id":         "tr1",
			"trigger.name":       "My Trigger",
			"webhook.id":         "wh1",
			"score":              "85.5000",
			"sentiment":          "0.6500",
		}

		for k, want := range expected {
			got, ok := vars[k]
			if !ok {
				t.Errorf("missing key %q", k)
				continue
			}
			if got != want {
				t.Errorf("vars[%q] = %q, want %q", k, got, want)
			}
		}

		// timestamp should be present
		if _, ok := vars["timestamp"]; !ok {
			t.Error("missing timestamp")
		}
	})

	t.Run("nil document", func(t *testing.T) {
		vars := BuildTriggerVars(webhook, trigger, nil, "articles", 0, 0)

		if _, ok := vars["doc.id"]; ok {
			t.Error("doc.id should not be present for nil doc")
		}
		if vars["collection"] != "articles" {
			t.Errorf("collection = %q, want %q", vars["collection"], "articles")
		}
		if vars["trigger.id"] != "tr1" {
			t.Errorf("trigger.id = %q, want %q", vars["trigger.id"], "tr1")
		}
	})
}

func TestBuildCronVars(t *testing.T) {
	webhook := &AutomationRule{ID: "wh2"}
	vars := BuildCronVars(webhook, "cron1", "Daily Backup")

	expected := map[string]string{
		"cron.id":    "cron1",
		"cron.name":  "Daily Backup",
		"event":      "cron.fired",
		"webhook.id": "wh2",
	}

	for k, want := range expected {
		got, ok := vars[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("vars[%q] = %q, want %q", k, got, want)
		}
	}

	if _, ok := vars["timestamp"]; !ok {
		t.Error("missing timestamp")
	}
}

func TestExpandWebhookURLAndHeaders(t *testing.T) {
	vars := map[string]string{
		"doc.id":     "abc",
		"collection": "blog",
		"doc.key":    "my-post",
	}

	url := "https://example.com/{{collection}}/{{doc.id}}"
	headers := map[string]string{
		"X-Doc-Key":    "{{doc.key}}",
		"Content-Type": "application/json",
	}

	expandedURL, expandedHeaders := expandWebhookURLAndHeaders(url, headers, vars)

	if expandedURL != "https://example.com/blog/abc" {
		t.Errorf("expandedURL = %q, want %q", expandedURL, "https://example.com/blog/abc")
	}
	if expandedHeaders["X-Doc-Key"] != "my-post" {
		t.Errorf("X-Doc-Key = %q, want %q", expandedHeaders["X-Doc-Key"], "my-post")
	}
	if expandedHeaders["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want %q", expandedHeaders["Content-Type"], "application/json")
	}
}

func TestExpandWebhookURLAndHeadersEmpty(t *testing.T) {
	url, headers := expandWebhookURLAndHeaders("https://example.com", nil, map[string]string{})
	if url != "https://example.com" {
		t.Errorf("url = %q, want %q", url, "https://example.com")
	}
	if len(headers) != 0 {
		t.Errorf("expected empty headers, got %v", headers)
	}
}
