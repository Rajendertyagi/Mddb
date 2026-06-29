package main

import (
	"mddb/internal/fts"
	"sort"
	"strings"
)

// BoostMinMultiplier is the floor applied to per-doc boost multipliers so
// a large stack of negative boosts cannot collapse score to zero.
const BoostMinMultiplier = 0.001

// parseBoostKey splits a boost key "metaKey:metaValue" into its parts.
// Returns ("", "", false) when the key is malformed.
func parseBoostKey(key string) (metaKey, metaValue string, ok bool) {
	idx := strings.IndexByte(key, ':')
	if idx <= 0 || idx == len(key)-1 {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}

// normalizeBoostFactor converts a user-supplied boost value into a positive
// multiplier. Positive values multiply directly (5.0 → 5x), negative values
// act as an inverse demotion (-2.0 → 0.5x), zero is ignored.
func normalizeBoostFactor(v float64) float64 {
	if v > 0 {
		return v
	}
	if v < 0 {
		return 1.0 / -v
	}
	return 1.0
}

// buildBoostLookup resolves each boost entry to the set of docIDs that match
// its metaKey:metaValue predicate. Entries with invalid keys or unmatched
// docs are skipped. Zero-valued boosts are treated as no-ops.
func (s *Server) buildBoostLookup(collection string, boost map[string]float64) []boostGroup {
	if len(boost) == 0 {
		return nil
	}
	groups := make([]boostGroup, 0, len(boost))
	for key, val := range boost {
		if val == 0 {
			continue
		}
		metaKey, metaValue, ok := parseBoostKey(key)
		if !ok {
			continue
		}
		ids := s.getDocIDsByMeta(collection, map[string][]string{metaKey: {metaValue}})
		if len(ids) == 0 {
			continue
		}
		groups = append(groups, boostGroup{factor: normalizeBoostFactor(val), docs: ids})
	}
	return groups
}

// boostGroup pairs a computed multiplier with the set of docIDs it applies to.
type boostGroup struct {
	factor float64
	docs   map[string]bool
}

// docMultiplier returns the combined multiplier for a docID across all groups.
func docMultiplier(groups []boostGroup, docID string) float64 {
	if len(groups) == 0 {
		return 1.0
	}
	m := 1.0
	for _, g := range groups {
		if g.docs[docID] {
			m *= g.factor
		}
	}
	if m < BoostMinMultiplier {
		m = BoostMinMultiplier
	}
	return m
}

// applyBoostFTS rewrites fts.FTSResult scores in-place using the given boost
// configuration and re-sorts the slice by descending score. It is safe to
// call with a nil or empty boost map — in that case the slice is returned
// unchanged.
func (s *Server) applyBoostFTS(collection string, results []fts.FTSResult, boost map[string]float64) []fts.FTSResult {
	groups := s.buildBoostLookup(collection, boost)
	if len(groups) == 0 || len(results) == 0 {
		return results
	}
	for i := range results {
		results[i].Score *= docMultiplier(groups, results[i].DocID)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

// applyBoostHybrid rewrites HybridSearchResultItem CombinedScore in-place
// using the boost configuration and re-sorts the slice. FTSScore and
// VectorScore remain untouched so callers can still see the raw sub-scores.
func (s *Server) applyBoostHybrid(collection string, items []HybridSearchResultItem, boost map[string]float64) []HybridSearchResultItem {
	groups := s.buildBoostLookup(collection, boost)
	if len(groups) == 0 || len(items) == 0 {
		return items
	}
	for i := range items {
		items[i].CombinedScore *= docMultiplier(groups, items[i].Document.ID)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CombinedScore > items[j].CombinedScore
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	return items
}
