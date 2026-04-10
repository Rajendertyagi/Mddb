package main

import (
	"regexp"
	"strings"
)

// wikitextToMarkdown converts MediaWiki wikitext markup to Markdown.
// Handles the most common constructs: headings, bold/italic, links,
// lists, templates, references, categories, and tables.
func wikitextToMarkdown(wikitext string) string {
	s := wikitext

	// Remove categories: [[Category:...]]
	s = reCategoryLink.ReplaceAllString(s, "")

	// Remove file/image links: [[File:...]] [[Image:...]]
	s = reFileLink.ReplaceAllString(s, "")

	// Strip templates: {{...}} (non-greedy, handles single-level nesting)
	s = stripTemplates(s)

	// Strip HTML comments
	s = reHTMLComment.ReplaceAllString(s, "")

	// Convert references to inline notes
	s = reRef.ReplaceAllString(s, "")
	s = reRefSelfClose.ReplaceAllString(s, "")

	// Strip remaining HTML tags but keep content
	s = reHTMLTagWiki.ReplaceAllString(s, "")

	// Lists first (before headings, because wiki # lists conflict with markdown # headings)
	s = convertLists(s)

	// Headings: ====h4==== → #### h4
	s = reH6.ReplaceAllStringFunc(s, func(m string) string {
		return "###### " + strings.TrimSpace(reH6.FindStringSubmatch(m)[1])
	})
	s = reH5.ReplaceAllStringFunc(s, func(m string) string {
		return "##### " + strings.TrimSpace(reH5.FindStringSubmatch(m)[1])
	})
	s = reH4.ReplaceAllStringFunc(s, func(m string) string {
		return "#### " + strings.TrimSpace(reH4.FindStringSubmatch(m)[1])
	})
	s = reH3.ReplaceAllStringFunc(s, func(m string) string {
		return "### " + strings.TrimSpace(reH3.FindStringSubmatch(m)[1])
	})
	s = reH2.ReplaceAllStringFunc(s, func(m string) string {
		return "## " + strings.TrimSpace(reH2.FindStringSubmatch(m)[1])
	})

	// Bold and italic: '''bold''' → **bold**, ''italic'' → *italic*
	s = reBold.ReplaceAllString(s, "**$1**")
	s = reItalic.ReplaceAllString(s, "*$1*")

	// Internal links: [[Page|display]] → [display](Page), [[Page]] → Page
	s = reWikiLinkPiped.ReplaceAllString(s, "$2")
	s = reWikiLinkSimple.ReplaceAllString(s, "$1")

	// External links: [http://... text] → [text](http://...)
	s = reExtLink.ReplaceAllString(s, "[$2]($1)")

	// Horizontal rules: ---- → ---
	s = reHRule.ReplaceAllString(s, "---")

	// Convert wiki tables to simple text
	s = convertTables(s)

	// Magic words and parser functions
	s = reMagicWord.ReplaceAllString(s, "")

	// Clean up excessive blank lines
	s = reMultiBlank.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)

	return s
}

// Pre-compiled regular expressions for wikitext conversion.
var (
	// Headings (match from h6 down to h2 to avoid false greedy matches)
	reH6 = regexp.MustCompile(`(?m)^======\s*(.+?)\s*======\s*$`)
	reH5 = regexp.MustCompile(`(?m)^=====\s*(.+?)\s*=====\s*$`)
	reH4 = regexp.MustCompile(`(?m)^====\s*(.+?)\s*====\s*$`)
	reH3 = regexp.MustCompile(`(?m)^===\s*(.+?)\s*===\s*$`)
	reH2 = regexp.MustCompile(`(?m)^==\s*(.+?)\s*==\s*$`)

	// Bold / italic
	reBold   = regexp.MustCompile(`'''(.+?)'''`)
	reItalic = regexp.MustCompile(`''(.+?)''`)

	// Links
	reWikiLinkPiped  = regexp.MustCompile(`\[\[([^|\]]+)\|([^\]]+)\]\]`)
	reWikiLinkSimple = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	reExtLink        = regexp.MustCompile(`\[(https?://[^\s\]]+)\s+([^\]]+)\]`)

	// Categories and files
	reCategoryLink = regexp.MustCompile(`(?i)\[\[Category:[^\]]*\]\]`)
	reFileLink     = regexp.MustCompile(`(?i)\[\[(?:File|Image):[^\]]*\]\]`)

	// HTML
	reHTMLComment  = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reRef          = regexp.MustCompile(`(?s)<ref[^>]*>.*?</ref>`)
	reRefSelfClose = regexp.MustCompile(`<ref[^/]*/\s*>`)
	reHTMLTagWiki  = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

	// Horizontal rule
	reHRule = regexp.MustCompile(`(?m)^-{4,}\s*$`)

	// Magic words
	reMagicWord = regexp.MustCompile(`(?i)__(?:TOC|NOTOC|FORCETOC|NOEDITSECTION|NEWSECTIONLINK|NONEWSECTIONLINK)__`)

	// Cleanup
	reMultiBlank = regexp.MustCompile(`\n{3,}`)
)

