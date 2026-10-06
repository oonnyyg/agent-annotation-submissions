
package main

import "sort"

// Change is a minimal business span where ownership differs between two
// knowledge cutoffs. Empty Before/After means no recorded ownership.
type Change struct {
	From   Date
	To     Date
	Before string
	After  string
}

// Diff compares the full business timeline of device as known at cutoffA
// vs cutoffB and returns the minimal merged segments where the owner
// actually changed (including to/from "no record").
func (r *Register) Diff(device string, cutoffA, cutoffB int64) []Change {
	a := r.Timeline(device, cutoffA)
	b := r.Timeline(device, cutoffB)
	bounds := map[Date]bool{}
	for _, s := range a {
		bounds[s.From] = true
		bounds[s.To] = true
	}
	for _, s := range b {
		bounds[s.From] = true
		bounds[s.To] = true
	}
	pts := make([]Date, 0, len(bounds))
	for p := range bounds {
		pts = append(pts, p)
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Before(pts[j]) })
	ownerAt := func(tl []Segment, d Date) string {
		for _, s := range tl {
			if !d.Before(s.From) && d.Before(s.To) {
				return s.Unit
			}
		}
		return ""
	}
	var out []Change
	for i := 0; i+1 < len(pts); i++ {
		from, to := pts[i], pts[i+1]
		before, after := ownerAt(a, from), ownerAt(b, from)
		if before == after {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Before == before && out[n-1].After == after && out[n-1].To.Equal(from) {
			out[n-1].To = to
			continue
		}
		out = append(out, Change{From: from, To: to, Before: before, After: after})
	}
	return out
}
