package main

import (
	"context"
	"testing"
)

// newGeoTestClient builds a DirectClient wired to a minimal Server that
// has both geo indexes ready. Reuses newTestServerForGeo from
// geo_handlers_test.go to avoid the larger gRPC setup.
func newGeoTestClient(t *testing.T) (*DirectClient, func()) {
	t.Helper()
	s, cleanup := newTestServerForGeo(t)
	return NewDirectClient(s), cleanup
}

func TestDirectClientGeoSearch_MissingCollection(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Lat: 52, Lng: 13, RadiusMeters: 1000,
	})
	if err == nil {
		t.Error("expected error for missing collection")
	}
}

func TestDirectClientGeoSearch_ZeroRadius(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Collection: "v", Lat: 52, Lng: 13, RadiusMeters: 0,
	})
	if err == nil {
		t.Error("expected error for zero radius")
	}
}

func TestDirectClientGeoSearch_InvalidLatLng(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Collection: "v", Lat: 999, Lng: 0, RadiusMeters: 1000,
	})
	if err == nil {
		t.Error("expected error for invalid lat/lng")
	}
}

func TestDirectClientGeoSearch_UnknownAlgorithm(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Collection: "v", Lat: 52, Lng: 13, RadiusMeters: 1000, Algorithm: "bogus",
	})
	if err == nil {
		t.Error("expected error for unknown algorithm")
	}
}

func TestDirectClientGeoSearch_EmptyIndex(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	resp, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Collection: "v", Lat: 52, Lng: 13, RadiusMeters: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 0 {
		t.Errorf("total=%d, want 0", resp.Total)
	}
	if resp.Algorithm != "rtree" {
		t.Errorf("algorithm=%q, want rtree", resp.Algorithm)
	}
}

func TestDirectClientGeoSearch_GeohashAlgorithm(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	resp, err := c.GeoSearch(context.Background(), &MCPGeoSearchRequest{
		Collection: "v", Lat: 52, Lng: 13, RadiusMeters: 1000, Algorithm: "geohash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Algorithm != "geohash" {
		t.Errorf("algorithm=%q, want geohash", resp.Algorithm)
	}
}

func TestDirectClientGeoWithin_InvalidBBox(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoWithin(context.Background(), &MCPGeoWithinRequest{
		Collection: "v",
		MinLat:     10, MaxLat: 5, MinLng: 0, MaxLng: 10,
	})
	if err == nil {
		t.Error("expected error for min>max")
	}
}

func TestDirectClientGeoWithin_MissingCollection(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	_, err := c.GeoWithin(context.Background(), &MCPGeoWithinRequest{
		MinLat: 0, MaxLat: 1, MinLng: 0, MaxLng: 1,
	})
	if err == nil {
		t.Error("expected error for missing collection")
	}
}

func TestDirectClientGeoWithin_Empty(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	resp, err := c.GeoWithin(context.Background(), &MCPGeoWithinRequest{
		Collection: "v", MinLat: 0, MaxLat: 90, MinLng: 0, MaxLng: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 0 {
		t.Errorf("total=%d, want 0", resp.Total)
	}
}

func TestDirectClientGeoStats_Empty(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	resp, err := c.GeoStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ready {
		t.Error("expected ready=true")
	}
	if len(resp.Collections) != 0 {
		t.Errorf("collections=%v, want empty", resp.Collections)
	}
}

func TestDirectClientGeoEncodeDecode(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	h, err := c.GeoEncode(context.Background(), 52.52, 13.405, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 8 {
		t.Errorf("geohash length=%d, want 8", len(h))
	}
	lat, lng, err := c.GeoDecode(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	if lat < 52.5 || lat > 52.6 || lng < 13.3 || lng > 13.5 {
		t.Errorf("decoded (%v, %v) not near Berlin", lat, lng)
	}
}

func TestDirectClientGeoEncode_DefaultPrecision(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	h, err := c.GeoEncode(context.Background(), 52.52, 13.405, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != geohashMaxPrecision {
		t.Errorf("length=%d, want %d", len(h), geohashMaxPrecision)
	}
}

func TestDirectClientGeoEncode_Invalid(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	if _, err := c.GeoEncode(context.Background(), 999, 0, 8); err == nil {
		t.Error("expected error for invalid lat")
	}
}

func TestDirectClientGeoDecode_Empty(t *testing.T) {
	c, cleanup := newGeoTestClient(t)
	defer cleanup()
	if _, _, err := c.GeoDecode(context.Background(), ""); err == nil {
		t.Error("expected error for empty hash")
	}
}
