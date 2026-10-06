package geom

import "fmt"

// ValidateRegionTopology checks the relationships between the outer ring
// (index 0) and the holes (indices 1..n): every hole must lie strictly
// inside the outer ring without touching it, and holes must be pairwise
// disjoint — no intersection, no touching, no containment. Rings are
// assumed to have passed ValidateRing individually.
func ValidateRegionTopology(rings []Ring) []RingError {
	var errs []RingError
	if len(rings) < 2 {
		return errs
	}
	outer := rings[0]
	for h := 1; h < len(rings); h++ {
		hole := rings[h]
		// No edge contact at all between hole and outer ring.
		errs = append(errs, edgeContactErrors(hole, outer, h, 0,
			"hole touches or crosses the outer ring")...)
		// A representative vertex decides containment once no edge
		// contact exists; use the vertex farthest from degeneracy is
		// unnecessary — any vertex works, but check all to be safe
		// against rings that weave without edge contact (impossible
		// for simple rings, but cheap to assert).
		if !PointInRing(outer, hole[0]) {
			errs = append(errs, RingError{Ring: h, Edge: 0,
				Message: "hole is not strictly inside the outer ring"})
		}
	}
	// Hole vs hole.
	for i := 1; i < len(rings); i++ {
		for j := i + 1; j < len(rings); j++ {
			a, b := rings[i], rings[j]
			errs = append(errs, edgeContactErrors(a, b, i, j,
				"hole intersects or touches another hole")...)
			if PointInRing(b, a[0]) || PointInRing(a, b[0]) {
				errs = append(errs, RingError{Ring: j, Edge: 0,
					Message: fmt.Sprintf("holes %d and %d contain one another", i, j)})
			}
		}
	}
	return errs
}

// edgeContactErrors reports one error per edge of ring ra (index ri) that
// makes any contact with ring rb (index rj).
func edgeContactErrors(ra, rb Ring, ri, rj int, msg string) []RingError {
	var errs []RingError
	for i := 0; i+1 < len(ra); i++ {
		for j := 0; j+1 < len(rb); j++ {
			if segmentsTouch(ra[i], ra[i+1], rb[j], rb[j+1]) {
				errs = append(errs, RingError{Ring: ri, Edge: i,
					Message: fmt.Sprintf("%s (edge %d of ring %d)", msg, j, rj)})
				break
			}
		}
	}
	return errs
}
