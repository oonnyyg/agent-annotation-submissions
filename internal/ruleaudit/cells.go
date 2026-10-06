package ruleaudit

import (
	"fmt"
	"sort"
	"strings"
)

// Segment is one atomic partition piece of a field domain: every rule
// condition either fully contains or fully excludes it.
type Segment struct {
	EnumValues []string // enum: the equivalent values (declaration order)
	Lo, Hi     int      // int: inclusive bounds
}

// Label renders the segment boundary for reports.
func (s Segment) Label(f *Field) string {
	if f.Type == KindEnum {
		return strings.Join(s.EnumValues, "|")
	}
	if s.Lo == s.Hi {
		return fmt.Sprintf("%d", s.Lo)
	}
	return fmt.Sprintf("%d..%d", s.Lo, s.Hi)
}

// fieldPartition holds the atomic segments of one field, ordered by the
// canonical witness order (enum: declaration order; int: numeric order).
type fieldPartition struct {
	field    *Field
	segments []Segment
}

// buildPartitions splits every field domain into atomic segments using all
// rule condition boundaries.
func buildPartitions(spec *Spec) []fieldPartition {
	parts := make([]fieldPartition, len(spec.Fields))
	for i := range spec.Fields {
		f := &spec.Fields[i]
		if f.Type == KindEnum {
			parts[i] = fieldPartition{field: f, segments: enumSegments(spec, f)}
		} else {
			parts[i] = fieldPartition{field: f, segments: intSegments(spec, f)}
		}
	}
	return parts
}

// enumSegments groups enum values that belong to exactly the same set of
// rule conditions. Groups are ordered by their first declared value.
func enumSegments(spec *Spec, f *Field) []Segment {
	type group struct {
		firstIdx int
		values   []string
	}
	groups := map[string]*group{}
	var order []*group
	for vi, v := range f.Values {
		var sig strings.Builder
		for ri := range spec.Rules {
			if enumMatches(&spec.Rules[ri], f.Name, v) {
				fmt.Fprintf(&sig, "1")
			} else {
				fmt.Fprintf(&sig, "0")
			}
		}
		key := sig.String()
		g, ok := groups[key]
		if !ok {
			g = &group{firstIdx: vi}
			groups[key] = g
			order = append(order, g)
		}
		g.values = append(g.values, v)
	}
	segs := make([]Segment, len(order))
	for i, g := range order {
		segs[i] = Segment{EnumValues: g.values}
	}
	return segs
}

// intSegments splits the int domain at every rule condition boundary.
func intSegments(spec *Spec, f *Field) []Segment {
	cuts := map[int]bool{*f.Min: true, *f.Max + 1: true}
	for ri := range spec.Rules {
		cond, ok := spec.Rules[ri].When[f.Name]
		if !ok {
			continue
		}
		lo, hi, err := f.NormalizeRange(cond.Range)
		if err != nil || lo > hi {
			continue // invalid rules are rejected during validation
		}
		cuts[lo] = true
		if hi+1 <= *f.Max {
			cuts[hi+1] = true
		}
	}
	bounds := make([]int, 0, len(cuts))
	for b := range cuts {
		bounds = append(bounds, b)
	}
	sort.Ints(bounds)
	var segs []Segment
	for i := 0; i+1 < len(bounds); i++ {
		segs = append(segs, Segment{Lo: bounds[i], Hi: bounds[i+1] - 1})
	}
	return segs
}

// enumMatches reports whether rule's condition on field fname (if any)
// accepts value v.
func enumMatches(rule *Rule, fname, v string) bool {
	cond, ok := rule.When[fname]
	if !ok {
		return true
	}
	for _, x := range cond.Values {
		if x == v {
			return true
		}
	}
	return false
}

// intMatches reports whether rule's condition on field fname (if any)
// accepts value v.
func intMatches(rule *Rule, f *Field, v int) bool {
	cond, ok := rule.When[f.Name]
	if !ok {
		return true
	}
	lo, hi, err := f.NormalizeRange(cond.Range)
	if err != nil {
		return false
	}
	return v >= lo && v <= hi
}
