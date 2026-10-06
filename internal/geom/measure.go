package geom

import "math"

// Measurements holds derived quantities for a valid region.
type Measurements struct {
	// Area is the net area: outer ring minus holes.
	Area float64 `json:"area"`
	// Centroid is the area-weighted centroid of the net region.
	Centroid [2]float64 `json:"centroid"`
	// Perimeter is the total boundary length, outer ring plus all holes.
	Perimeter float64 `json:"perimeter"`
}

// ringCentroidNumerators returns the centroid numerator sums (Cx*6A,
// Cy*6A) as floats and the signed double area as an exact integer.
// Centroid numerators use float64 because with coordinates up to 1e6 a
// single term can reach 4e18, near the int64 limit; the resulting
// centroid relative error stays below 1e-9, well inside the documented
// tolerance.
func ringCentroidNumerators(r Ring) (cxNum, cyNum float64, doubleArea int64) {
	for i := 0; i+1 < len(r); i++ {
		cross := r[i].X*r[i+1].Y - r[i+1].X*r[i].Y
		doubleArea += cross
		cxNum += float64(r[i].X+r[i+1].X) * float64(cross)
		cyNum += float64(r[i].Y+r[i+1].Y) * float64(cross)
	}
	return cxNum, cyNum, doubleArea
}

// Measure computes net area, area centroid, and total perimeter for a
// region whose rings have been validated. Ring orientation does not
// matter: outer contributions are taken as positive and holes as
// negative regardless of winding direction.
func Measure(rings []Ring) Measurements {
	var cxNum, cyNum float64
	var doubleArea int64
	for idx, r := range rings {
		cx, cy, da := ringCentroidNumerators(r)
		if idx == 0 {
			if da < 0 {
				cx, cy, da = -cx, -cy, -da
			}
		} else {
			if da > 0 {
				cx, cy, da = -cx, -cy, -da
			}
		}
		cxNum += cx
		cyNum += cy
		doubleArea += da
	}
	m := Measurements{
		Area:     float64(doubleArea) / 2,
		Centroid: [2]float64{cxNum / (3 * float64(doubleArea)), cyNum / (3 * float64(doubleArea))},
	}
	for _, r := range rings {
		for i := 0; i+1 < len(r); i++ {
			m.Perimeter += math.Hypot(float64(r[i+1].X-r[i].X), float64(r[i+1].Y-r[i].Y))
		}
	}
	return m
}
