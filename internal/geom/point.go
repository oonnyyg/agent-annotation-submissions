package geom

// Location classifies a point relative to a ring.
type Location int

const (
	Outside Location = iota
	Inside
	Boundary
)

// PointOnRingBoundary reports whether p lies exactly on any edge of r.
func PointOnRingBoundary(r Ring, p Point) bool {
	for i := 0; i+1 < len(r); i++ {
		if onSegment(r[i], r[i+1], p) {
			return true
		}
	}
	return false
}

// PointInRing reports whether p is strictly inside the ring (boundary
// excluded), using the exact even-odd ray casting rule on integer
// arithmetic. Works for any ring orientation.
func PointInRing(r Ring, p Point) bool {
	inside := false
	n := len(r)
	for i := 0; i+1 < n; i++ {
		a, b := r[i], r[i+1]
		// Consider edges straddling the horizontal ray y = p.Y, x > p.X.
		if (a.Y > p.Y) == (b.Y > p.Y) {
			continue
		}
		// X coordinate where the edge crosses y = p.Y, compared against
		// p.X using exact cross-multiplication. The crossing x is
		// a.X + (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y); denominators are nonzero
		// here and we keep the sign of (b.Y-a.Y) to compare safely.
		num := (p.Y - a.Y) * (b.X - a.X)
		den := b.Y - a.Y
		var crosses bool
		if den > 0 {
			crosses = num > (p.X-a.X)*den
		} else {
			crosses = num < (p.X-a.X)*den
		}
		if crosses {
			inside = !inside
		}
	}
	return inside
}

// LocatePoint classifies p against ring r.
func LocatePoint(r Ring, p Point) Location {
	if PointOnRingBoundary(r, p) {
		return Boundary
	}
	if PointInRing(r, p) {
		return Inside
	}
	return Outside
}
