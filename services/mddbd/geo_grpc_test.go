package main

import (
	"context"
	"testing"

	pb "mddb/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// enableGeoOnTestServer augments newTestGRPCServer's output with a
// ready GeoStore + GeoIndex + GeoHashIndex. newTestGRPCServer itself
// is shared with many existing tests, so we don't touch it.
func enableGeoOnTestServer(t *testing.T, s *Server) {
	t.Helper()
	s.GeoStore = NewGeoStore(s.DB)
	if err := s.GeoStore.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	s.GeoIndex = NewGeoIndex()
	s.GeoIndex.SetReady()
	s.GeoHashIndex = NewGeoHashIndex()
	s.GeoHashIndex.SetReady()
}

func TestGRPCGeoSearch_MissingCollection(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	_, err := gs.GeoSearch(context.Background(), &pb.GeoSearchRequest{
		Lat:          52,
		Lng:          13,
		RadiusMeters: 1000,
	})
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoSearch_InvalidRadius(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	_, err := gs.GeoSearch(context.Background(), &pb.GeoSearchRequest{
		Collection:   "v",
		Lat:          52,
		Lng:          13,
		RadiusMeters: 0,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoSearch_InvalidLatLng(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	_, err := gs.GeoSearch(context.Background(), &pb.GeoSearchRequest{
		Collection:   "v",
		Lat:          999,
		Lng:          0,
		RadiusMeters: 1000,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoSearch_Empty(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	resp, err := gs.GeoSearch(context.Background(), &pb.GeoSearchRequest{
		Collection:   "v",
		Lat:          52,
		Lng:          13,
		RadiusMeters: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Algorithm != "rtree" {
		t.Errorf("algorithm=%q, want rtree", resp.Algorithm)
	}
	if resp.Total != 0 {
		t.Errorf("total=%d, want 0", resp.Total)
	}
}

func TestGRPCGeoSearch_IndexNotReady(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	s.GeoStore = NewGeoStore(s.DB)
	_ = s.GeoStore.EnsureBucket()
	s.GeoIndex = NewGeoIndex()
	// do not call SetReady()
	_, err := gs.GeoSearch(context.Background(), &pb.GeoSearchRequest{
		Collection:   "v",
		Lat:          0,
		Lng:          0,
		RadiusMeters: 1000,
	})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code=%v, want Unavailable", status.Code(err))
	}
}

func TestGRPCGeoWithin_InvalidBBox(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	_, err := gs.GeoWithin(context.Background(), &pb.GeoWithinRequest{
		Collection: "v",
		MinLat:     10,
		MaxLat:     5,
		MinLng:     0,
		MaxLng:     10,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoWithin_MissingCollection(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	_, err := gs.GeoWithin(context.Background(), &pb.GeoWithinRequest{
		MinLat: 0, MaxLat: 1, MinLng: 0, MaxLng: 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoWithin_Empty(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	resp, err := gs.GeoWithin(context.Background(), &pb.GeoWithinRequest{
		Collection: "v",
		MinLat:     0, MaxLat: 90, MinLng: 0, MaxLng: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 0 {
		t.Errorf("total=%d, want 0", resp.Total)
	}
}

func TestGRPCGeoStats_Empty(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	resp, err := gs.GeoStats(context.Background(), &pb.GeoStatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ready {
		t.Error("expected ready=true")
	}
}

func TestGRPCGeoReindex_EmptyBucket(t *testing.T) {
	gs, s, cleanup := newTestGRPCServer(t)
	defer cleanup()
	enableGeoOnTestServer(t, s)
	resp, err := gs.GeoReindex(context.Background(), &pb.GeoReindexRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Points != 0 {
		t.Errorf("points=%d, want 0", resp.Points)
	}
}

func TestGRPCGeoEncode_Valid(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	resp, err := gs.GeoEncode(context.Background(), &pb.GeoEncodeRequest{
		Lat: 52.52, Lng: 13.405, Precision: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Geohash) != 8 {
		t.Errorf("geohash length=%d, want 8", len(resp.Geohash))
	}
	if resp.Precision != 8 {
		t.Errorf("precision=%d, want 8", resp.Precision)
	}
}

func TestGRPCGeoEncode_InvalidLatLng(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	_, err := gs.GeoEncode(context.Background(), &pb.GeoEncodeRequest{
		Lat: 999, Lng: 0,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoEncode_DefaultPrecision(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	resp, err := gs.GeoEncode(context.Background(), &pb.GeoEncodeRequest{
		Lat: 52.52, Lng: 13.405,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Precision != geohashMaxPrecision {
		t.Errorf("default precision=%d, want %d", resp.Precision, geohashMaxPrecision)
	}
}

func TestGRPCGeoDecode_Valid(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	resp, err := gs.GeoDecode(context.Background(), &pb.GeoDecodeRequest{
		Geohash: "u33d8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MinLat == resp.MaxLat {
		t.Error("bbox should have extent")
	}
}

func TestGRPCGeoDecode_Missing(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	_, err := gs.GeoDecode(context.Background(), &pb.GeoDecodeRequest{Geohash: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPCGeoDecode_Invalid(t *testing.T) {
	gs, _, cleanup := newTestGRPCServer(t)
	defer cleanup()
	_, err := gs.GeoDecode(context.Background(), &pb.GeoDecodeRequest{Geohash: "!!!"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code=%v, want InvalidArgument", status.Code(err))
	}
}
