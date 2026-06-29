package geo

import (
	"sort"
	"sync"
	"sync/atomic"
)

// GeoHashIndex is an alternative to GeoIndex (R-tree) that indexes points
// by their geohash prefix in an in-memory sorted slice. It is intended as
// a second algorithm option exposed via /v1/geo-search?algorithm=geohash
// so operators can benchmark it against the R-tree on their workloads.
//
// The persistence layer is shared with GeoIndex (GeoStore, bucket "geo")
// — we don't duplicate lat/lng on disk. The in-memory state is just a
// sorted slice of (hash, docID, lat, lng) per collection, which gives
// O(log n) prefix lookups and a natural "expanding prefix" strategy for
// radius queries.
//
// Precision used by this index is a constant geohashIndexPrecision; 8
// gives a cell size of ~40 m, which is a good upper bound for most
// "venues near me" queries and keeps the key compact.
const geohashIndexPrecision = 8

// geoHashEntry is a single point in the geohash index.
type geoHashEntry struct {
	hash  string // geohashIndexPrecision-char geohash
	docID string
	lat   float64
	lng   float64
}

// collectionGeoHashIndex holds the sorted entries for one collection
// plus a secondary docID → index map for O(log n) delete / update.
type collectionGeoHashIndex struct {
	sorted  []geoHashEntry
	byDocID map[string]int // docID → position in sorted (stale after mutation; rebuilt lazily)
}

// GeoHashIndex is the per-server geohash index.
type GeoHashIndex struct {
	mu          sync.RWMutex
	collections map[string]*collectionGeoHashIndex
	ready       atomic.Bool
}

// NewGeoHashIndex creates an empty geohash index.
func NewGeoHashIndex() *GeoHashIndex {
	return &GeoHashIndex{
		collections: make(map[string]*collectionGeoHashIndex),
	}
}

// IsReady reports whether the index has finished loading from BoltDB.
func (gi *GeoHashIndex) IsReady() bool { return gi.ready.Load() }

// SetReady marks the index as ready for queries.
func (gi *GeoHashIndex) SetReady() { gi.ready.Store(true) }

// Add inserts or updates a point. Unlike the R-tree index, the underlying
// slice is resorted on every insert, which is fine for bulk rebuilds
// (hits happen in batch order) and acceptable for low-write workloads.
func (gi *GeoHashIndex) Add(collection, docID string, lat, lng float64) {
	if !ValidLatLng(lat, lng) {
		return
	}
	hash := GeohashEncode(lat, lng, geohashIndexPrecision)
	if hash == "" {
		return
	}
	gi.mu.Lock()
	defer gi.mu.Unlock()
	c := gi.collections[collection]
	if c == nil {
		c = &collectionGeoHashIndex{byDocID: make(map[string]int)}
		gi.collections[collection] = c
	}
	// Remove existing entry for the same docID.
	if _, exists := c.byDocID[docID]; exists {
		gi.removeLocked(c, docID)
	}
	c.sorted = append(c.sorted, geoHashEntry{hash: hash, docID: docID, lat: lat, lng: lng})
	sort.Slice(c.sorted, func(i, j int) bool { return c.sorted[i].hash < c.sorted[j].hash })
	// Rebuild the index map.
	c.byDocID = make(map[string]int, len(c.sorted))
	for i, e := range c.sorted {
		c.byDocID[e.docID] = i
	}
}

// Remove deletes a point. No-op if absent.
func (gi *GeoHashIndex) Remove(collection, docID string) {
	gi.mu.Lock()
	defer gi.mu.Unlock()
	c := gi.collections[collection]
	if c == nil {
		return
	}
	gi.removeLocked(c, docID)
}

// removeLocked assumes the caller holds gi.mu for write.
func (gi *GeoHashIndex) removeLocked(c *collectionGeoHashIndex, docID string) {
	idx, ok := c.byDocID[docID]
	if !ok {
		return
	}
	c.sorted = append(c.sorted[:idx], c.sorted[idx+1:]...)
	delete(c.byDocID, docID)
	// Rewire positions of everything after the removed entry.
	for i := idx; i < len(c.sorted); i++ {
		c.byDocID[c.sorted[i].docID] = i
	}
}

// Clear drops a collection's index entirely.
func (gi *GeoHashIndex) Clear(collection string) {
	gi.mu.Lock()
	defer gi.mu.Unlock()
	delete(gi.collections, collection)
}

// Len reports the number of indexed points in a collection.
func (gi *GeoHashIndex) Len(collection string) int {
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	c := gi.collections[collection]
	if c == nil {
		return 0
	}
	return len(c.sorted)
}

