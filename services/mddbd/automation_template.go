package main

import (
	"fmt"
	"mddb/internal/storage"
	"strings"
	"time"
)

// ExpandTemplate replaces all {{key}} patterns in the template string with
// values from the vars map. Unknown variables are left as-is.
func ExpandTemplate(template string, vars map[string]string) string {
	for key, val := range vars {
		template = strings.ReplaceAll(template, "{{"+key+"}}", val)
	}
	return template
}

// BuildTriggerVars builds a template variable map for trigger webhook expansion.
func BuildTriggerVars(webhook *AutomationRule, trigger *AutomationRule, doc *storage.Doc, collection string, score float64, sentimentScore float64) map[string]string {
	vars := map[string]string{
		"collection":   collection,
		"event":        "trigger.matched",
		"trigger.id":   trigger.ID,
		"trigger.name": trigger.Name,
		"score":        fmt.Sprintf("%.4f", score),
		"sentiment":    fmt.Sprintf("%.4f", sentimentScore),
		"timestamp":    fmt.Sprintf("%d", time.Now().Unix()),
		"webhook.id":   webhook.ID,
	}
	if doc != nil {
		vars["doc.id"] = doc.ID
		vars["doc.key"] = doc.Key
		vars["doc.lang"] = doc.Lang
		vars["doc.addedAt"] = fmt.Sprintf("%d", doc.AddedAt)
		vars["doc.updatedAt"] = fmt.Sprintf("%d", doc.UpdatedAt)
		for field, values := range doc.Meta {
			if len(values) > 0 {
				vars["doc.meta."+field] = values[0]
			}
		}
	}
	return vars
}

// BuildCronVars builds a template variable map for cron webhook expansion.
func BuildCronVars(webhook *AutomationRule, cronID, cronName string) map[string]string {
	return map[string]string{
		"cron.id":    cronID,
		"cron.name":  cronName,
		"event":      "cron.fired",
		"timestamp":  fmt.Sprintf("%d", time.Now().Unix()),
		"webhook.id": webhook.ID,
	}
}

// expandWebhookURLAndHeaders applies template variable substitution to the
// webhook URL and all header values, returning the expanded URL and headers.
func expandWebhookURLAndHeaders(url string, headers map[string]string, vars map[string]string) (string, map[string]string) {
	expandedURL := ExpandTemplate(url, vars)
	expandedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		expandedHeaders[k] = ExpandTemplate(v, vars)
	}
	return expandedURL, expandedHeaders
}
