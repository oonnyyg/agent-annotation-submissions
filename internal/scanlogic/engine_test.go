package scanlogic

import (
	"strings"
	"testing"
)

func loadProgram(t *testing.T, src string) *Program {
	t.Helper()
	p, err := LoadProgram(strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	return p
}

// Earlier statements write values that later statements see in the same
// scan; earlier statements still see the old value of states written by
// later statements (no parallel evaluation).
func TestSameScanOrdering(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["A", "B", "C"],
	  "statements": [
	    {"coil": {"target": "A", "condition": {"contact": "I1"}}},
	    {"coil": {"target": "B", "condition": {"contact": "A"}}},
	    {"coil": {"target": "C", "condition": {"contact": "B"}}}
	  ]
	}`)
	eng := NewEngine(p)
	res := eng.RunScan(0, ScanInput{DurationMs: 10, Inputs: map[string]bool{"I1": true}})
	// A becomes true this scan, B sees the new A immediately, but C
	// still reads B's old value (false) because B is written after C...
	// order is A,B,C so C reads B written earlier in the same scan.
	if !res.State.Coils["A"] || !res.State.Coils["B"] || !res.State.Coils["C"] {
		t.Fatalf("forward visibility failed: %+v", res.State.Coils)
	}

	// Reverse order: reader before writer must see the old value.
	p2 := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["A", "B"],
	  "statements": [
	    {"coil": {"target": "B", "condition": {"contact": "A"}}},
	    {"coil": {"target": "A", "condition": {"contact": "I1"}}}
	  ]
	}`)
	eng2 := NewEngine(p2)
	r0 := eng2.RunScan(0, ScanInput{DurationMs: 10, Inputs: map[string]bool{"I1": true}})
	if r0.State.Coils["B"] {
		t.Fatalf("B must read A's old value in scan 0, got true")
	}
	r1 := eng2.RunScan(1, ScanInput{DurationMs: 10, Inputs: map[string]bool{"I1": true}})
	if !r1.State.Coils["B"] {
		t.Fatalf("B must see A from previous scan, got false")
	}
}

// Self-holding (seal-in) circuit: Start sets Q, Q keeps itself on until Stop.
func TestSelfHoldingCircuit(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["Start", "Stop"],
	  "coils": ["Motor"],
	  "statements": [
	    {"coil": {"target": "Motor", "condition": {"and": [
	      {"or": [{"contact": "Start"}, {"contact": "Motor"}]},
	      {"not": {"contact": "Stop"}}
	    ]}}}
	  ]
	}`)
	eng := NewEngine(p)
	scans := []ScanInput{
		{DurationMs: 5, Inputs: map[string]bool{"Start": false, "Stop": false}},
		{DurationMs: 5, Inputs: map[string]bool{"Start": true, "Stop": false}},
		{DurationMs: 5, Inputs: map[string]bool{"Start": false, "Stop": false}},
		{DurationMs: 5, Inputs: map[string]bool{"Start": false, "Stop": true}},
	}
	want := []bool{false, true, true, false}
	for i, s := range scans {
		res := eng.RunScan(i, s)
		if res.State.Coils["Motor"] != want[i] {
			t.Fatalf("scan %d: Motor=%v, want %v", i, res.State.Coils["Motor"], want[i])
		}
	}
}

// TON accumulates while true, caps at preset and sets done; clears on false.
func TestTON(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["Q"],
	  "timers": [{"name": "T1", "preset": 20}],
	  "statements": [
	    {"ton": {"timer": "T1", "condition": {"contact": "I1"}}},
	    {"coil": {"target": "Q", "condition": {"contact": "T1"}}}
	  ]
	}`)
	eng := NewEngine(p)
	on := ScanInput{DurationMs: 8, Inputs: map[string]bool{"I1": true}}
	off := ScanInput{DurationMs: 8, Inputs: map[string]bool{"I1": false}}

	r := eng.RunScan(0, on) // acc 8
	if r.State.Timers["T1"].Acc != 8 || r.State.Timers["T1"].Done {
		t.Fatalf("scan0: %+v", r.State.Timers["T1"])
	}
	r = eng.RunScan(1, on) // acc 16
	if r.State.Timers["T1"].Acc != 16 || r.State.Timers["T1"].Done {
		t.Fatalf("scan1: %+v", r.State.Timers["T1"])
	}
	r = eng.RunScan(2, on) // acc 24 -> capped at 20, done
	if r.State.Timers["T1"].Acc != 20 || !r.State.Timers["T1"].Done {
		t.Fatalf("scan2 cap: %+v", r.State.Timers["T1"])
	}
	if !r.State.Coils["Q"] {
		t.Fatalf("Q must follow T1 done flag in same scan")
	}
	r = eng.RunScan(3, on) // stays capped
	if r.State.Timers["T1"].Acc != 20 || !r.State.Timers["T1"].Done {
		t.Fatalf("scan3 hold: %+v", r.State.Timers["T1"])
	}
	r = eng.RunScan(4, off) // immediate clear
	if r.State.Timers["T1"].Acc != 0 || r.State.Timers["T1"].Done || r.State.Coils["Q"] {
		t.Fatalf("scan4 clear: %+v Q=%v", r.State.Timers["T1"], r.State.Coils["Q"])
	}
}

