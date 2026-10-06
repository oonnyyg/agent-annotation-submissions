package region

import (
	"math"
	"testing"
)

func pts(coords ...int64) Ring {
	r := make(Ring, 0, len(coords)/2)
	for i := 0; i+1 < len(coords); i += 2 {
		r = append(r, Point{coords[i], coords[i+1]})
	}
	return r
}

func reversed(r Ring) Ring {
	out := make(Ring, 0, len(r))
	out = append(out, r[0])
	for i := len(r) - 2; i >= 1; i-- {
		out = append(out, r[i])
	}
	out = append(out, r[0])
	return out
}

// concaveRegion is a valid L-shaped (concave) outer ring with one hole.
func concaveRegion() []Ring {
	return []Ring{
		pts(0, 0, 10, 0, 10, 10, 5, 10, 5, 5, 0, 5, 0, 0),
		pts(1, 1, 3, 1, 3, 3, 1, 3, 1, 1),
	}
}

func TestValidConcaveRegion(t *testing.T) {
	if issues := ValidateRegion(concaveRegion()); len(issues) != 0 {
		t.Fatalf("expected valid region, got %v", issues)
	}
}

func TestUnclosedRing(t *testing.T) {
	rings := []Ring{pts(0, 0, 4, 0, 4, 4, 0, 4)}
	issues := ValidateRegion(rings)
	if len(issues) == 0 {
		t.Fatal("expected closure issue")
	}
}

func TestZeroLengthEdge(t *testing.T) {
	rings := []Ring{pts(0, 0, 4, 0, 4, 0, 4, 4, 0, 0)}
	issues := ValidateRegion(rings)
	if len(issues) == 0 {
		t.Fatal("expected zero-length edge issue")
	}
	found := false
	for _, is := range issues {
		if is.Ring == 0 && is.Edge != nil && *is.Edge == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected issue at ring 0 edge 1, got %v", issues)
	}
}

func TestSelfIntersectingRing(t *testing.T) {
	// Bowtie.
	rings := []Ring{pts(0, 0, 4, 4, 4, 0, 0, 4, 0, 0)}
	if issues := ValidateRegion(rings); len(issues) == 0 {
		t.Fatal("expected self-intersection issue")
	}
}

func TestNonAdjacentTouch(t *testing.T) {
	// Ring where edge 3 endpoint touches edge 1's interior.
	rings := []Ring{pts(0, 0, 6, 0, 6, 4, 3, 0, 3, 4, 0, 4, 0, 0)}
	if issues := ValidateRegion(rings); len(issues) == 0 {
		t.Fatal("expected non-adjacent touch issue")
	}
}

func TestHoleTouchingOuter(t *testing.T) {
	rings := []Ring{
		pts(0, 0, 10, 0, 10, 10, 0, 10, 0, 0),
		pts(2, 0, 5, 0, 5, 3, 2, 3, 2, 0), // touches outer edge y=0
	}
	if issues := ValidateRegion(rings); len(issues) == 0 {
		t.Fatal("expected hole-touching-outer issue")
	}
}

func TestTangentHoles(t *testing.T) {
	rings := []Ring{
		pts(0, 0, 10, 0, 10, 10, 0, 10, 0, 0),
		pts(1, 1, 4, 1, 4, 4, 1, 4, 1, 1),
		pts(4, 1, 7, 1, 7, 4, 4, 4, 4, 1), // shares edge x=4 with hole 1
	}
	if issues := ValidateRegion(rings); len(issues) == 0 {
		t.Fatal("expected tangent-holes issue")
	}
}

func TestNestedHoles(t *testing.T) {
	rings := []Ring{
		pts(0, 0, 10, 0, 10, 10, 0, 10, 0, 0),
		pts(1, 1, 8, 1, 8, 8, 1, 8, 1, 1),
		pts(3, 3, 5, 3, 5, 5, 3, 5, 3, 3),
	}
	issues := ValidateRegion(rings)
	if len(issues) == 0 {
		t.Fatal("expected nested-hole issue")
	}
}

func TestHoleOutsideOuter(t *testing.T) {
	rings := []Ring{
		pts(0, 0, 10, 0, 10, 10, 0, 10, 0, 0),
		pts(20, 20, 25, 20, 25, 25, 20, 25, 20, 20),
	}
	if issues := ValidateRegion(rings); len(issues) == 0 {
		t.Fatal("expected hole-outside issue")
	}
}

func TestMeasureAndReversalInvariance(t *testing.T) {
	base := concaveRegion()
	m1 := Measure(base)
	rev := []Ring{reversed(base[0]), reversed(base[1])}
	if issues := ValidateRegion(rev); len(issues) != 0 {
		t.Fatalf("reversed region should be valid, got %v", issues)
	}
	m2 := Measure(rev)
	if m1.Area != m2.Area || m1.Perimeter != m2.Perimeter ||
		m1.Centroid != m2.Centroid {
		t.Fatalf("reversal changed metrics: %+v vs %+v", m1, m2)
	}
	// Outer area = 10*10 - 5*5 (L-shape) = 75; hole = 4; net = 71.
	if m1.Area != 71 {
		t.Fatalf("net area = %v, want 71", m1.Area)
	}
	wantPerim := 40.0 + 8.0 // outer L perimeter 40, hole perimeter 8
	if math.Abs(m1.Perimeter-wantPerim) > 1e-9 {
		t.Fatalf("perimeter = %v, want %v", m1.Perimeter, wantPerim)
	}
	// Centroid must lie inside the region (not in the hole, not outside).
	c := Point{int64(math.Round(m1.Centroid[0])), int64(math.Round(m1.Centroid[1]))}
	if loc := Locate(c, base); loc != Inside {
		t.Fatalf("rounded centroid %v classified %v", c, loc)
	}
}

func TestLocate(t *testing.T) {
	rings := concaveRegion()
	cases := []struct {
		p    Point
		want Location
	}{
		{Point{8, 8}, Inside},
		{Point{2, 2}, Outside},       // inside the hole
		{Point{20, 20}, Outside},     // outside entirely
		{Point{0, 2}, OuterBoundary}, // on outer edge x=0
		{Point{2, 1}, HoleBoundary},  // on hole edge y=1
		{Point{5, 7}, OuterBoundary}, // concave notch edge x=5
		{Point{7, 2}, Inside},
	}
	for _, tc := range cases {
		if got := Locate(tc.p, rings); got != tc.want {
			t.Errorf("Locate(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}
