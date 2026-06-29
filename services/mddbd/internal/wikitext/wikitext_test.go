package wikitext

import (
	"strings"
	"testing"
)

// --- ToMarkdown ---

func TestWikitextHeadings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"h2", "== History ==", "## History"},
		{"h3", "=== Overview ===", "### Overview"},
		{"h4", "==== Details ====", "#### Details"},
		{"h5", "===== Subsection =====", "##### Subsection"},
		{"h6", "====== Deep ======", "###### Deep"},
		{"h2 with spaces", "==  Spaced  ==", "## Spaced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.input)
			if strings.TrimSpace(got) != tt.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(got), tt.want)
			}
		})
	}
}

func TestWikitextBoldItalic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bold", "'''bold text'''", "**bold text**"},
		{"italic", "''italic text''", "*italic text*"},
		{"bold+italic", "'''bold''' and ''italic''", "**bold** and *italic*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.input)
			if strings.TrimSpace(got) != tt.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(got), tt.want)
			}
		})
	}
}

func TestWikitextLinks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple link", "[[Poland]]", "Poland"},
		{"piped link", "[[Poland|Republic of Poland]]", "Republic of Poland"},
		{"external link", "[https://example.com Example]", "[Example](https://example.com)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.input)
			if strings.TrimSpace(got) != tt.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(got), tt.want)
			}
		})
	}
}

func TestWikitextCategories(t *testing.T) {
	input := "Some text\n[[Category:Science]]\n[[Category:Physics]]"
	got := ToMarkdown(input)
	if strings.Contains(got, "Category") {
		t.Errorf("categories should be removed, got %q", got)
	}
	if !strings.Contains(got, "Some text") {
		t.Errorf("content should be preserved, got %q", got)
	}
}

func TestWikitextFileLinks(t *testing.T) {
	input := "Text before [[File:Example.png|thumb|Caption]] text after"
	got := ToMarkdown(input)
	if strings.Contains(got, "File:") {
		t.Errorf("file links should be removed, got %q", got)
	}
	if !strings.Contains(got, "Text before") || !strings.Contains(got, "text after") {
		t.Errorf("surrounding text should be preserved, got %q", got)
	}
}

func TestWikitextImageLinks(t *testing.T) {
	input := "[[Image:Photo.jpg|200px|right]]"
	got := ToMarkdown(input)
	if strings.Contains(got, "Image:") {
		t.Errorf("image links should be removed, got %q", got)
	}
}

func TestWikitextTemplates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple template", "Hello {{cite web|url=test}} world", "Hello  world"},
		{"nested template", "{{Infobox {{subst:date}}}}", ""},
		{"template with text", "Start {{refn|note}} end", "Start  end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.input)
			got = strings.TrimSpace(got)
			// Normalize spaces
			for strings.Contains(got, "  ") {
				got = strings.ReplaceAll(got, "  ", " ")
			}
			expected := strings.TrimSpace(tt.want)
			for strings.Contains(expected, "  ") {
				expected = strings.ReplaceAll(expected, "  ", " ")
			}
			if got != expected {
				t.Errorf("got %q, want %q", got, expected)
			}
		})
	}
}

func TestWikitextReferences(t *testing.T) {
	input := "Fact<ref>Source book, p. 42</ref> and another<ref name=\"x\" />."
	got := ToMarkdown(input)
	if strings.Contains(got, "<ref") || strings.Contains(got, "</ref>") {
		t.Errorf("references should be stripped, got %q", got)
	}
	if !strings.Contains(got, "Fact") || !strings.Contains(got, "another") {
		t.Errorf("surrounding text should be preserved, got %q", got)
	}
}

func TestWikitextHTMLComments(t *testing.T) {
	input := "Visible <!-- hidden comment --> text"
	got := ToMarkdown(input)
	if strings.Contains(got, "hidden") {
		t.Errorf("HTML comments should be removed, got %q", got)
	}
}