// CTU counts only false->true edges, caps at preset, reset has priority,
// and edge memory updates even during reset.
func TestCTUEdgeCapReset(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["Pulse", "Rst"],
	  "counters": [{"name": "C1", "preset": 2}],
	  "statements": [
	    {"ctu": {"counter": "C1",
	      "condition": {"contact": "Pulse"},
	      "reset": {"contact": "Rst"}}}
	  ]
	}`)
	eng := NewEngine(p)
	step := func(i int, pulse, rst bool) *CounterState {
		res := eng.RunScan(i, ScanInput{DurationMs: 10, Inputs: map[string]bool{"Pulse": pulse, "Rst": rst}})
		return res.State.Counters["C1"]
	}
	if c := step(0, true, false); c.Count != 1 { // rising edge
		t.Fatalf("edge count: %+v", c)
	}
	if c := step(1, true, false); c.Count != 1 { // held true: no count
		t.Fatalf("held true must not count: %+v", c)
	}
	if c := step(2, true, false); c.Count != 1 {
		t.Fatalf("still held: %+v", c)
	}
	if c := step(3, false, false); c.Count != 1 { // release
		t.Fatalf("release: %+v", c)
	}
	if c := step(4, true, false); c.Count != 2 || !c.Done { // reaches preset
		t.Fatalf("preset reached: %+v", c)
	}
	if c := step(5, false, false); !c.Done || c.Count != 2 {
		t.Fatalf("done persists: %+v", c)
	}
	if c := step(6, true, false); c.Count != 2 { // capped at preset
		t.Fatalf("cap: %+v", c)
	}
	// Reset while pulse is true: reset wins, edge memory still updates.
	if c := step(7, true, true); c.Count != 0 || c.Done {
		t.Fatalf("reset priority: %+v", c)
	}
	// Release reset with pulse still true: no spurious count.
	if c := step(8, true, false); c.Count != 0 {
		t.Fatalf("reset release must not count: %+v", c)
	}
	// Fresh edge counts again.
	if c := step(9, false, false); c.Count != 0 {
		t.Fatalf("release: %+v", c)
	}
	if c := step(10, true, false); c.Count != 1 {
		t.Fatalf("count after reset: %+v", c)
	}
}

func TestDiffsReported(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["A"],
	  "statements": [
	    {"coil": {"target": "A", "condition": {"contact": "I1"}}}
	  ]
	}`)
	eng := NewEngine(p)
	res := eng.RunScan(0, ScanInput{DurationMs: 3, Inputs: map[string]bool{"I1": true}})
	if len(res.Diffs) != 1 {
		t.Fatalf("want 1 diff, got %d", len(res.Diffs))
	}
	d := res.Diffs[0]
	if d.Target != "A" || d.Before != false || d.After != true || !d.Changed() {
		t.Fatalf("bad diff: %+v", d)
	}
	res = eng.RunScan(1, ScanInput{DurationMs: 3, Inputs: map[string]bool{"I1": true}})
	if res.Diffs[0].Changed() {
		t.Fatalf("unchanged write must report Changed()=false")
	}
}
