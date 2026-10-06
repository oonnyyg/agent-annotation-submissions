package ruleaudit

import (
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func loadSpec(t *testing.T, src string) *Spec {
	t.Helper()
	spec, err := LoadSpec(strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	return spec
}

// TestJointShadowing: rule 2 is fully covered only by the union of
// rules 0 and 1; neither covers it alone.
func TestJointShadowing(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [{"name":"x","type":"int","min":0,"max":10}],
	  "rules": [
	    {"name":"lo","when":{"x":{"min":0,"max":5}},"output":"a"},
	    {"name":"hi","when":{"x":{"min":6,"max":10}},"output":"b"},
	    {"name":"all","when":{"x":{"min":0,"max":10}},"output":"c"},
	    {"name":"free","output":"d"}
	  ]
	}`)
	rep := Analyze(spec, StrategyFirst, 1000)
	if !rep.Complete {
		t.Fatal("expected complete analysis")
	}
	// Rule 2 (x in 0..10) is covered only jointly by rules 0 and 1.
	// Rule 3 (no condition) is covered by rules 0, 1, 2 together.
	if len(rep.Shadowed) != 2 {
		t.Fatalf("expected rules 2 and 3 shadowed, got %+v", rep.Shadowed)
	}
	if rep.Shadowed[0].Rule != 2 {
		t.Fatalf("expected rule 2 shadowed, got %+v", rep.Shadowed[0])
	}
	by := rep.Shadowed[0].CoveredBy
	if len(by) != 2 || by[0] != 0 || by[1] != 1 {
		t.Fatalf("expected joint coverage by rules 0 and 1, got %v", by)
	}
	// Rules 0 and 1 remain effective, each with a minimal witness.
	if len(rep.Witnesses) != 2 {
		t.Fatalf("expected 2 effective rules, got %+v", rep.Witnesses)
	}
	if rep.Witnesses[0].Rule != 0 || rep.Witnesses[0].Witness["x"] != 0 {
		t.Fatalf("bad witness for rule 0: %+v", rep.Witnesses[0])
	}
	if rep.Witnesses[1].Rule != 1 || rep.Witnesses[1].Witness["x"] != 6 {
		t.Fatalf("bad witness for rule 1: %+v", rep.Witnesses[1])
	}
}

// TestOpenClosedEndpoints checks that endpoint openness is honored when
// computing coverage and witnesses.
func TestOpenClosedEndpoints(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [{"name":"x","type":"int","min":0,"max":10}],
	  "rules": [
	    {"name":"open-lo","when":{"x":{"min":5,"minExclusive":true}},"output":"hi"},
	    {"name":"closed-lo","when":{"x":{"max":5,"maxExclusive":true}},"output":"lo"}
	  ]
	}`)
	rep := Analyze(spec, StrategyUnique, 1000)
	if !rep.Complete {
		t.Fatal("expected complete analysis")
	}
	// x=5 is excluded by both: (5,10] and [0,5) leave 5 uncovered.
	if len(rep.Uncovered) != 1 {
		t.Fatalf("expected exactly one uncovered cell, got %+v", rep.Uncovered)
	}
	if got := rep.Uncovered[0].Witness["x"]; got != 5 {
		t.Fatalf("expected uncovered witness x=5, got %v", got)
	}
	if len(rep.Overlaps) != 0 {
		t.Fatalf("expected no overlaps, got %+v", rep.Overlaps)
	}
}

// TestUniqueConflictSameOutput: unique strategy flags conflicts even when
// both rules produce the same output.
func TestUniqueConflictSameOutput(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [{"name":"t","type":"enum","values":["a","b"]}],
	  "rules": [
	    {"when":{"t":["a","b"]},"output":"same"},
	    {"when":{"t":["b"]},"output":"same"}
	  ]
	}`)
	rep := Analyze(spec, StrategyUnique, 1000)
	if len(rep.Overlaps) != 1 {
		t.Fatalf("expected one overlap, got %+v", rep.Overlaps)
	}
	if got := rep.Overlaps[0].Witness["t"]; got != "b" {
		t.Fatalf("expected overlap witness t=b, got %v", got)
	}
	res, err := Evaluate(spec, StrategyUnique, Assignment{"t": "b"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Matched) != 2 {
		t.Fatalf("expected conflict on eval, got %+v", res)
	}
}

// TestCellLimit: analysis must refuse to conclude when cells exceed limit.
func TestCellLimit(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [
	    {"name":"a","type":"int","min":0,"max":100},
	    {"name":"b","type":"int","min":0,"max":100}
	  ],
	  "rules": [{"when":{"a":{"min":50}},"output":1}]
	}`)
	rep := Analyze(spec, StrategyFirst, 1)
	if rep.Complete {
		t.Fatal("expected incomplete analysis")
	}
	if rep.TotalCells <= 1 {
		t.Fatalf("expected TotalCells > limit, got %d", rep.TotalCells)
	}
	if len(rep.Uncovered) != 0 || len(rep.Overlaps) != 0 {
		t.Fatal("incomplete analysis must not report conclusions")
	}
}