func TestWikitextUnorderedLists(t *testing.T) {
	input := "* Item one\n* Item two\n** Sub-item"
	got := ToMarkdown(input)
	if !strings.Contains(got, "- Item one") {
		t.Errorf("should convert * to -, got %q", got)
	}
	if !strings.Contains(got, "  - Sub-item") {
		t.Errorf("should indent ** to 2 spaces, got %q", got)
	}
}

func TestWikitextOrderedLists(t *testing.T) {
	input := "# First\n# Second\n## Nested"
	got := ToMarkdown(input)
	if !strings.Contains(got, "1. First") {
		t.Errorf("should convert # to 1., got %q", got)
	}
	if !strings.Contains(got, "  1. Nested") {
		t.Errorf("should indent ## to 2 spaces, got %q", got)
	}
}

func TestWikitextDefinitionList(t *testing.T) {
	input := "; Term : Definition"
	got := ToMarkdown(input)
	if !strings.Contains(got, "**Term**") {
		t.Errorf("should bold the term, got %q", got)
	}
	if !strings.Contains(got, "Definition") {
		t.Errorf("should include definition, got %q", got)
	}
}

func TestWikitextIndent(t *testing.T) {
	input := ": Indented text"
	got := ToMarkdown(input)
	if !strings.Contains(got, "> Indented text") {
		t.Errorf("should convert : to blockquote, got %q", got)
	}
}

func TestWikitextHorizontalRule(t *testing.T) {
	input := "Before\n----\nAfter"
	got := ToMarkdown(input)
	if !strings.Contains(got, "---") {
		t.Errorf("should convert ---- to ---, got %q", got)
	}
}

func TestWikitextMagicWords(t *testing.T) {
	input := "__TOC__\nSome content\n__NOTOC__"
	got := ToMarkdown(input)
	if strings.Contains(got, "__TOC__") || strings.Contains(got, "__NOTOC__") {
		t.Errorf("magic words should be removed, got %q", got)
	}
	if !strings.Contains(got, "Some content") {
		t.Errorf("content should be preserved, got %q", got)
	}
}

func TestWikitextHTMLTags(t *testing.T) {
	input := "<div class=\"mw-parser-output\">Content</div>"
	got := ToMarkdown(input)
	if strings.Contains(got, "<div") || strings.Contains(got, "</div>") {
		t.Errorf("HTML tags should be stripped, got %q", got)
	}
	if !strings.Contains(got, "Content") {
		t.Errorf("content should be preserved, got %q", got)
	}
}

func TestWikitextTable(t *testing.T) {
	input := `{| class="wikitable"
|+ Caption
! Header 1 !! Header 2
|-
| Cell 1 || Cell 2
|-
| Cell 3 || Cell 4
|}`
	got := ToMarkdown(input)
	if !strings.Contains(got, "**Caption**") {
		t.Errorf("table caption should be bold, got %q", got)
	}
	if !strings.Contains(got, "Header 1") {
		t.Errorf("headers should be present, got %q", got)
	}
	if !strings.Contains(got, "Cell 1") {
		t.Errorf("cells should be present, got %q", got)
	}
}

func TestWikitextComplexArticle(t *testing.T) {
	input := `'''Poland''' ([[Polish language|Polish]]: ''Polska''), officially the '''Republic of Poland''', is a country in [[Central Europe]].

== History ==
Poland has a long [[history]].

=== Medieval period ===
The medieval period saw many changes.<ref>History Book, 2020</ref>

* First point
* Second point
** Sub-point

[[Category:Countries in Europe]]
[[Category:Poland]]
{{Infobox country
|name = Poland
|capital = Warsaw
}}`

	got := ToMarkdown(input)

	// Should have markdown headings
	if !strings.Contains(got, "## History") {
		t.Errorf("should convert == to ##, got:\n%s", got)
	}
	if !strings.Contains(got, "### Medieval period") {
		t.Errorf("should convert === to ###, got:\n%s", got)
	}

	// Should have bold
	if !strings.Contains(got, "**Poland**") {
		t.Errorf("should convert bold, got:\n%s", got)
	}

	// Should remove categories
	if strings.Contains(got, "Category") {
		t.Errorf("should remove categories, got:\n%s", got)
	}

	// Should remove templates
	if strings.Contains(got, "Infobox") {
		t.Errorf("should remove templates, got:\n%s", got)
	}

	// Should remove refs
	if strings.Contains(got, "<ref>") {
		t.Errorf("should remove refs, got:\n%s", got)
	}

	// Should have lists
	if !strings.Contains(got, "- First point") {
		t.Errorf("should convert lists, got:\n%s", got)
	}
}

