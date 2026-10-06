package ruleaudit

import (
	"strings"
	"testing"
)

func expectErr(t *testing.T, src, wantSub string) {
	t.Helper()
	_, err := LoadSpec(strings.NewReader(src))
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("expected error containing %q, got %q", wantSub, err.Error())
	}
}

func TestValidationErrors(t *testing.T) {
	base := `"fields":[{"name":"t","type":"enum","values":["a","b"]},{"name":"x","type":"int","min":0,"max":10}]`
	expectErr(t, `{"fields":[`+`{"name":"t","type":"enum","values":["a"]}`+`],"rules":[{"when":{"t":["zzz"]}}]}`,
		`rule 0: field "t": unknown enum value "zzz"`)
	expectErr(t, `{`+base+`,"rules":[{"when":{"x":{"min":-3}}}]}`,
		`rule 0: field "x": min -3 outside domain`)
	expectErr(t, `{`+base+`,"rules":[{"when":{"x":{"min":5,"max":5,"maxExclusive":true}}}]}`,
		`rule 0: field "x": empty condition interval`)
	expectErr(t, `{`+base+`,"rules":[{"when":{"nope":["a"]}}]}`,
		`rule 0: condition on unknown field "nope"`)
	expectErr(t, `{`+base+`,"rules":[{"when":{"t":{"min":1}}}]}`,
		`rule 0: field "t" is enum, range condition not allowed`)
	// Error must be located to the correct rule index.
	expectErr(t, `{`+base+`,"rules":[{"when":{"t":["a"]}},{"when":{"x":{"max":99}}}]}`,
		`rule 1: field "x": max 99 outside domain`)
}

// TestEvalConsistency: for every cell witness produced by the analysis,
// sample evaluation must agree with the static outcome.
func TestEvalConsistency(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [
	    {"name":"t","type":"enum","values":["free","pro"]},
	    {"name":"x","type":"int","min":0,"max":10}
	  ],
	  "rules": [
	    {"name":"r0","when":{"t":["pro"],"x":{"min":3,"max":7}},"output":"mid"},
	    {"name":"r1","when":{"x":{"min":8}},"output":"high"},
	    {"name":"r2","when":{"t":["pro"]},"output":"pro"}
	  ]
	}`)
	for _, strat := range []Strategy{StrategyUnique, StrategyFirst} {
		rep := Analyze(spec, strat, 10000)
		if !rep.Complete {
			t.Fatal("expected complete analysis")
		}
		// Every overlap witness must evaluate to the same matched set.
		for _, ov := range rep.Overlaps {
			res, err := Evaluate(spec, strat, ov.Witness)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Matched) != len(ov.Rules) {
				t.Fatalf("%s: static overlap %v vs eval %v", strat, ov.Rules, res.Matched)
			}
		}
		// Every gap witness must evaluate to no match.
		for _, gap := range rep.Uncovered {
			res, err := Evaluate(spec, strat, gap.Witness)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Matched) != 0 {
				t.Fatalf("%s: static gap but eval matched %v", strat, res.Matched)
			}
		}
		// Every rule witness must make that rule the outcome.
		for _, rw := range rep.Witnesses {
			res, err := Evaluate(spec, strat, rw.Witness)
			if err != nil {
				t.Fatal(err)
			}
			if !res.OK || res.Rule != rw.Rule {
				t.Fatalf("%s: witness for rule %d gave %+v", strat, rw.Rule, res)
			}
		}
	}
}

// TestFirstStrategyEvalKeepsAllMatches: first-match returns the earliest
// rule but retains every matching rule.
func TestFirstStrategyEvalKeepsAllMatches(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [{"name":"x","type":"int","min":0,"max":10}],
	  "rules": [
	    {"name":"a","when":{"x":{"min":5}},"output":"A"},
	    {"name":"b","when":{"x":{"min":7}},"output":"B"}
	  ]
	}`)
	res, err := Evaluate(spec, StrategyFirst, Assignment{"x": 9})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Rule != 0 {
		t.Fatalf("expected rule 0 to win, got %+v", res)
	}
	if len(res.Matched) != 2 || res.Matched[0] != 0 || res.Matched[1] != 1 {
		t.Fatalf("expected matches [0 1], got %v", res.Matched)
	}
	if string(res.Output) != `"A"` {
		t.Fatalf("expected output A, got %s", res.Output)
	}
}

// TestWitnessOrder: counterexamples take the minimal input in field
// declaration order; enums use declaration order, ints numeric order.
func TestWitnessOrder(t *testing.T) {
	spec := loadSpec(t, `{
	  "fields": [
	    {"name":"t","type":"enum","values":["z","m","a"]},
	    {"name":"x","type":"int","min":0,"max":10}
	  ],
	  "rules": [
	    {"when":{"t":["m","a"],"x":{"min":2}},"output":1}
	  ]
	}`)
	rep := Analyze(spec, StrategyFirst, 1000)
	if len(rep.Uncovered) == 0 {
		t.Fatal("expected gaps")
	}
	// Minimal gap: first field takes its first declared value "z",
	// second field its numeric minimum 0.
	first := rep.Uncovered[0].Witness
	if first["t"] != "z" || first["x"] != 0 {
		t.Fatalf("expected minimal witness {t=z, x=0}, got %v", first)
	}
}
