// Package region implements exact-integer geometry helpers for auditing
// polygonal regions with holes. Coordinates are integer plane coordinates
// (not lat/lon); all predicates use exact int64 arithmetic.
package region

// Point is an integer plane coordinate.
type Point struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

// Ring is a closed polygon ring. The first and last points must coincide.
type Ring []Point

// cross returns (b-a) x (c-a) as an exact int64.
func cross(a, b, c Point) int64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// onSegment reports whether p lies on segment ab (inclusive).
func onSegment(a, b, p Point) bool {
	if cross(a, b, p) != 0 {
		return false
	}
	return p.X >= min(a.X, b.X) && p.X <= max(a.X, b.X) &&
		p.Y >= min(a.Y, b.Y) && p.Y <= max(a.Y, b.Y)
}

// segIntersect reports whether segments ab and cd have any point in common,
// including endpoint touches and collinear overlaps.
func segIntersect(a, b, c, d Point) bool {
	d1 := cross(c, d, a)
	d2 := cross(c, d, b)
	d3 := cross(a, b, c)
	d4 := cross(a, b, d)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	if d1 == 0 && onSegment(c, d, a) {
		return true
	}
	if d2 == 0 && onSegment(c, d, b) {
		return true
	}
	if d3 == 0 && onSegment(a, b, c) {
		return true
	}
	if d4 == 0 && onSegment(a, b, d) {
		return true
	}
	return false
}

// pointInRing classifies a point against a closed ring using exact integer
// ray casting. It returns 1 for strictly inside, 0 on the boundary, -1 outside.
func pointInRing(p Point, r Ring) int {
	n := len(r) - 1 // last point duplicates the first
	inside := false
	for i := 0; i < n; i++ {
		a, b := r[i], r[i+1]
		if onSegment(a, b, p) {
			return 0
		}
		if (a.Y > p.Y) != (b.Y > p.Y) {
			c := cross(a, b, p)
			if b.Y > a.Y {
				if c > 0 {
					inside = !inside
				}
			} else {
				if c < 0 {
					inside = !inside
				}
			}
		}
	}
	if inside {
		return 1
	}
	return -1
}