// stripTemplates removes {{ ... }} template calls, handling one level of nesting.
func stripTemplates(s string) string {
	// Iteratively strip innermost templates until none remain (max 8 passes).
	for range 8 {
		// Match innermost {{ ... }} (no nested {{ inside)
		idx := strings.Index(s, "{{")
		if idx < 0 {
			break
		}
		replaced := false
		depth := 0
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '{' && s[i+1] == '{' {
				if depth == 0 {
					idx = i
				}
				depth++
				i++ // skip second '{'
			} else if s[i] == '}' && s[i+1] == '}' {
				depth--
				if depth == 0 {
					// Remove this template
					s = s[:idx] + s[i+2:]
					replaced = true
					break
				}
				i++ // skip second '}'
			}
		}
		if !replaced {
			break
		}
	}
	return s
}

// convertLists converts wiki list markup (* and #) to markdown lists.
func convertLists(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")

		// Unordered: *, **, ***
		if len(trimmed) > 0 && trimmed[0] == '*' {
			level := 0
			for level < len(trimmed) && trimmed[level] == '*' {
				level++
			}
			indent := strings.Repeat("  ", level-1)
			lines[i] = indent + "- " + strings.TrimSpace(trimmed[level:])
			continue
		}

		// Ordered: #, ##, ###
		if len(trimmed) > 0 && trimmed[0] == '#' {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			indent := strings.Repeat("  ", level-1)
			lines[i] = indent + "1. " + strings.TrimSpace(trimmed[level:])
			continue
		}

		// Definition list: ; term : definition
		if len(trimmed) > 0 && trimmed[0] == ';' {
			rest := strings.TrimSpace(trimmed[1:])
			if idx := strings.Index(rest, " : "); idx > 0 {
				lines[i] = "**" + strings.TrimSpace(rest[:idx]) + "**: " + strings.TrimSpace(rest[idx+3:])
			} else {
				lines[i] = "**" + rest + "**"
			}
			continue
		}
		if len(trimmed) > 0 && trimmed[0] == ':' {
			lines[i] = "> " + strings.TrimSpace(trimmed[1:])
			continue
		}
	}
	return strings.Join(lines, "\n")
}

// convertTables converts simple wiki tables {| ... |} to markdown text.
func convertTables(s string) string {
	var result strings.Builder
	lines := strings.Split(s, "\n")
	inTable := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "{|") {
			inTable = true
			continue
		}
		if trimmed == "|}" {
			inTable = false
			result.WriteString("\n")
			continue
		}
		if !inTable {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Table caption
		if strings.HasPrefix(trimmed, "|+") {
			result.WriteString("**")
			result.WriteString(strings.TrimSpace(trimmed[2:]))
			result.WriteString("**\n")
			continue
		}

		// Header row
		if strings.HasPrefix(trimmed, "!") {
			cells := splitTableCells(trimmed[1:], "!!")
			result.WriteString("| ")
			result.WriteString(strings.Join(cells, " | "))
			result.WriteString(" |\n")
			// Add separator
			sep := make([]string, len(cells))
			for i := range sep {
				sep[i] = "---"
			}
			result.WriteString("| ")
			result.WriteString(strings.Join(sep, " | "))
			result.WriteString(" |\n")
			continue
		}

		// Row separator |- (must be checked before the generic "|" prefix
		// below, otherwise the data-row branch swallows it).
		if strings.HasPrefix(trimmed, "|-") {
			continue
		}

		// Data row
		if strings.HasPrefix(trimmed, "|") {
			cells := splitTableCells(trimmed[1:], "||")
			result.WriteString("| ")
			result.WriteString(strings.Join(cells, " | "))
			result.WriteString(" |\n")
			continue
		}
	}

	return result.String()
}

// splitTableCells splits a wiki table row into cells, stripping formatting attributes.
func splitTableCells(row string, sep string) []string {
	parts := strings.Split(row, sep)
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Strip cell attributes: "attr | content" → "content"
		if idx := strings.Index(p, " | "); idx > 0 {
			// Check if before | looks like attributes (no wiki markup)
			before := p[:idx]
			if !strings.Contains(before, "[[") && !strings.Contains(before, "{{") {
				p = strings.TrimSpace(p[idx+3:])
			}
		}
		cells = append(cells, p)
	}
	return cells
}
