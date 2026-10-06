package ruleaudit

import (
	"fmt"
	"strings"
)

// Strategy selects how multiple matching rules are resolved.
type Strategy string

const (
	// StrategyUnique requires exactly one matching rule per input.
	StrategyUnique Strategy = "unique"
	// StrategyFirst returns the earliest matching rule.
	StrategyFirst Strategy = "first"
)

// Assignment maps field names to a concrete value (string for enum,
// int for int fields).
type Assignment map[string]any

// Region is one equivalence cell reported in a diagnostic.
type Region struct {
	Segments []string // per-field "name=segment" labels, declaration order
	Rules    []int    // matching rule indices
	Witness  Assignment
}

// ShadowReport describes a rule fully shadowed by earlier rules
// (first-match strategy only).
type ShadowReport struct {
	Rule      int
	CoveredBy []int // earlier rules that jointly cover it
}

// RuleWitness records a minimal input on which a rule still takes effect.
type RuleWitness struct {
	Rule    int
	Witness Assignment
}

// Report is the result of a static analysis run.
type Report struct {
	Strategy   Strategy
	Complete   bool
	TotalCells int
	MaxCells   int
	Uncovered  []Region
	Overlaps   []Region // cells with >= 2 matching rules
	Shadowed   []ShadowReport
	Witnesses  []RuleWitness // for rules that can still fire
}

// Analyze performs an exact exhaustive analysis over equivalence cells.
// If the cell count exceeds maxCells, it returns an incomplete report and
// draws no conclusions.
func Analyze(spec *Spec, strategy Strategy, maxCells int) *Report {
	rep := &Report{Strategy: strategy, MaxCells: maxCells}
	parts := buildPartitions(spec)
	total := 1
	for _, p := range parts {
		total *= len(p.segments)
	}
	rep.TotalCells = total
	if maxCells > 0 && total > maxCells {
		return rep // Complete stays false
	}
	rep.Complete = true

	matched := make([]bool, len(spec.Rules))   // rule matches some cell
	effective := make([]bool, len(spec.Rules)) // rule is the outcome of some cell
	shadowCovered := make([]bool, len(spec.Rules))
	shadowBy := make([]map[int]bool, len(spec.Rules))

	idx := make([]int, len(parts))
	for {
		matches := matchCell(spec, parts, idx)
		for _, m := range matches {
			matched[m] = true
		}
		switch {
		case len(matches) == 0:
			rep.Uncovered = append(rep.Uncovered, makeRegion(spec, parts, idx, matches))
		case len(matches) >= 2:
			rep.Overlaps = append(rep.Overlaps, makeRegion(spec, parts, idx, matches))
		}
		if len(matches) > 0 {
			winner := matches[0]
			if strategy == StrategyUnique && len(matches) != 1 {
				winner = -1
			}
			if winner >= 0 {
				if !effective[winner] {
					effective[winner] = true
					rep.Witnesses = append(rep.Witnesses, RuleWitness{
						Rule:    winner,
						Witness: witnessOf(parts, idx),
					})
				}
			}
			// Shadow bookkeeping: a rule is shadowed on this cell if an
			// earlier rule also matches.
			for pos, m := range matches {
				if pos > 0 {
					shadowCovered[m] = true
					if shadowBy[m] == nil {
						shadowBy[m] = map[int]bool{}
					}
					for _, e := range matches[:pos] {
						shadowBy[m][e] = true
					}
				}
			}
		}
		if !nextCell(parts, idx) {
			break
		}
	}

	if strategy == StrategyFirst {
		for ri := range spec.Rules {
			if matched[ri] && !effective[ri] {
				by := make([]int, 0, len(shadowBy[ri]))
				for e := range shadowBy[ri] {
					by = append(by, e)
				}
				sortInts(by)
				rep.Shadowed = append(rep.Shadowed, ShadowReport{Rule: ri, CoveredBy: by})
			}
		}
	}
	return rep
}

// matchCell returns the indices of all rules matching the cell, in rule
// order. Cells are atomic, so testing the representative point suffices.
func matchCell(spec *Spec, parts []fieldPartition, idx []int) []int {
	var out []int
	for ri := range spec.Rules {
		if ruleMatchesCell(spec, parts, idx, ri) {
			out = append(out, ri)
		}
	}
	return out
}

func ruleMatchesCell(spec *Spec, parts []fieldPartition, idx []int, ri int) bool {
	rule := &spec.Rules[ri]
	for fi := range parts {
		p := &parts[fi]
		seg := p.segments[idx[fi]]
		if p.field.Type == KindEnum {
			if !enumMatches(rule, p.field.Name, seg.EnumValues[0]) {
				return false
			}
		} else {
			if !intMatches(rule, p.field, seg.Lo) {
				return false
			}
		}
	}
	return true
}

// nextCell advances idx in lexicographic order over field partitions;
// the first field is the most significant, so the first cell found for a
// criterion yields the minimal witness in field declaration order.
func nextCell(parts []fieldPartition, idx []int) bool {
	for fi := len(parts) - 1; fi >= 0; fi-- {
		if idx[fi]+1 < len(parts[fi].segments) {
			idx[fi]++
			for j := fi + 1; j < len(parts); j++ {
				idx[j] = 0
			}
			return true
		}
	}
	return false
}

func makeRegion(spec *Spec, parts []fieldPartition, idx []int, matches []int) Region {
	segs := make([]string, len(parts))
	for fi := range parts {
		segs[fi] = parts[fi].field.Name + "=" + parts[fi].segments[idx[fi]].Label(parts[fi].field)
	}
	return Region{Segments: segs, Rules: matches, Witness: witnessOf(parts, idx)}
}

// witnessOf returns the minimal concrete input inside the cell: enum uses
// declaration order, int uses the numeric lower bound.
func witnessOf(parts []fieldPartition, idx []int) Assignment {
	w := Assignment{}
	for fi := range parts {
		p := &parts[fi]
		seg := p.segments[idx[fi]]
		if p.field.Type == KindEnum {
			w[p.field.Name] = seg.EnumValues[0]
		} else {
			w[p.field.Name] = seg.Lo
		}
	}
	return w
}

func sortInts(xs []int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// FormatAssignment renders an assignment deterministically in field
// declaration order.
func FormatAssignment(spec *Spec, a Assignment) string {
	parts := make([]string, 0, len(spec.Fields))
	for i := range spec.Fields {
		name := spec.Fields[i].Name
		if v, ok := a[name]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", name, v))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
