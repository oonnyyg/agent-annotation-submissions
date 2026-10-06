package geom

import "testing"

func square(x0, y0, x1, y1 int64) Ring {
	return Ring{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}, {x0, y0}}
}

func reversed(r Ring) Ring {
	out := make(Ring, len(r))
	for i, p := range r {
		out[len(r)-1-i] = p
	}
	return out
}

func TestValidateRingValid(t *testing.T) {
	// Concave outer ring (L-shape).
	concave := Ring{{0, 0}, {4, 0}, {4, 2}, {2, 2}, {2, 4}, {0, 4}, {0, 0}}
	if errs := ValidateRing(concave, 0); len(errs) != 0 {
		t.Fatalf("concave ring rejected: %v", errs)
	}
	if errs := ValidateRing(reversed(concave), 0); len(errs) != 0 {
		t.Fatalf("reversed concave ring rejected: %v", errs)
	}
}

func TestValidateRingRejects(t *testing.T) {
	cases := map[string]Ring{
		"not closed":       {{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		"too few points":   {{0, 0}, {1, 1}, {0, 0}},
		"zero length edge": {{0, 0}, {2, 0}, {2, 0}, {0, 2}, {0, 0}},
		"zero area":        {{0, 0}, {2, 0}, {4, 0}, {0, 0}},
		"self crossing":    {{0, 0}, {4, 4}, {4, 0}, {0, 4}, {0, 0}},
		// Non-adjacent edges touch at a point (figure-eight on a vertex).
		"vertex touch": {{0, 0}, {4, 0}, {4, 4}, {0, 0}, {-4, 4}, {-4, 0}, {0, 0}},
		// Adjacent edges fold back collinearly.
		"fold back": {{0, 0}, {4, 0}, {2, 0}, {0, 4}, {0, 0}},
	}
	for name, r := range cases {
		if errs := ValidateRing(r, 0); len(errs) == 0 {
			t.Errorf("%s: expected errors, got none", name)
		}
	}
}

func TestTopology(t *testing.T) {
	outer := square(0, 0, 10, 10)
	goodHole := square(2, 2, 4, 4)
	if errs := ValidateRegionTopology([]Ring{outer, goodHole}); len(errs) != 0 {
		t.Fatalf("valid hole rejected: %v", errs)
	}
	// Hole tangent to the outer ring.
	tangentOuter := square(2, 2, 10, 6)
	if errs := ValidateRegionTopology([]Ring{outer, tangentOuter}); len(errs) == 0 {
		t.Fatal("tangent-to-outer hole accepted")
	}
	// Two holes tangent to each other.
	h1 := square(2, 2, 5, 5)
	h2 := square(5, 2, 8, 5)
	if errs := ValidateRegionTopology([]Ring{outer, h1, h2}); len(errs) == 0 {
		t.Fatal("mutually tangent holes accepted")
	}
	// Nested holes (containment without touching).
	big := square(1, 1, 9, 9)
	small := square(3, 3, 6, 6)
	if errs := ValidateRegionTopology([]Ring{outer, big, small}); len(errs) == 0 {
		t.Fatal("nested holes accepted")
	}
	// Hole outside the outer ring.
	outside := square(20, 20, 30, 30)
	if errs := ValidateRegionTopology([]Ring{outer, outside}); len(errs) == 0 {
		t.Fatal("hole outside outer ring accepted")
	}
}

func TestMeasureOrientationInvariant(t *testing.T) {
	outer := Ring{{0, 0}, {10, 0}, {10, 6}, {6, 6}, {6, 10}, {0, 10}, {0, 0}}
	hole := square(1, 1, 3, 3)
	m1 := Measure([]Ring{outer, hole})
	m2 := Measure([]Ring{reversed(outer), reversed(hole)})
	if m1 != m2 {
		t.Fatalf("orientation changed result: %+v vs %+v", m1, m2)
	}
	wantArea := float64(10*10-4*4) - 4 // outer 84 minus hole 4
	if m1.Area != wantArea {
		t.Fatalf("area = %v, want %v", m1.Area, wantArea)
	}
	wantPerim := float64(10+6+4+4+6+10) + 8
	if m1.Perimeter != wantPerim {
		t.Fatalf("perimeter = %v, want %v", m1.Perimeter, wantPerim)
	}
}

func TestLocatePoint(t *testing.T) {
	outer := square(0, 0, 10, 10)
	if got := LocatePoint(outer, Point{5, 5}); got != Inside {
		t.Fatalf("inside point: %v", got)
	}
	if got := LocatePoint(outer, Point{0, 5}); got != Boundary {
		t.Fatalf("boundary point: %v", got)
	}
	if got := LocatePoint(outer, Point{0, 0}); got != Boundary {
		t.Fatalf("vertex point: %v", got)
	}
	if got := LocatePoint(outer, Point{20, 5}); got != Outside {
		t.Fatalf("outside point: %v", got)
	}
}
