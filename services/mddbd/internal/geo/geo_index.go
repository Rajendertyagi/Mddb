package geo

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/rtree"
)

// earthRadiusM is the mean Earth radius (WGS84) in meters, used by haversine.
const earthRadiusM = 6371000.0

// maxGeoRadiusM caps a single radius query at half the planet's circumference
// so obvious garbage (NaN/Inf/negative-as-uint-overflow) is rejected early.
const maxGeoRadiusM = 50_000_000.0

// Reserved document metadata keys. Users write these in frontmatter; the
// index extracts them at Add time. See docs/GEOSEARCH.md.
const (
	metaKeyGeoLat      = "geo_lat"
	metaKeyGeoLng      = "geo_lng"
	metaKeyGeoPostcode = "geo_postcode"
	metaKeyGeoCountry  = "geo_country"
)

// geoPoint is what we store per docID: the raw lat/lng so Remove can reconstruct
// the bbox the R-tree was indexed with, and so we can re-score candidates via
// exact haversine after an R-tree bbox prefilter.
type geoPoint struct {
	lat, lng float64
}

// GeoResult represents a single geo search result.
type GeoResult struct {
	DocID          string
	DistanceMeters float64
}

// collectionGeoIndex holds the in-memory R-tree for one collection plus a
// secondary docID → point map. tidwall/rtree deletes by (bbox, payload), so we
// need the point to reconstruct the bbox on Remove — we cannot look it up from
// docID alone.
type collectionGeoIndex struct {
	tree   *rtree.RTreeG[string]
	points map[string]geoPoint
}

// GeoIndex is an in-memory R-tree index for 2D geographic points, keyed per
// collection. The lifecycle mirrors VectorIndex: RWMutex-protected map of
// per-collection sub-indexes, atomic.Bool ready flag for async startup
// rebuild, and RLock released before exact scoring.
type GeoIndex struct {
	mu          sync.RWMutex
	collections map[string]*collectionGeoIndex
	ready       atomic.Bool
	lastRebuild map[string]time.Time
	postcodes   *PostcodeLookup // optional; set via SetPostcodes
	postcodesMu sync.RWMutex
}

// NewGeoIndex creates an empty geo index.
func NewGeoIndex() *GeoIndex {
	return &GeoIndex{
		collections: make(map[string]*collectionGeoIndex),
		lastRebuild: make(map[string]time.Time),
	}
}

// IsReady reports whether the index has finished loading from BoltDB.
func (gi *GeoIndex) IsReady() bool { return gi.ready.Load() }

// SetReady marks the index as ready for queries.
func (gi *GeoIndex) SetReady() { gi.ready.Store(true) }

// SetPostcodes attaches an optional postcode lookup used by AddFromMeta when
// a document has only geo_postcode/geo_country and no explicit geo_lat/geo_lng.
func (gi *GeoIndex) SetPostcodes(p *PostcodeLookup) {
	gi.postcodesMu.Lock()
	defer gi.postcodesMu.Unlock()
	gi.postcodes = p
}

// Postcodes returns the current postcode lookup (may be nil).
func (gi *GeoIndex) Postcodes() *PostcodeLookup {
	gi.postcodesMu.RLock()
	defer gi.postcodesMu.RUnlock()
	return gi.postcodes
}

// Add inserts or updates a single point. If the docID already exists it is
// replaced, so the index stays consistent with document updates.
func (gi *GeoIndex) Add(collection, docID string, lat, lng float64) {
	if !ValidLatLng(lat, lng) {
		return
	}
	gi.mu.Lock()
	defer gi.mu.Unlock()
	c := gi.collections[collection]
	if c == nil {
		c = &collectionGeoIndex{
			tree:   &rtree.RTreeG[string]{},
			points: make(map[string]geoPoint),
		}
		gi.collections[collection] = c
	}
	if old, ok := c.points[docID]; ok {
		c.tree.Delete([2]float64{old.lng, old.lat}, [2]float64{old.lng, old.lat}, docID)
	}
	c.tree.Insert([2]float64{lng, lat}, [2]float64{lng, lat}, docID)
	c.points[docID] = geoPoint{lat: lat, lng: lng}
}

