package scanlogic

import (
	"strings"
	"testing"
)

func mustFail(t *testing.T, src string) {
	t.Helper()
	if _, err := LoadProgram(strings.NewReader(src)); err == nil {
		t.Fatalf("expected program to be rejected, got nil error")
	}
}

func TestRejectIllegalPrograms(t *testing.T) {
	cases := []string{
		// undeclared contact
		`{"inputs":[],"coils":["A"],"statements":[{"coil":{"target":"A","condition":{"contact":"X"}}}]}`,
		// undeclared write target
		`{"inputs":[],"coils":["A"],"statements":[{"coil":{"target":"B","condition":{"contact":"A"}}}]}`,
		// two writers for the same coil
		`{"inputs":["I"],"coils":["A"],"statements":[
		  {"coil":{"target":"A","condition":{"contact":"I"}}},
		  {"coil":{"target":"A","condition":{"not":{"contact":"I"}}}}]}`,
		// zero preset timer
		`{"inputs":["I"],"timers":[{"name":"T","preset":0}],"statements":[
		  {"ton":{"timer":"T","condition":{"contact":"I"}}}]}`,
		// negative preset counter
		`{"inputs":["I"],"counters":[{"name":"C","preset":-1}],"statements":[
		  {"ctu":{"counter":"C","condition":{"contact":"I"},"reset":{"contact":"I"}}}]}`,
		// duplicate declaration
		`{"inputs":["A"],"coils":["A"],"statements":[{"coil":{"target":"A","condition":{"contact":"A"}}}]}`,
		// coil statement writing a timer
		`{"inputs":["I"],"timers":[{"name":"T","preset":5}],"statements":[
		  {"coil":{"target":"T","condition":{"contact":"I"}}}]}`,
		// statement with two kinds set
		`{"inputs":["I"],"coils":["A"],"timers":[{"name":"T","preset":5}],"statements":[
		  {"coil":{"target":"A","condition":{"contact":"I"}},"ton":{"timer":"T","condition":{"contact":"I"}}}]}`,
		// empty condition
		`{"inputs":["I"],"coils":["A"],"statements":[{"coil":{"target":"A","condition":{}}}]}`,
		// no statements
		`{"inputs":["I"],"coils":["A"],"statements":[]}`,
	}
	for i, src := range cases {
		if _, err := LoadProgram(strings.NewReader(src)); err == nil {
			t.Fatalf("case %d: expected rejection", i)
		}
	}
}

func TestRejectIllegalInputLog(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["A"],
	  "statements": [{"coil": {"target": "A", "condition": {"contact": "I1"}}}]
	}`)
	bad := []string{
		`{"scans":[{"duration_ms":0,"inputs":{"I1":true}}]}`,        // non-positive duration
		`{"scans":[{"duration_ms":-5,"inputs":{"I1":true}}]}`,       // negative duration
		`{"scans":[{"duration_ms":10,"inputs":{}}]}`,                // incomplete inputs
		`{"scans":[{"duration_ms":10,"inputs":{"I1":true,"X":1}}]}`, // undeclared input
	}
	for i, src := range bad {
		if _, err := LoadInputLog(strings.NewReader(src), p); err == nil {
			t.Fatalf("case %d: expected rejection", i)
		}
	}
	good := `{"scans":[{"duration_ms":10,"inputs":{"I1":true}}]}`
	if _, err := LoadInputLog(strings.NewReader(good), p); err != nil {
		t.Fatalf("valid log rejected: %v", err)
	}
}

func TestNestedConditions(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["A", "B", "C"],
	  "coils": ["Q"],
	  "statements": [
	    {"coil": {"target": "Q", "condition": {"or": [
	      {"and": [{"contact": "A"}, {"not": {"contact": "B"}}]},
	      {"contact": "C"}
	    ]}}}
	  ]
	}`)
	eng := NewEngine(p)
	check := func(a, b, c, want bool) {
		res := eng.RunScan(0, ScanInput{DurationMs: 1, Inputs: map[string]bool{"A": a, "B": b, "C": c}})
		if res.State.Coils["Q"] != want {
			t.Fatalf("A=%v B=%v C=%v: got %v want %v", a, b, c, res.State.Coils["Q"], want)
		}
	}
	check(true, false, false, true)
	check(true, true, false, false)
	check(false, false, true, true)
	check(false, false, false, false)
}
