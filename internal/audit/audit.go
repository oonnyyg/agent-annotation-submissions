// Package audit combines geometry validation, measurement, and point
// classification into the region audit report.
package audit

import (
	"example.com/polygon-region-audit/internal/geom"
)

// RegionInput is one region as supplied in the input document.
type RegionInput struct {
	ID    string      `json:"id"`
	Rings []geom.Ring `json:"rings"`
}

// PointInput is one query point as supplied in the input document.
type PointInput struct {
	ID string `json:"id"`
	X  int64  `json:"x"`
	Y  int64  `json:"y"`
}

// Input is the top-level input document.
type Input struct {
	Regions []RegionInput `json:"regions"`
	Points  []PointInput  `json:"points"`
}

// RegionReport is the per-region audit result.
type RegionReport struct {
	ID        string           `json:"id"`
	Valid     bool             `json:"valid"`
	Errors    []geom.RingError `json:"errors,omitempty"`
	Area      float64          `json:"area,omitempty"`
	Centroid  *[2]float64      `json:"centroid,omitempty"`
	Perimeter float64          `json:"perimeter,omitempty"`
}

// Point classification labels.
const (
	LocInside        = "inside"
	LocOutside       = "outside"
	LocOuterBoundary = "outer_boundary"
	LocHoleBoundary  = "hole_boundary"
)

// PointReport maps region IDs to the point's classification.
type PointReport struct {
	ID      string            `json:"id"`
	Results map[string]string `json:"results"`
}

// Output is the top-level output document.
type Output struct {
	Regions []RegionReport `json:"regions"`
	Points  []PointReport  `json:"points"`
}

// AuditRegion validates one region and, when valid, computes its
// measurements.
func AuditRegion(r RegionInput) RegionReport {
	rep := RegionReport{ID: r.ID}
	if len(r.Rings) == 0 {
		rep.Errors = append(rep.Errors, geom.RingError{Ring: 0, Edge: 0,
			Message: "region has no rings"})
		return rep
	}
	for i, ring := range r.Rings {
		rep.Errors = append(rep.Errors, geom.ValidateRing(ring, i)...)
	}
	if len(rep.Errors) == 0 {
		rep.Errors = append(rep.Errors, geom.ValidateRegionTopology(r.Rings)...)
	}
	if len(rep.Errors) > 0 {
		return rep
	}
	rep.Valid = true
	m := geom.Measure(r.Rings)
	rep.Area = m.Area
	rep.Centroid = &m.Centroid
	rep.Perimeter = m.Perimeter
	return rep
}

// ClassifyPoint locates p against one valid region.
func ClassifyPoint(r RegionInput, p geom.Point) string {
	if geom.PointOnRingBoundary(r.Rings[0], p) {
		return LocOuterBoundary
	}
	for _, hole := range r.Rings[1:] {
		if geom.PointOnRingBoundary(hole, p) {
			return LocHoleBoundary
		}
	}
	if !geom.PointInRing(r.Rings[0], p) {
		return LocOutside
	}
	for _, hole := range r.Rings[1:] {
		if geom.PointInRing(hole, p) {
			return LocOutside
		}
	}
	return LocInside
}

// Run audits all regions and classifies all points. Invalid regions are
// reported but excluded from point classification.
func Run(in Input) Output {
	out := Output{}
	valid := make([]RegionInput, 0, len(in.Regions))
	for _, r := range in.Regions {
		rep := AuditRegion(r)
		out.Regions = append(out.Regions, rep)
		if rep.Valid {
			valid = append(valid, r)
		}
	}
	for _, p := range in.Points {
		pr := PointReport{ID: p.ID, Results: map[string]string{}}
		pt := geom.Point{X: p.X, Y: p.Y}
		for _, r := range valid {
			pr.Results[r.ID] = ClassifyPoint(r, pt)
		}
		out.Points = append(out.Points, pr)
	}
	return out
}
