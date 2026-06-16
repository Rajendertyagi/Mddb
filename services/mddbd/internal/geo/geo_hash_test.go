package geo

import (
	"math"
	"testing"
)

// Reference vectors from https://en.wikipedia.org/wiki/Geohash.
var geohashVectors = []struct {
	lat, lng  float64
	precision int
	hash      string
}{
	{57.64911, 10.40744, 12, "u4pruydqqvj8"},
	{42.605, -5.603, 5, "ezs42"},
	{0, 0, 5, "s0000"},
}

func TestGeohashEncodeKnown(t *testing.T) {
	for _, v := range geohashVectors {
		got := GeohashEncode(v.lat, v.lng, v.precision)
		if got != v.hash {
			t.Errorf("encode(%v, %v, %d) = %q, want %q", v.lat, v.lng, v.precision, got, v.hash)
		}
	}
}

func TestGeohashDecodeKnown(t *testing.T) {
	// Decoded point is the centroid of the cell, so the error envelope
	// is at most half the cell dimension. Use GeohashBBox to compute
	// the actual cell for each test vector rather than guessing.
	for _, v := range geohashVectors {
		lat, lng, err := GeohashDecode(v.hash)
		if err != nil {
			t.Fatalf("decode(%q) error: %v", v.hash, err)
		}
		minLat, maxLat, minLng, maxLng, err := GeohashBBox(v.hash)
		if err != nil {
			t.Fatal(err)
		}
		// Encoded point must lie inside its own cell.
		if v.lat < minLat || v.lat > maxLat || v.lng < minLng || v.lng > maxLng {
			t.Errorf("encoded point (%v, %v) not inside bbox of %q", v.lat, v.lng, v.hash)
		}
		// Decoded centroid must also lie inside the cell.
		if lat < minLat || lat > maxLat || lng < minLng || lng > maxLng {
			t.Errorf("decoded (%v, %v) outside bbox of %q [%v-%v, %v-%v]",
				lat, lng, v.hash, minLat, maxLat, minLng, maxLng)
		}
	}
}

func TestGeohashRoundTrip(t *testing.T) {
	// Encoding and decoding at max precision should round-trip within
	// the sub-mm cell size.
	points := [][2]float64{
		{52.52, 13.405},
		{-33.8688, 151.2093}, // Sydney
		{37.7749, -122.4194}, // SF
		{0, 0},
		{90, 180},
	}
	for _, p := range points {
		h := GeohashEncode(p[0], p[1], GeohashMaxPrecision)
		lat, lng, err := GeohashDecode(h)
		if err != nil {
			t.Fatalf("decode(%q) error: %v", h, err)
		}
		if math.Abs(lat-p[0]) > 1e-5 || math.Abs(lng-p[1]) > 1e-5 {
			t.Errorf("roundtrip (%v, %v) → %q → (%v, %v)", p[0], p[1], h, lat, lng)
		}
	}
}

func TestGeohashCaseInsensitive(t *testing.T) {
	lat, lng, err := GeohashDecode("U4PRUYDQQVJ8")
	if err != nil {
		t.Fatalf("uppercase should decode, got %v", err)
	}
	_ = lat
	_ = lng
}

func TestGeohashInvalidInputs(t *testing.T) {
	if h := GeohashEncode(math.NaN(), 0, 5); h != "" {
		t.Errorf("NaN lat should encode to empty, got %q", h)
	}
	if h := GeohashEncode(0, math.Inf(1), 5); h != "" {
		t.Errorf("Inf lng should encode to empty, got %q", h)
	}
	if _, _, err := GeohashDecode(""); err == nil {
		t.Error("empty hash should error")
	}
	if _, _, err := GeohashDecode("u4pr!ydqqvj8"); err == nil {
		t.Error("invalid character should error")
	}
}

func TestGeohashPrecisionClamp(t *testing.T) {
	// Precision <1 and >12 should be clamped.
	if h := GeohashEncode(0, 0, 0); len(h) != 1 {
		t.Errorf("precision 0 clamped to 1, got %d", len(h))
	}
	if h := GeohashEncode(0, 0, 99); len(h) != GeohashMaxPrecision {
		t.Errorf("precision 99 clamped to %d, got %d", GeohashMaxPrecision, len(h))
	}
}

func TestGeohashBBox(t *testing.T) {
	// The bbox of a length-5 geohash should contain the encoded point.
	lat, lng := 52.52, 13.405
	h := GeohashEncode(lat, lng, 5)
	minLat, maxLat, minLng, maxLng, err := GeohashBBox(h)
	if err != nil {
		t.Fatal(err)
	}
	if lat < minLat || lat > maxLat {
		t.Errorf("lat %v outside bbox [%v, %v]", lat, minLat, maxLat)
	}
	if lng < minLng || lng > maxLng {
		t.Errorf("lng %v outside bbox [%v, %v]", lng, minLng, maxLng)
	}
}

func TestExtractGeoHash(t *testing.T) {
	cases := []struct {
		name   string
		meta   map[string][]string
		wantOK bool
	}{
		{"valid", map[string][]string{"geo_hash": {"u33d8"}}, true},
		{"missing", map[string][]string{}, false},
		{"invalid", map[string][]string{"geo_hash": {"!!!"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, ok := extractGeoHash(c.meta)
			if ok != c.wantOK {
				t.Errorf("got %v, want %v", ok, c.wantOK)
			}
		})
	}
}

func TestGeoIndexAddFromMetaGeohashFallback(t *testing.T) {
	idx := NewGeoIndex()
	// No geo_lat/geo_lng, but geo_hash present.
	lat, lng, ok := idx.AddFromMeta("venues", "d1", map[string][]string{
		"geo_hash": {"u33d8"},
	})
	if !ok {
		t.Fatal("expected geohash to be extracted")
	}
	// Decoding u33d8 should be close to the Berlin area.
	if math.Abs(lat-52.5) > 1 || math.Abs(lng-13.4) > 1 {
		t.Errorf("unexpected decoded point (%v, %v)", lat, lng)
	}
	if idx.Len("venues") != 1 {
		t.Errorf("Len=%d, want 1", idx.Len("venues"))
	}
}
