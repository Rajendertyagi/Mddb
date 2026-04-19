package main

import (
	"fmt"
	"math"
)

// --- GeoJSON types ---

// GeoJSONPolygon follows RFC 7946 §3.1.6. The outer `coordinates` slice
// holds rings: the first ring is the polygon's outer boundary; any
// subsequent rings are holes. Each ring is a closed line string —
// [[lng,lat], [lng,lat], …, [lng,lat]] with the first and last point equal.
// Callers may omit the closing point; validatePolygon tolerates both forms.
type GeoJSONPolygon struct {
	Type        string        `json:"type"`
	Coordinates [][][]float64 `json:"coordinates"`
}

// GeoJSONMultiPolygon is RFC 7946 §3.1.7 — an array of Polygon coordinate
// arrays. A document is considered "inside" a MultiPolygon if it falls
// inside any of the member polygons (union semantics).
type GeoJSONMultiPolygon struct {
	Type        string          `json:"type"`
	Coordinates [][][][]float64 `json:"coordinates"`
}

// --- Validation ---

// validatePolygon rejects malformed input before any index scan so the
// caller gets a clean 400 rather than silently empty results from an
// unsatisfiable query.
func validatePolygon(p *GeoJSONPolygon) error {
	if p == nil {
		return fmt.Errorf("polygon is nil")
	}
	if p.Type != "" && p.Type != "Polygon" {
		return fmt.Errorf("polygon.type must be \"Polygon\", got %q", p.Type)
	}
	if len(p.Coordinates) == 0 {
		return fmt.Errorf("polygon.coordinates must have at least one ring")
	}
	for i, ring := range p.Coordinates {
		if len(ring) < 3 {
			return fmt.Errorf("polygon ring %d must have at least 3 points", i)
		}
		for j, pt := range ring {
			if len(pt) < 2 {
				return fmt.Errorf("polygon ring %d point %d must have [lng, lat]", i, j)
			}
			if !validLatLng(pt[1], pt[0]) {
				return fmt.Errorf("polygon ring %d point %d has invalid lat/lng: %v", i, j, pt)
			}
		}
	}
	return nil
}

// validateMultiPolygon applies validatePolygon to every member.
func validateMultiPolygon(mp *GeoJSONMultiPolygon) error {
	if mp == nil {
		return fmt.Errorf("multiPolygon is nil")
	}
	if mp.Type != "" && mp.Type != "MultiPolygon" {
		return fmt.Errorf("multiPolygon.type must be \"MultiPolygon\", got %q", mp.Type)
	}
	if len(mp.Coordinates) == 0 {
		return fmt.Errorf("multiPolygon.coordinates must have at least one polygon")
	}
	for i, poly := range mp.Coordinates {
		if err := validatePolygon(&GeoJSONPolygon{Type: "Polygon", Coordinates: poly}); err != nil {
			return fmt.Errorf("polygon %d: %w", i, err)
		}
	}
	return nil
}

// --- Ray casting (point-in-polygon) ---

