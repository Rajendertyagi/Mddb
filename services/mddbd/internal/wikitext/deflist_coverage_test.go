package wikitext

import (
	"strings"
	"testing"
)

// TestDefinitionListWithoutColon covers the convertLists branch for a `;term`
// definition line that has no " : " separator (bolded term only).
func TestDefinitionListWithoutColon(t *testing.T) {
	got := ToMarkdown(";Glossary")
	if !strings.Contains(got, "**Glossary**") {
		t.Errorf("ToMarkdown(;Glossary) = %q, want a bolded **Glossary**", got)
	}
}
