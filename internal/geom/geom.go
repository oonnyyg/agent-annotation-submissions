// Package geom provides exact-integer planar geometry helpers used by the
// region audit tool. Coordinates are plain integer plane coordinates, not
// lat/lon.
package geom

import (
	"encoding/json"
	"fmt"
)

// Point is an integer plane coordinate.
type Point struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

// UnmarshalJSON accepts either [x, y] arrays or {"x":..,"y":..} objects.
func (p *Point) UnmarshalJSON(data []byte) error {
	var arr [2]int64
	if err := json.Unmarshal(data, &arr); err == nil {
		p.X, p.Y = arr[0], arr[1]
		return nil
	}
	var obj struct {
		X int64 `json:"x"`
		Y int64 `json:"y"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("point must be [x, y] or {\"x\":..,\"y\":..}")
	}
	p.X, p.Y = obj.X, obj.Y
	return nil
}

// Ring is a closed polygon ring. The first and last points must be equal
// (explicitly closed). The closing duplicate is stored as part of the slice.
type Ring []Point

// cross returns the 2D cross product (b-a) x (c-a). Exact in int64 for
// coordinates up to 1e6 in magnitude.
func cross(a, b, c Point) int64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// onSegment reports whether p lies on segment ab (inclusive), using exact
// integer arithmetic.
func onSegment(a, b, p Point) bool {
	if cross(a, b, p) != 0 {
		return false
	}
	return min(a.X, b.X) <= p.X && p.X <= max(a.X, b.X) &&
		min(a.Y, b.Y) <= p.Y && p.Y <= max(a.Y, b.Y)
}

// segmentsTouch reports whether segments ab and cd share any point at all
// (crossing, endpoint touching, or collinear overlap).
func segmentsTouch(a, b, c, d Point) bool {
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

// segmentsProperlyOverlap reports whether collinear segments ab and cd
// share more than a single point (i.e. overlap with positive length).
func segmentsProperlyOverlap(a, b, c, d Point) bool {
	if cross(a, b, c) != 0 || cross(a, b, d) != 0 {
		return false
	}
	pts := map[Point]bool{}
	for _, p := range []Point{a, b} {
		if onSegment(c, d, p) {
			pts[p] = true
		}
	}
	for _, p := range []Point{c, d} {
		if onSegment(a, b, p) {
			pts[p] = true
		}
	}
	return len(pts) >= 2
}

// SignedDoubleArea returns twice the signed area of the ring. Positive for
// counter-clockwise orientation. Exact integer arithmetic.
func SignedDoubleArea(r Ring) int64 {
	var sum int64
	for i := 0; i+1 < len(r); i++ {
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

// NetDoubleArea returns twice the ring area as a positive value,
// independent of orientation.
func NetDoubleArea(r Ring) int64 {
	return abs64(SignedDoubleArea(r))
}
