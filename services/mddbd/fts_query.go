package main

import (
	"strings"
	"unicode"
)

// QueryClause represents a parsed element in an advanced search query.
type QueryClause struct {
	Type      string // "term", "phrase", "proximity", "wildcard", "not"
	Value     string // the term/phrase text
	Operator  string // "AND", "OR" (between clauses)
	Distance  int    // proximity distance (for "proximity" type)
	IsNegated bool   // NOT / - prefix
}

// ParsedQuery is the result of parsing an advanced search query.
type ParsedQuery struct {
	Clauses       []QueryClause
	HasBoolean    bool
	HasPhrase     bool
	HasProximity  bool
	HasWildcard   bool
	DefaultOp     string // "AND" or "OR" for implicit operator
	OriginalQuery string
}

// ParseAdvancedQuery parses a query string into structured clauses.
// Supported syntax:
//   - Simple terms: rust performance
//   - Phrases: "machine learning"
//   - Proximity: "rust performance"~5
//   - Boolean: rust AND performance, rust OR go, NOT java, +required -excluded
//   - Wildcards: prog*, te?t
func ParseAdvancedQuery(query string) *ParsedQuery {
	pq := &ParsedQuery{
		OriginalQuery: query,
		DefaultOp:     "AND",
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return pq
	}

	runes := []rune(query)
	i := 0
	pendingOp := ""

	for i < len(runes) {
		// Skip whitespace
		for i < len(runes) && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= len(runes) {
			break
		}

		// Check for boolean operators
		rest := string(runes[i:])
		if strings.HasPrefix(rest, "AND ") || strings.HasPrefix(rest, "AND\t") {
			pendingOp = "AND"
			pq.HasBoolean = true
			i += 4
			continue
		}
		if strings.HasPrefix(rest, "OR ") || strings.HasPrefix(rest, "OR\t") {
			pendingOp = "OR"
			pq.HasBoolean = true
			i += 3
			continue
		}
		if strings.HasPrefix(rest, "NOT ") || strings.HasPrefix(rest, "NOT\t") {
			pq.HasBoolean = true
			// Parse next clause as negated
			i += 4
			clause := parseNextClause(runes, &i)
			clause.IsNegated = true
			clause.Operator = pendingOp
			pendingOp = ""
			pq.Clauses = append(pq.Clauses, clause)
			updateFlags(pq, clause)
			continue
		}

		// Check for +/- prefix
		negated := false
		required := false
		if i < len(runes) && runes[i] == '-' && i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			negated = true
			pq.HasBoolean = true
			i++
		} else if i < len(runes) && runes[i] == '+' && i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			required = true
			pq.HasBoolean = true
			i++
		}
		_ = required // required terms use AND implicitly

		clause := parseNextClause(runes, &i)
		clause.IsNegated = negated
		clause.Operator = pendingOp
		pendingOp = ""
		pq.Clauses = append(pq.Clauses, clause)
		updateFlags(pq, clause)
	}

	return pq
}

// parseNextClause extracts the next clause starting at position i.
func parseNextClause(runes []rune, i *int) QueryClause {
	if *i >= len(runes) {
		return QueryClause{}
	}

	// Quoted phrase or proximity
	if runes[*i] == '"' {
		*i++ // skip opening quote
		var sb strings.Builder
		for *i < len(runes) && runes[*i] != '"' {
			sb.WriteRune(runes[*i])
			*i++
		}
		if *i < len(runes) {
			*i++ // skip closing quote
		}
		phrase := sb.String()

		// Check for proximity: ~N
		if *i < len(runes) && runes[*i] == '~' {
			*i++
			dist := parseNumber(runes, i)
			if dist > 0 {
				return QueryClause{
					Type:     "proximity",
					Value:    phrase,
					Distance: dist,
				}
			}
		}

		return QueryClause{
			Type:  "phrase",
			Value: phrase,
		}
	}

	// Regular term (possibly with wildcards)
	var sb strings.Builder
	for *i < len(runes) && !unicode.IsSpace(runes[*i]) {
		sb.WriteRune(runes[*i])
		*i++
	}

	term := sb.String()
	if term == "" {
		return QueryClause{}
	}

	// Check for wildcards
	if strings.ContainsAny(term, "*?") {
		return QueryClause{
			Type:  "wildcard",
			Value: term,
		}
	}

	return QueryClause{
		Type:  "term",
		Value: term,
	}
}

// parseNumber extracts a decimal number from runes starting at i.
func parseNumber(runes []rune, i *int) int {
	n := 0
	for *i < len(runes) && runes[*i] >= '0' && runes[*i] <= '9' {
		n = n*10 + int(runes[*i]-'0')
		*i++
	}
	return n
}

// updateFlags sets the HasXxx flags on ParsedQuery based on a clause type.
func updateFlags(pq *ParsedQuery, c QueryClause) {
	switch c.Type {
	case "phrase":
		pq.HasPhrase = true
	case "proximity":
		pq.HasProximity = true
	case "wildcard":
		pq.HasWildcard = true
	}
}

// IsAdvanced returns true if the query uses any advanced syntax.
func (pq *ParsedQuery) IsAdvanced() bool {
	return pq.HasBoolean || pq.HasPhrase || pq.HasProximity || pq.HasWildcard
}