// pointInRing implements the standard even-odd crossing test for a 2D
// point against a closed polygon ring. Ring coordinates follow GeoJSON
// convention: each entry is [lng, lat]. Returns true when the point lies
// strictly inside or on an edge pointing "right". The tiny numerical
// asymmetry on edges is acceptable for spatial filtering — no caller
// ties a business rule to exact boundary membership.
func pointInRing(lat, lng float64, ring [][]float64) bool {
	n := len(ring)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		// GeoJSON order: ring[k][0] = lng, ring[k][1] = lat.
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > lat) != (yj > lat) {
			// Horizontal ray from (lng, lat) to +∞ crosses edge (i-1,i).
			slope := (xj - xi) * (lat - yi) / (yj - yi)
			if lng < slope+xi {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// pointInPolygon treats the first ring as the outer boundary and any
// subsequent rings as holes. A point is "inside" the polygon iff it lies
// inside the outer ring AND not inside any hole.
func pointInPolygon(lat, lng float64, coords [][][]float64) bool {
	if len(coords) == 0 {
		return false
	}
	if !pointInRing(lat, lng, coords[0]) {
		return false
	}
	for _, hole := range coords[1:] {
		if pointInRing(lat, lng, hole) {
			return false
		}
	}
	return true
}

// pointInMultiPolygon returns true if the point falls inside any member
// polygon (union semantics). Early-exits on first hit.
func pointInMultiPolygon(lat, lng float64, coords [][][][]float64) bool {
	for _, poly := range coords {
		if pointInPolygon(lat, lng, poly) {
			return true
		}
	}
	return false
}

// --- Bounds ---

// polygonBounds returns the axis-aligned bounding box of all points in
// the polygon (outer ring + holes). Used to prefilter R-tree candidates
// so the point-in-polygon test only runs on plausible hits, not the
// entire collection.
func polygonBounds(coords [][][]float64) (minLat, maxLat, minLng, maxLng float64, ok bool) {
	minLat, minLng = math.Inf(1), math.Inf(1)
	maxLat, maxLng = math.Inf(-1), math.Inf(-1)
	seen := 0
	for _, ring := range coords {
		for _, pt := range ring {
			if len(pt) < 2 {
				continue
			}
			lng, lat := pt[0], pt[1]
			if lat < minLat {
				minLat = lat
			}
			if lat > maxLat {
				maxLat = lat
			}
			if lng < minLng {
				minLng = lng
			}
			if lng > maxLng {
				maxLng = lng
			}
			seen++
		}
	}
	if seen == 0 {
		return 0, 0, 0, 0, false
	}
	return minLat, maxLat, minLng, maxLng, true
}

// multiPolygonBounds returns the bbox enclosing every member polygon.
func multiPolygonBounds(coords [][][][]float64) (minLat, maxLat, minLng, maxLng float64, ok bool) {
	minLat, minLng = math.Inf(1), math.Inf(1)
	maxLat, maxLng = math.Inf(-1), math.Inf(-1)
	anySeen := false
	for _, poly := range coords {
		a, b, c, d, polyOK := polygonBounds(poly)
		if !polyOK {
			continue
		}
		anySeen = true
		if a < minLat {
			minLat = a
		}
		if b > maxLat {
			maxLat = b
		}
		if c < minLng {
			minLng = c
		}
		if d > maxLng {
			maxLng = d
		}
	}
	if !anySeen {
		return 0, 0, 0, 0, false
	}
	return minLat, maxLat, minLng, maxLng, true
}

// --- R-tree prefilter + polygon test ---

// SearchPolygon returns all points in a collection that fall inside the
// given polygon. The R-tree narrows the candidate set to the polygon's
// bounding box first; each survivor is then ray-cast against the actual
// ring geometry. allowedDocIDs, when non-nil, scopes the result set to
// a caller-maintained allow-list (used for metadata pre-filtering on the
// handler side, matching existing geo-search and geo-within semantics).
func (gi *GeoIndex) SearchPolygon(collection string, coords [][][]float64, allowedDocIDs map[string]struct{}) []GeoResult {
	minLat, maxLat, minLng, maxLng, ok := polygonBounds(coords)
	if !ok {
		return nil
	}
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	c := gi.collections[collection]
	if c == nil || c.tree.Len() == 0 {
		return nil
	}
	out := make([]GeoResult, 0, 64)
	c.tree.Search(
		[2]float64{minLng, minLat},
		[2]float64{maxLng, maxLat},
		func(min, _ [2]float64, docID string) bool {
			if allowedDocIDs != nil {
				if _, ok := allowedDocIDs[docID]; !ok {
					return true
				}
			}
			// For point entries min == max, so either corner carries the
			// real coordinates. Matches the pattern used by Search() above.
			lng, lat := min[0], min[1]
			if pointInPolygon(lat, lng, coords) {
				out = append(out, GeoResult{DocID: docID})
			}
			return true
		},
	)
	return out
}

// SearchMultiPolygon is the union-over-polygons variant. Uses the outer
// bbox to prefilter the R-tree once, then runs pointInMultiPolygon on
// each candidate. Cheaper than calling SearchPolygon N times when the
// member polygons overlap or cluster geographically.
func (gi *GeoIndex) SearchMultiPolygon(collection string, coords [][][][]float64, allowedDocIDs map[string]struct{}) []GeoResult {
	minLat, maxLat, minLng, maxLng, ok := multiPolygonBounds(coords)
	if !ok {
		return nil
	}
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	c := gi.collections[collection]
	if c == nil || c.tree.Len() == 0 {
		return nil
	}
	out := make([]GeoResult, 0, 64)
	c.tree.Search(
		[2]float64{minLng, minLat},
		[2]float64{maxLng, maxLat},
		func(min, _ [2]float64, docID string) bool {
			if allowedDocIDs != nil {
				if _, ok := allowedDocIDs[docID]; !ok {
					return true
				}
			}
			lng, lat := min[0], min[1]
			if pointInMultiPolygon(lat, lng, coords) {
				out = append(out, GeoResult{DocID: docID})
			}
			return true
		},
	)
	return out
}
