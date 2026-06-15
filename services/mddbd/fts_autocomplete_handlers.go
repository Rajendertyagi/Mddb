package main

import (
	"errors"
	"net/http"
)

// AutocompleteResponse is returned by the HTTP handler.
type AutocompleteResponse struct {
	Items []AutocompleteItem `json:"items"`
	Total int                `json:"total"`
	Query string             `json:"query"`
	Field string             `json:"field,omitempty"`
}

// handleAutocomplete serves GET /v1/autocomplete?collection=X&q=mar&field=Y&topN=10.
func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.FTSIndex == nil {
		bad(w, errors.New("full-text search not initialized"))
		return
	}

	collection := r.URL.Query().Get("collection")
	query := r.URL.Query().Get("q")
	field := r.URL.Query().Get("field")
	topN := parseIntParam(r, "topN", 10)

	if collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if query == "" {
		// Empty prefix returns empty result set rather than 400 — saves
		// client-side guards on every keystroke that clears the input.
		ok(w, AutocompleteResponse{Items: []AutocompleteItem{}, Query: query, Field: field})
		return
	}

	items, err := s.FTSIndex.Autocomplete(collection, query, field, topN)
	if err != nil {
		bad(w, err)
		return
	}
	if s.Metrics != nil {
		s.Metrics.IncOp("autocomplete", field)
	}
	ok(w, AutocompleteResponse{
		Items: items,
		Total: len(items),
		Query: query,
		Field: field,
	})
}

// parseIntParam reads a URL query parameter as int with a fallback. Used for
// numeric knobs (topN, limit) where zero/negative means "use default".
func parseIntParam(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}
