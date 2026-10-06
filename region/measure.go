package region

import "math"

// signedDoubleArea returns twice the signed area of a closed ring. The
// sign depends on winding; magnitudes are exact for integer coordinates.
func signedDoubleArea(r Ring) int64 {
	var sum int64
	for i := 0; i < len(r)-1; i++ {
		sum += r[i].X*r[i+1].Y - r[i+1].X*r[i].Y
	}
	return sum
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ringCentroid returns the area centroid of a single ring. The result is
// independent of the ring's winding direction.
func ringCentroid(r Ring) (x, y float64) {
	var cx, cy float64
	for i := 0; i < len(r)-1; i++ {
		c := float64(r[i].X*r[i+1].Y - r[i+1].X*r[i].Y)
		cx += float64(r[i].X+r[i+1].X) * c
		cy += float64(r[i].Y+r[i+1].Y) * c
	}
	a2 := float64(signedDoubleArea(r))
	return cx / (3 * a2), cy / (3 * a2)
}

// Metrics holds the derived measurements of a valid region.
type Metrics struct {
	Area      float64    `json:"area"`
	Centroid  [2]float64 `json:"centroid"`
	Perimeter float64    `json:"perimeter"`
}

// Measure computes net area (outer minus holes), the area centroid of the
// net region, and the total boundary length including hole perimeters.
// Results are invariant under reversal of any ring. The region must be
// valid (see ValidateRegion).
func Measure(rings []Ring) Metrics {
	var m Metrics
	var wsum, cxsum, cysum float64
	for i, r := range rings {
		a2 := abs64(signedDoubleArea(r))
		area := float64(a2) / 2
		sign := 1.0
		if i > 0 {
			sign = -1.0 // holes subtract
		}
		cx, cy := ringCentroid(r)
		wsum += sign * area
		cxsum += sign * area * cx
		cysum += sign * area * cy
		for j := 0; j < len(r)-1; j++ {
			dx := float64(r[j+1].X - r[j].X)
			dy := float64(r[j+1].Y - r[j].Y)
			m.Perimeter += math.Hypot(dx, dy)
		}
	}
	m.Area = wsum
	if wsum != 0 {
		m.Centroid = [2]float64{cxsum / wsum, cysum / wsum}
	}
	return m
}

// Location is the classification of a query point against a region.
type Location string

const (
	Inside        Location = "inside"
	Outside       Location = "outside"
	OuterBoundary Location = "outer_boundary"
	HoleBoundary  Location = "hole_boundary"
)

// Locate classifies a point against a valid region. Points inside a hole
// are outside the region.
func Locate(p Point, rings []Ring) Location {
	switch pointInRing(p, rings[0]) {
	case 0:
		return OuterBoundary
	case -1:
		return Outside
	}
	for _, hole := range rings[1:] {
		switch pointInRing(p, hole) {
		case 0:
			return HoleBoundary
		case 1:
			return Outside
		}
	}
	return Inside
}
