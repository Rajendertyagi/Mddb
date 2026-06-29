package geo

import "testing"

func TestPointInRing_Triangle(t *testing.T) {
	// Triangle in Berlin: corners around Brandenburg Gate, Potsdamer Platz,
	// Hauptbahnhof. Each as [lng, lat] (GeoJSON order).
	ring := [][]float64{
		{13.3777, 52.5163}, // Brandenburg Gate
		{13.3758, 52.5096}, // Potsdamer Platz
		{13.3692, 52.5250}, // Hauptbahnhof
		{13.3777, 52.5163}, // close
	}
	// Centroid-ish point that is obviously inside.
	if !pointInRing(52.5170, 13.3740, ring) {
		t.Error("expected point inside triangle")
	}
	// Far away point clearly outside.
	if pointInRing(48.8566, 2.3522, ring) {
		t.Error("Paris should be outside Berlin triangle")
	}
	// Just outside the bounding box but still within lat range.
	if pointInRing(52.5163, 13.50, ring) {
		t.Error("point east of ring should be outside")
	}
}
func TestPointInRing_DegenerateRejected(t *testing.T) {
	// Ring with only 2 points cannot enclose anything — must return false
	// rather than panic on the edge-wrapping index math.
	ring := [][]float64{{0, 0}, {1, 1}}
	if pointInRing(0.5, 0.5, ring) {
		t.Error("2-point ring cannot contain any point")
	}
}
func TestPointInPolygon_WithHole(t *testing.T) {
	// 10x10 square with a 4x4 hole centered inside.
	coords := [][][]float64{
		// outer
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		// hole
		{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}},
	}
	// Point inside outer but outside hole: matches.
	if !pointInPolygon(1, 1, coords) {
		t.Error("(1,1) should be inside the square, outside the hole")
	}
	// Point inside hole: rejected.
	if pointInPolygon(5, 5, coords) {
		t.Error("(5,5) is inside the hole — must not match")
	}
	// Point outside outer: rejected.
	if pointInPolygon(20, 20, coords) {
		t.Error("(20,20) is outside — must not match")
	}
}
func TestPointInMultiPolygon_Union(t *testing.T) {
	// Two disjoint 1x1 squares.
	mp := [][][][]float64{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}},
		{{{10, 10}, {11, 10}, {11, 11}, {10, 11}, {10, 10}}},
	}
	if !pointInMultiPolygon(0.5, 0.5, mp) {
		t.Error("(0.5,0.5) in first square")
	}
	if !pointInMultiPolygon(10.5, 10.5, mp) {
		t.Error("(10.5,10.5) in second square")
	}
	if pointInMultiPolygon(5, 5, mp) {
		t.Error("(5,5) is between the squares — no match")
	}
}
func TestPolygonBounds_OuterPlusHoles(t *testing.T) {
	coords := [][][]float64{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}},
	}
	minLat, maxLat, minLng, maxLng, ok := polygonBounds(coords)
	if !ok {
		t.Fatal("expected ok")
	}
	if minLat != 0 || maxLat != 10 || minLng != 0 || maxLng != 10 {
		t.Errorf("bbox mismatch: got lat[%v,%v] lng[%v,%v]", minLat, maxLat, minLng, maxLng)
	}
}
func TestPolygonBounds_Empty(t *testing.T) {
	if _, _, _, _, ok := polygonBounds(nil); ok {
		t.Error("empty coords → ok should be false")
	}
}