// Remove deletes a point from the index. No-op if absent.
func (gi *GeoIndex) Remove(collection, docID string) {
	gi.mu.Lock()
	defer gi.mu.Unlock()
	c := gi.collections[collection]
	if c == nil {
		return
	}
	old, ok := c.points[docID]
	if !ok {
		return
	}
	c.tree.Delete([2]float64{old.lng, old.lat}, [2]float64{old.lng, old.lat}, docID)
	delete(c.points, docID)
}

// AddFromMeta extracts coordinates from a document's metadata in priority
// order and adds the resulting point to the index:
//
//  1. Explicit geo_lat + geo_lng (float64 strings) — canonical, fastest.
//  2. geo_hash — a canonical geohash string. Decoded via GeohashDecode
//     to the centroid of the cell. This is a third ingest path beyond
//     explicit coords and postcode fallback; useful for clients that
//     only have a geohash from an upstream system.
//  3. geo_postcode + geo_country — resolved via the optional postcode
//     lookup (see PostcodeLookup). Only consulted if SetPostcodes was
//     called with a populated lookup for that country.
//
// Returns (lat, lng, true) on success, or (0, 0, false) if the document
// has no usable geo info. Used by the storage.go hook on Add/Update and
// by GeoStore.Rebuild at startup.
func (gi *GeoIndex) AddFromMeta(collection, docID string, meta map[string][]string) (float64, float64, bool) {
	lat, lng, ok := extractLatLng(meta)
	if !ok {
		lat, lng, ok = extractGeoHash(meta)
	}
	if !ok {
		if pc := gi.Postcodes(); pc != nil {
			lat, lng, ok = pc.ResolveFromMeta(meta)
		}
	}
	if !ok {
		return 0, 0, false
	}
	gi.Add(collection, docID, lat, lng)
	return lat, lng, true
}