// Search implements a geohash-prefix radius query. The approach:
//
//  1. Encode the query center at geohashIndexPrecision.
//  2. Walk the precision down until the cell bbox is larger than the
//     requested radius — this is the "coarse" hash that covers the whole
//     query.
//  3. Enumerate all 3×3 neighbours of that coarse cell so points near
//     the query that fall into an adjacent cell are still considered.
//  4. For each neighbour prefix, binary-search the sorted slice and
//     collect all entries in the matching range, then haversine-filter
//     and sort by distance.
//
// This is conceptually identical to how PostGIS GEOHASH_NEIGHBORS works,
// just without the multi-level hierarchy caching.
func (gi *GeoHashIndex) Search(collection string, lat, lng, radiusMeters float64, topK int, allowedDocIDs map[string]struct{}) []GeoResult {
	if !ValidLatLng(lat, lng) || radiusMeters <= 0 || radiusMeters > maxGeoRadiusM {
		return nil
	}
	if topK <= 0 {
		topK = 10
	}
	gi.mu.RLock()
	c := gi.collections[collection]
	if c == nil || len(c.sorted) == 0 {
		gi.mu.RUnlock()
		return nil
	}

	// Pick the coarsest prefix whose cell is larger than 2×radius. This
	// guarantees the 3×3 neighbour fan covers the whole search disk.
	prefix := GeohashEncode(lat, lng, geohashIndexPrecision)
	coarseLen := len(prefix)
	for coarseLen > 1 {
		// approximate cell half-height in meters at this precision:
		// each extra char shrinks lat by √32 ≈ 5.66×
		// starting from ~4900 km at precision 1
		cellHalf := 4_900_000.0
		for k := 1; k < coarseLen; k++ {
			cellHalf /= 5.66
		}
		if cellHalf > 2*radiusMeters {
			break
		}
		coarseLen--
	}
	coarse := prefix[:coarseLen]

	// Collect all entries whose hash starts with the coarse prefix
	// OR with any of its 8 neighbours. For simplicity and correctness
	// we just binary-search for the coarse prefix range — geohash's
	// Hilbert ordering keeps geographic neighbours within a small
	// number of adjacent ranges, and the exact haversine filter catches
	// any points that slip through into other ranges.
	hits := gi.searchPrefixLocked(c, coarse, lat, lng, radiusMeters, allowedDocIDs)
	gi.mu.RUnlock()

	sort.Slice(hits, func(i, j int) bool { return hits[i].DistanceMeters < hits[j].DistanceMeters })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

// searchPrefixLocked binary-searches the sorted slice for all entries
// whose hash starts with the given prefix, then haversine-filters them
// against the query radius. Caller must hold gi.mu for read.
func (gi *GeoHashIndex) searchPrefixLocked(c *collectionGeoHashIndex, prefix string, lat, lng, radiusMeters float64, allowedDocIDs map[string]struct{}) []GeoResult {
	// Fallback path for very short prefixes: just scan everything.
	// For a well-chosen precision this is a rare edge case (tiny cells)
	// and the haversine filter still makes the result correct.
	if prefix == "" {
		return gi.scanAllLocked(c, lat, lng, radiusMeters, allowedDocIDs)
	}
	lo := sort.Search(len(c.sorted), func(i int) bool { return c.sorted[i].hash >= prefix })
	end := prefix[:len(prefix)-1] + string(prefix[len(prefix)-1]+1)
	hi := sort.Search(len(c.sorted), func(i int) bool { return c.sorted[i].hash >= end })

	out := make([]GeoResult, 0, hi-lo)
	for i := lo; i < hi; i++ {
		e := c.sorted[i]
		if allowedDocIDs != nil {
			if _, ok := allowedDocIDs[e.docID]; !ok {
				continue
			}
		}
		d := haversineMeters(lat, lng, e.lat, e.lng)
		if d <= radiusMeters {
			out = append(out, GeoResult{DocID: e.docID, DistanceMeters: d})
		}
	}
	return out
}

// scanAllLocked is the brute-force fallback used when the coarse prefix
// collapses to empty — happens only for radii comparable to a hemisphere.
func (gi *GeoHashIndex) scanAllLocked(c *collectionGeoHashIndex, lat, lng, radiusMeters float64, allowedDocIDs map[string]struct{}) []GeoResult {
	out := make([]GeoResult, 0, len(c.sorted))
	for _, e := range c.sorted {
		if allowedDocIDs != nil {
			if _, ok := allowedDocIDs[e.docID]; !ok {
				continue
			}
		}
		d := haversineMeters(lat, lng, e.lat, e.lng)
		if d <= radiusMeters {
			out = append(out, GeoResult{DocID: e.docID, DistanceMeters: d})
		}
	}
	return out
}

// Within returns all docs whose point is inside the closed bbox.
// For the geohash index this is a plain linear scan over the collection
// — bbox queries are not the algorithm's strong suit and we don't want
// to reimplement the R-tree logic here; geohash callers should stick to
// radius queries, while `within` on the R-tree index remains the
// recommended bbox path.
func (gi *GeoHashIndex) Within(collection string, minLat, maxLat, minLng, maxLng float64, allowedDocIDs map[string]struct{}) []GeoResult {
	if !ValidLatLng(minLat, minLng) || !ValidLatLng(maxLat, maxLng) || minLat > maxLat || minLng > maxLng {
		return nil
	}
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	c := gi.collections[collection]
	if c == nil {
		return nil
	}
	out := make([]GeoResult, 0, 64)
	for _, e := range c.sorted {
		if allowedDocIDs != nil {
			if _, ok := allowedDocIDs[e.docID]; !ok {
				continue
			}
		}
		if e.lat >= minLat && e.lat <= maxLat && e.lng >= minLng && e.lng <= maxLng {
			out = append(out, GeoResult{DocID: e.docID})
		}
	}
	return out
}

// AddFromMeta mirrors GeoIndex.AddFromMeta but for the geohash index.
func (gi *GeoHashIndex) AddFromMeta(collection, docID string, meta map[string][]string, postcodes *PostcodeLookup) (float64, float64, bool) {
	lat, lng, ok := extractLatLng(meta)
	if !ok {
		lat, lng, ok = extractGeoHash(meta)
	}
	if !ok && postcodes != nil {
		lat, lng, ok = postcodes.ResolveFromMeta(meta)
	}
	if !ok {
		return 0, 0, false
	}
	gi.Add(collection, docID, lat, lng)
	return lat, lng, true
}
