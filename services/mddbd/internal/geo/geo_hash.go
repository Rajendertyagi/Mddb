package geo

// Pure-Go geohash encoder/decoder. Compatible with the canonical
// geohash.org alphabet and bit-interleaving scheme:
//
//   - 32-char base32 alphabet ("0123456789bcdefghjkmnpqrstuvwxyz")
//   - odd-indexed bits encode longitude (-180..+180), even-indexed bits latitude (-90..+90)
//   - each character is 5 bits; 12 chars ≈ 37 mm precision
//
// Kept in a separate file so the encoding utilities can be used
// independently of the R-tree index subsystem — e.g. directly from
// the /v1/geo-encode and /v1/geo-decode HTTP endpoints.

import (
	"errors"
	"strings"
)

const (
	geohashAlphabet     = "0123456789bcdefghjkmnpqrstuvwxyz"
	GeohashMaxPrecision = 12
)

// geohashAlphabetIndex maps each alphabet rune back to its 0..31 index.
// Filled in init() — the table is small and read-only after that.
var geohashAlphabetIndex [256]int8

func init() {
	for i := range geohashAlphabetIndex {
		geohashAlphabetIndex[i] = -1
	}
	for i := 0; i < len(geohashAlphabet); i++ {
		geohashAlphabetIndex[geohashAlphabet[i]] = int8(i)
	}
}

// GeohashEncode converts (lat, lng) to a geohash string of the requested
// precision. Invalid coordinates or out-of-range precision return "".
// Precision is clamped to [1, 12].
func GeohashEncode(lat, lng float64, precision int) string {
	if !ValidLatLng(lat, lng) {
		return ""
	}
	if precision < 1 {
		precision = 1
	}
	if precision > GeohashMaxPrecision {
		precision = GeohashMaxPrecision
	}
	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0

	var b strings.Builder
	b.Grow(precision)
	bit, ch := 0, 0
	even := true
	for b.Len() < precision {
		if even {
			mid := (lngMin + lngMax) / 2
			if lng >= mid {
				ch |= 1 << (4 - bit)
				lngMin = mid
			} else {
				lngMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if lat >= mid {
				ch |= 1 << (4 - bit)
				latMin = mid
			} else {
				latMax = mid
			}
		}
		even = !even
		bit++
		if bit == 5 {
			b.WriteByte(geohashAlphabet[ch])
			bit, ch = 0, 0
		}
	}
	return b.String()
}

// GeohashDecode converts a geohash string back to the center (lat, lng)
// of its cell. Returns an error on unknown characters. The returned
// point is the centroid of the cell, not any corner.
func GeohashDecode(hash string) (float64, float64, error) {
	if hash == "" {
		return 0, 0, errors.New("empty geohash")
	}
	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0
	even := true
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		// geohash is case-insensitive by convention.
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		idx := geohashAlphabetIndex[c]
		if idx < 0 {
			return 0, 0, errors.New("invalid geohash character: " + string(rune(c)))
		}
		for bit := 4; bit >= 0; bit-- {
			on := (idx>>bit)&1 == 1
			if even {
				mid := (lngMin + lngMax) / 2
				if on {
					lngMin = mid
				} else {
					lngMax = mid
				}
			} else {
				mid := (latMin + latMax) / 2
				if on {
					latMin = mid
				} else {
					latMax = mid
				}
			}
			even = !even
		}
	}
	return (latMin + latMax) / 2, (lngMin + lngMax) / 2, nil
}

// GeohashBBox returns the (minLat, maxLat, minLng, maxLng) bounding box
// covered by a geohash cell. Used by the geohash index for bbox queries
// and by `/v1/geo-decode?bbox=true` for introspection.
func GeohashBBox(hash string) (minLat, maxLat, minLng, maxLng float64, err error) {
	if hash == "" {
		return 0, 0, 0, 0, errors.New("empty geohash")
	}
	minLat, maxLat = -90, 90
	minLng, maxLng = -180, 180
	even := true
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		idx := geohashAlphabetIndex[c]
		if idx < 0 {
			return 0, 0, 0, 0, errors.New("invalid geohash character")
		}
		for bit := 4; bit >= 0; bit-- {
			on := (idx>>bit)&1 == 1
			if even {
				mid := (minLng + maxLng) / 2
				if on {
					minLng = mid
				} else {
					maxLng = mid
				}
			} else {
				mid := (minLat + maxLat) / 2
				if on {
					minLat = mid
				} else {
					maxLat = mid
				}
			}
			even = !even
		}
	}
	return
}

// metaKeyGeoHash is the reserved meta key for storing a geohash on a
// document. When present, AddFromMeta will decode it to (lat, lng)
// in case geo_lat/geo_lng are absent — giving users a third way to
// attach coordinates (explicit floats, geohash, or postcode fallback).
const metaKeyGeoHash = "geo_hash"

// extractGeoHash pulls geo_hash out of doc metadata and decodes it.
// Returns (lat, lng, true) on success or (0, 0, false) otherwise.
func extractGeoHash(meta map[string][]string) (float64, float64, bool) {
	vals := meta[metaKeyGeoHash]
	if len(vals) == 0 {
		return 0, 0, false
	}
	lat, lng, err := GeohashDecode(vals[0])
	if err != nil {
		return 0, 0, false
	}
	if !ValidLatLng(lat, lng) {
		return 0, 0, false
	}
	return lat, lng, true
}