// Search returns docs within radiusMeters of (lat, lng), ordered by ascending
// distance, truncated to topK. If allowedDocIDs is non-nil, only docs present
// in that set are returned (used for FilterMeta / hybrid composition).
func (gi *GeoIndex) Search(collection string, lat, lng, radiusMeters float64, topK int, allowedDocIDs map[string]struct{}) []GeoResult {
	if !ValidLatLng(lat, lng) || radiusMeters <= 0 || radiusMeters > maxGeoRadiusM {
		return nil
	}
	if topK <= 0 {
		topK = 10
	}

	gi.mu.RLock()
	c := gi.collections[collection]
	if c == nil || c.tree.Len() == 0 {
		gi.mu.RUnlock()
		return nil
	}
	// Convert radius to a conservative lat/lng bbox. 1 degree of latitude is
	// ~111.32 km at sea level. Longitude shrinks with cos(lat); guard against
	// cos(lat)≈0 near the poles by clamping to a wide bbox there — the R-tree
	// pre-filter is inclusive and the haversine pass below fixes precision.
	latDelta := radiusMeters / 111320.0
	cosLat := math.Cos(lat * math.Pi / 180)
	var lngDelta float64
	if math.Abs(cosLat) < 1e-6 {
		lngDelta = 360
	} else {
		lngDelta = radiusMeters / (111320.0 * cosLat)
	}
	minLng, maxLng := lng-lngDelta, lng+lngDelta
	minLat, maxLat := lat-latDelta, lat+latDelta

	// Collect R-tree candidates under RLock.
	candidates := make([]GeoResult, 0, 64)
	c.tree.Search(
		[2]float64{minLng, minLat},
		[2]float64{maxLng, maxLat},
		func(min, max [2]float64, docID string) bool {
			if allowedDocIDs != nil {
				if _, ok := allowedDocIDs[docID]; !ok {
					return true
				}
			}
			candidates = append(candidates, GeoResult{
				DocID:          docID,
				DistanceMeters: haversineMeters(lat, lng, min[1], min[0]),
			})
			return true
		},
	)
	gi.mu.RUnlock()

	// Exact haversine filter + sort happens lock-free on the snapshot.
	out := candidates[:0]
	for _, r := range candidates {
		if r.DistanceMeters <= radiusMeters {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceMeters < out[j].DistanceMeters })
	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

// Within returns all docs whose point is inside the closed bbox
// [minLat, maxLat] × [minLng, maxLng]. No ordering is applied.
func (gi *GeoIndex) Within(collection string, minLat, maxLat, minLng, maxLng float64, allowedDocIDs map[string]struct{}) []GeoResult {
	if !ValidLatLng(minLat, minLng) || !ValidLatLng(maxLat, maxLng) || minLat > maxLat || minLng > maxLng {
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
		func(_, _ [2]float64, docID string) bool {
			if allowedDocIDs != nil {
				if _, ok := allowedDocIDs[docID]; !ok {
					return true
				}
			}
			out = append(out, GeoResult{DocID: docID})
			return true
		},
	)
	return out
}

// Len reports the number of indexed points in a collection.
func (gi *GeoIndex) Len(collection string) int {
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	c := gi.collections[collection]
	if c == nil {
		return 0
	}
	return c.tree.Len()
}

// Collections returns a sorted snapshot of known collection names.
func (gi *GeoIndex) Collections() []string {
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	names := make([]string, 0, len(gi.collections))
	for k := range gi.collections {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// LastRebuild returns when a collection was last rebuilt from BoltDB.
func (gi *GeoIndex) LastRebuild(collection string) time.Time {
	gi.mu.RLock()
	defer gi.mu.RUnlock()
	return gi.lastRebuild[collection]
}

// markRebuilt is called by GeoStore.Rebuild to stamp the rebuild timestamp.
func (gi *GeoIndex) markRebuilt(collection string) {
	gi.mu.Lock()
	defer gi.mu.Unlock()
	gi.lastRebuild[collection] = time.Now()
}

// Clear drops the index for a single collection (used by reindex).
func (gi *GeoIndex) Clear(collection string) {
	gi.mu.Lock()
	defer gi.mu.Unlock()
	delete(gi.collections, collection)
	delete(gi.lastRebuild, collection)
}

// extractLatLng reads geo_lat and geo_lng from document metadata.
// Returns (0, 0, false) if either is missing or unparseable.
func extractLatLng(meta map[string][]string) (float64, float64, bool) {
	latVals := meta[metaKeyGeoLat]
	lngVals := meta[metaKeyGeoLng]
	if len(latVals) == 0 || len(lngVals) == 0 {
		return 0, 0, false
	}
	lat, err := strconv.ParseFloat(latVals[0], 64)
	if err != nil {
		return 0, 0, false
	}
	lng, err := strconv.ParseFloat(lngVals[0], 64)
	if err != nil {
		return 0, 0, false
	}
	if !ValidLatLng(lat, lng) {
		return 0, 0, false
	}
	return lat, lng, true
}

// ValidLatLng rejects NaN, Inf, and out-of-range coordinates.
func ValidLatLng(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) || math.IsInf(lat, 0) || math.IsInf(lng, 0) {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// haversineMeters returns the great-circle distance between two points on
// the WGS84 sphere using the haversine formula. Accurate to better than 0.5%
// across the full range; matches what PostGIS ST_Distance_Sphere produces.
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const deg2rad = math.Pi / 180
	φ1 := lat1 * deg2rad
	φ2 := lat2 * deg2rad
	dφ := (lat2 - lat1) * deg2rad
	dλ := (lng2 - lng1) * deg2rad
	a := math.Sin(dφ/2)*math.Sin(dφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