func TestWikitextEmptyInput(t *testing.T) {
	got := ToMarkdown("")
	if got != "" {
		t.Errorf("empty input should return empty output, got %q", got)
	}
}

func TestWikitextMultipleBlankLines(t *testing.T) {
	input := "Line 1\n\n\n\n\nLine 2"
	got := ToMarkdown(input)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("should collapse multiple blank lines, got %q", got)
	}
}

// --- stripTemplates ---

func TestStripTemplatesNested(t *testing.T) {
	input := "{{outer {{inner}} more}}"
	got := stripTemplates(input)
	got = strings.TrimSpace(got)
	if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
		t.Errorf("nested templates should be fully stripped, got %q", got)
	}
}

func TestStripTemplatesNoTemplate(t *testing.T) {
	input := "Plain text without templates"
	got := stripTemplates(input)
	if got != input {
		t.Errorf("text without templates should be unchanged, got %q", got)
	}
}

func TestStripTemplatesUnclosed(t *testing.T) {
	input := "Start {{ unclosed template"
	got := stripTemplates(input)
	// Should not loop infinitely, just return as-is
	if !strings.Contains(got, "Start") {
		t.Errorf("unclosed template should preserve text, got %q", got)
	}
}

// --- convertLists ---

func TestConvertListsMixed(t *testing.T) {
	input := "* Bullet\n# Numbered\n: Indent\n; Term : Def"
	got := convertLists(input)
	if !strings.Contains(got, "- Bullet") {
		t.Errorf("should convert * to -, got %q", got)
	}
	if !strings.Contains(got, "1. Numbered") {
		t.Errorf("should convert # to 1., got %q", got)
	}
	if !strings.Contains(got, "> Indent") {
		t.Errorf("should convert : to >, got %q", got)
	}
	if !strings.Contains(got, "**Term**") {
		t.Errorf("should bold term, got %q", got)
	}
}

// --- convertTables ---

func TestConvertTablesBasic(t *testing.T) {
	input := `{| class="wikitable"
! Name !! Value
|-
| Alpha || 1
|-
| Beta || 2
|}`
	got := convertTables(input)
	if !strings.Contains(got, "| Name | Value |") {
		t.Errorf("should convert headers, got %q", got)
	}
	if !strings.Contains(got, "| Alpha | 1 |") {
		t.Errorf("should convert cells, got %q", got)
	}
}

func TestConvertTablesEmpty(t *testing.T) {
	input := "No tables here"
	got := convertTables(input)
	if !strings.Contains(got, "No tables here") {
		t.Errorf("text without tables should pass through, got %q", got)
	}
}

// --- splitTableCells ---

func TestSplitTableCellsBasic(t *testing.T) {
	got := splitTableCells("A || B || C", "||")
	if len(got) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(got))
	}
	if got[0] != "A" || got[1] != "B" || got[2] != "C" {
		t.Errorf("cells should be trimmed, got %v", got)
	}
}

func TestSplitTableCellsWithAttrs(t *testing.T) {
	got := splitTableCells("style=\"color:red\" | Red text || plain", "||")
	if got[0] != "Red text" {
		t.Errorf("should strip attributes, got %q", got[0])
	}
	if got[1] != "plain" {
		t.Errorf("plain cell should pass through, got %q", got[1])
	}
}
