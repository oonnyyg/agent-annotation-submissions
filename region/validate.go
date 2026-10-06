package region

import "fmt"

// Issue locates a geometry problem within a region.
type Issue struct {
	Ring    int    `json:"ring"`           // ring index (0 = outer ring)
	Edge    *int   `json:"edge,omitempty"` // edge index within the ring, when applicable
	Message string `json:"message"`
}

func edgePtr(i int) *int { return &i }

// validateRing checks a single ring in isolation: explicit closure, at
// least three distinct vertices, no zero-length edges, non-zero area, and
// no self-intersection (touches/overlaps of non-adjacent edges included).
func validateRing(r Ring, idx int) []Issue {
	var issues []Issue
	fail := func(edge *int, msg string) {
		issues = append(issues, Issue{Ring: idx, Edge: edge, Message: msg})
	}
	if len(r) < 4 {
		fail(nil, fmt.Sprintf("ring has %d points; need a closed ring with at least 3 distinct vertices", len(r)))
		return issues
	}
	if r[0] != r[len(r)-1] {
		fail(nil, "ring is not explicitly closed: first and last points differ")
	}
	n := len(r) - 1 // number of edges / vertices
	for i := 0; i < n; i++ {
		if r[i] == r[i+1] {
			fail(edgePtr(i), "zero-length edge")
		}
	}
	// Distinct vertex count.
	seen := make(map[Point]bool, n)
	for i := 0; i < n; i++ {
		seen[r[i]] = true
	}
	if len(seen) < 3 {
		fail(nil, "ring has fewer than 3 distinct vertices")
	}
	if signedDoubleArea(r) == 0 {
		fail(nil, "ring has zero area")
	}
	// Edge-pair intersection tests. Edges i and j are adjacent when they
	// share exactly one endpoint by construction (consecutive, wrapping).
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			adjacent := j == i+1 || (i == 0 && j == n-1)
			a, b, c, d := r[i], r[i+1], r[j], r[j+1]
			if adjacent {
				// Adjacent edges must share exactly one endpoint and
				// must not overlap (collinear) or touch elsewhere.
				if a == c || a == d || b == c || b == d {
					// The shared endpoint is expected (b==c, or
					// d==a for the wrap pair); any other equality
					// means a vertex is revisited.
					if !(b == c) && !(i == 0 && j == n-1 && d == a) {
						fail(edgePtr(j), "adjacent edges share more than one endpoint")
					}
				}
				if cross(a, b, c) == 0 && cross(a, b, d) == 0 {
					fail(edgePtr(j), "adjacent edges are collinear (overlap or spike)")
				}
				continue
			}
			if segIntersect(a, b, c, d) {
				fail(edgePtr(j), fmt.Sprintf("edge %d intersects or touches edge %d", j, i))
			}
		}
	}
	return issues
}

// ValidateRegion checks the outer ring and all holes of a region and
// returns every issue found. A nil/empty result means the region is valid.
func ValidateRegion(rings []Ring) []Issue {
	var issues []Issue
	if len(rings) == 0 {
		return []Issue{{Ring: 0, Message: "region has no rings"}}
	}
	for i, r := range rings {
		issues = append(issues, validateRing(r, i)...)
	}
	if len(issues) > 0 {
		return issues
	}
	outer := rings[0]
	// Holes must lie strictly inside the outer ring: every hole vertex
	// strictly inside, and no hole edge may touch or cross the outer ring.
	for h := 1; h < len(rings); h++ {
		hole := rings[h]
		for i := 0; i < len(hole)-1; i++ {
			if pointInRing(hole[i], outer) != 1 {
				issues = append(issues, Issue{Ring: h, Edge: edgePtr(i),
					Message: "hole vertex is not strictly inside the outer ring"})
				break
			}
		}
		issues = append(issues, ringPairTouch(outer, 0, hole, h)...)
	}
	// Holes must not intersect, touch, or contain one another.
	for a := 1; a < len(rings); a++ {
		for b := a + 1; b < len(rings); b++ {
			ha, hb := rings[a], rings[b]
			issues = append(issues, ringPairTouch(ha, a, hb, b)...)
			if pointInRing(ha[0], hb) == 1 {
				issues = append(issues, Issue{Ring: a,
					Message: fmt.Sprintf("hole %d is contained in hole %d", a, b)})
			}
			if pointInRing(hb[0], ha) == 1 {
				issues = append(issues, Issue{Ring: b,
					Message: fmt.Sprintf("hole %d is contained in hole %d", b, a)})
			}
		}
	}
	return issues
}

// ringPairTouch reports an issue for every edge pair of rings ra and rb
// that intersects or touches.
func ringPairTouch(ra Ring, ia int, rb Ring, ib int) []Issue {
	var issues []Issue
	for i := 0; i < len(ra)-1; i++ {
		for j := 0; j < len(rb)-1; j++ {
			if segIntersect(ra[i], ra[i+1], rb[j], rb[j+1]) {
				issues = append(issues, Issue{Ring: ib, Edge: edgePtr(j),
					Message: fmt.Sprintf("edge %d of ring %d intersects or touches edge %d of ring %d", j, ib, i, ia)})
			}
		}
	}
	return issues
}
