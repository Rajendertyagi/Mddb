package main

import (
	"mddb/internal/fts"
	"sort"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// RangeFilter defines a range condition on a field.
type RangeFilter struct {
	Field string `json:"field"`         // metadata key, or "addedAt" / "updatedAt"
	Gte   string `json:"gte,omitempty"` // greater than or equal
	Lte   string `json:"lte,omitempty"` // less than or equal
	Gt    string `json:"gt,omitempty"`  // greater than
	Lt    string `json:"lt,omitempty"`  // less than
}

// SearchRange filters documents by range conditions on metadata or timestamps.
// Can be combined with other search results as a post-filter.
func (s *Server) SearchRange(collection string, ranges []RangeFilter, limit int) ([]fts.FTSResult, error) {
	if len(ranges) == 0 {
		return nil, nil
	}

	type docScore struct {
		id    string
		score float64
	}
	scores := make(map[string]*docScore)

	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(s.BucketNames.Docs)
		if bDocs == nil {
			return nil
		}

		// Scan all docs in collection
		prefix := []byte("doc|" + collection + "|")
		c := bDocs.Cursor()
		for k, v := c.Seek(prefix); k != nil; k, v = c.Next() {
			if len(k) < len(prefix) || string(k[:len(prefix)]) != string(prefix) {
				break
			}

			doc, err := loadDoc(v)
			if err != nil || doc == nil {
				continue
			}

			// Skip expired
			if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
				continue
			}

			// Check all range filters
			match := true
			for _, rf := range ranges {
				if !matchRangeFilter(doc, rf) {
					match = false
					break
				}
			}

			if match {
				docID := string(k[len(prefix):])
				scores[docID] = &docScore{id: docID, score: 1.0}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	results := make([]fts.FTSResult, 0, len(scores))
	for _, ds := range scores {
		results = append(results, fts.FTSResult{
			DocID: ds.id,
			Score: ds.score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DocID < results[j].DocID
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// FilterByRange applies range filters to an existing result set.
func (s *Server) FilterByRange(collection string, results []fts.FTSResult, ranges []RangeFilter) ([]fts.FTSResult, error) {
	if len(ranges) == 0 {
		return results, nil
	}

	filtered := results[:0]
	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket(s.BucketNames.Docs)
		if bDocs == nil {
			return nil
		}

		for _, r := range results {
			v := bDocs.Get(kDoc(collection, r.DocID))
			if v == nil {
				continue
			}
			doc, err := loadDoc(v)
			if err != nil || doc == nil {
				continue
			}

			match := true
			for _, rf := range ranges {
				if !matchRangeFilter(doc, rf) {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, r)
			}
		}
		return nil
	})
	return filtered, err
}

// matchRangeFilter checks if a document matches a single range filter.
func matchRangeFilter(doc *Doc, rf RangeFilter) bool {
	field := strings.ToLower(rf.Field)

	// Built-in timestamp fields
	switch field {
	case "addedat":
		return matchTimestampRange(doc.AddedAt, rf)
	case "updatedat":
		return matchTimestampRange(doc.UpdatedAt, rf)
	}

	// Metadata field: get first value
	values, ok := doc.Meta[rf.Field]
	if !ok || len(values) == 0 {
		return false
	}
	val := values[0]

	// Try numeric comparison first
	if numVal, err := strconv.ParseFloat(val, 64); err == nil {
		return matchNumericRange(numVal, rf)
	}

	// Try date comparison
	if t, err := parseFlexibleDate(val); err == nil {
		return matchTimestampRange(t.Unix(), rf)
	}

	// Fall back to string comparison
	return matchStringRange(val, rf)
}

// matchTimestampRange checks a unix timestamp against range boundaries.
func matchTimestampRange(ts int64, rf RangeFilter) bool {
	if rf.Gte != "" {
		boundary := parseRangeBoundary(rf.Gte)
		if ts < boundary {
			return false
		}
	}
	if rf.Gt != "" {
		boundary := parseRangeBoundary(rf.Gt)
		if ts <= boundary {
			return false
		}
	}
	if rf.Lte != "" {
		boundary := parseRangeBoundary(rf.Lte)
		if ts > boundary {
			return false
		}
	}
	if rf.Lt != "" {
		boundary := parseRangeBoundary(rf.Lt)
		if ts >= boundary {
			return false
		}
	}
	return true
}

// matchNumericRange checks a numeric value against range boundaries.
func matchNumericRange(val float64, rf RangeFilter) bool {
	if rf.Gte != "" {
		if b, err := strconv.ParseFloat(rf.Gte, 64); err == nil && val < b {
			return false
		}
	}
	if rf.Gt != "" {
		if b, err := strconv.ParseFloat(rf.Gt, 64); err == nil && val <= b {
			return false
		}
	}
	if rf.Lte != "" {
		if b, err := strconv.ParseFloat(rf.Lte, 64); err == nil && val > b {
			return false
		}
	}
	if rf.Lt != "" {
		if b, err := strconv.ParseFloat(rf.Lt, 64); err == nil && val >= b {
			return false
		}
	}
	return true
}

// matchStringRange checks a string value against range boundaries (lexicographic).
func matchStringRange(val string, rf RangeFilter) bool {
	if rf.Gte != "" && val < rf.Gte {
		return false
	}
	if rf.Gt != "" && val <= rf.Gt {
		return false
	}
	if rf.Lte != "" && val > rf.Lte {
		return false
	}
	if rf.Lt != "" && val >= rf.Lt {
		return false
	}
	return true
}

// parseRangeBoundary parses a boundary value as unix timestamp or date string.
func parseRangeBoundary(s string) int64 {
	// Try as unix timestamp
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	// Try as date
	if t, err := parseFlexibleDate(s); err == nil {
		return t.Unix()
	}
	return 0
}

// parseFlexibleDate tries common date formats.
func parseFlexibleDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
		"2006/01/02",
		"02-01-2006",
		"Jan 2, 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}
