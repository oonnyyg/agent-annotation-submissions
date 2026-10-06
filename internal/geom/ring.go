package geom

import "fmt"

// RingError identifies a single ring validity problem, located by ring index
// and (when applicable) the edge index within the ring. Edge i connects
// vertex i to vertex i+1 of the ring as supplied (including the closing
// vertex).
type RingError struct {
	Ring    int    `json:"ring"`
	Edge    int    `json:"edge"`
	Message string `json:"message"`
}

func (e RingError) Error() string {
	return fmt.Sprintf("ring %d edge %d: %s", e.Ring, e.Edge, e.Message)
}

// edges returns the ring's edges as index pairs (i, i+1).
func edgeCount(r Ring) int {
	if len(r) == 0 {
		return 0
	}
	return len(r) - 1
}

// ValidateRing checks a single ring in isolation: explicit closure, at
// least three distinct vertices, no zero-length edges, no zero-area
// (degenerate) shape, no self-intersection, and adjacent edges sharing only
// their common endpoint. The ringIndex is used only for error reporting.
func ValidateRing(r Ring, ringIndex int) []RingError {
	var errs []RingError
	add := func(edge int, msg string) {
		errs = append(errs, RingError{Ring: ringIndex, Edge: edge, Message: msg})
	}

	if len(r) < 4 {
		add(0, "ring must have at least 3 distinct vertices and be explicitly closed")
		return errs
	}
	if r[0] != r[len(r)-1] {
		add(edgeCount(r)-1, "ring is not explicitly closed: first and last points differ")
		return errs
	}

	n := edgeCount(r)

	// Zero-length edges and distinct-vertex count.
	distinct := map[Point]bool{}
	zeroLen := false
	for i := 0; i < n; i++ {
		distinct[r[i]] = true
		if r[i] == r[i+1] {
			add(i, "zero-length edge: consecutive duplicate vertices")
			zeroLen = true
		}
	}
	if len(distinct) < 3 {
		add(0, "ring has fewer than 3 distinct vertices")
	}
	if zeroLen {
		// Further intersection checks assume non-degenerate edges.
		return errs
	}

	// Zero signed area means the ring degenerates to zero area.
	if SignedDoubleArea(r) == 0 {
		add(0, "ring has zero area")
	}

	// Pairwise edge checks.
	for i := 0; i < n; i++ {
		a, b := r[i], r[i+1]
		for j := i + 1; j < n; j++ {
			c, d := r[j], r[j+1]
			adjacent := j == i+1 || (i == 0 && j == n-1)
			if adjacent {
				// Adjacent edges share exactly one endpoint by
				// construction; any positive-length collinear
				// overlap (including folding back) is illegal.
				if segmentsProperlyOverlap(a, b, c, d) {
					add(i, fmt.Sprintf("adjacent edges %d and %d share more than one point", i, j))
				}
				continue
			}
			if segmentsTouch(a, b, c, d) {
				add(i, fmt.Sprintf("non-adjacent edges %d and %d intersect or touch", i, j))
			}
		}
	}
	return errs
}
