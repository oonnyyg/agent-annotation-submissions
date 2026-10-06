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

func records(t *testing.T, src string) []ScanRecord {
	t.Helper()
	recs, err := LoadRecords(strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadRecords: %v", err)
	}
	return recs
}

// Later statements see writes made earlier in the same scan; earlier
// statements that read a coil written later still see the old value.
func TestSameScanDependency(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["Q1", "Q2", "Q3"],
	  "statements": [
	    {"type":"coil","target":"Q3","condition":{"op":"contact","name":"Q1"}},
	    {"type":"coil","target":"Q1","condition":{"op":"contact","name":"I1"}},
	    {"type":"coil","target":"Q2","condition":{"op":"contact","name":"Q1"}}
	  ]
	}`)
	recs := records(t, `
{"scan":1,"duration_ms":10,"inputs":{"I1":true}}
{"scan":2,"duration_ms":10,"inputs":{"I1":true}}
`)
	res, err := p.Run(recs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Scan 1: Q3 reads Q1 before it is written -> old value false.
	// Q2 reads Q1 after it is written -> new value true.
	if res[0].State.Coils["Q3"] || !res[0].State.Coils["Q1"] || !res[0].State.Coils["Q2"] {
		t.Fatalf("scan1 coils = %v", res[0].State.Coils)
	}
	// Scan 2: Q3 now sees the latched Q1 from scan 1.
	if !res[1].State.Coils["Q3"] {
		t.Fatalf("scan2 Q3 should be true, coils = %v", res[1].State.Coils)
	}
}

// Seal-in (self-holding) circuit: Q = I_start OR (Q AND NOT I_stop).
func TestSelfHoldingCircuit(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I_start", "I_stop"],
	  "coils": ["Q"],
	  "statements": [
	    {"type":"coil","target":"Q","condition":{"op":"or","args":[
	      {"op":"contact","name":"I_start"},
	      {"op":"and","args":[
	        {"op":"contact","name":"Q"},
	        {"op":"not","args":[{"op":"contact","name":"I_stop"}]}
	      ]}
	    ]}}
	  ]
	}`)
	recs := records(t, `
{"scan":1,"duration_ms":5,"inputs":{"I_start":true,"I_stop":false}}
{"scan":2,"duration_ms":5,"inputs":{"I_start":false,"I_stop":false}}
{"scan":3,"duration_ms":5,"inputs":{"I_start":false,"I_stop":true}}
{"scan":4,"duration_ms":5,"inputs":{"I_start":false,"I_stop":false}}
`)
	res, err := p.Run(recs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []bool{true, true, false, false}
	for i, w := range want {
		if got := res[i].State.Coils["Q"]; got != w {
			t.Fatalf("scan %d: Q = %v, want %v", i+1, got, w)
		}
	}
}

// TON accumulates scan durations while true, caps at preset, sets done,
// and clears immediately when the condition goes false.
func TestTON(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["Q"],
	  "timers": [{"name":"T1","preset_ms":30}],
	  "statements": [
	    {"type":"ton","timer":"T1","condition":{"op":"contact","name":"I1"}},
	    {"type":"coil","target":"Q","condition":{"op":"done","name":"T1"}}
	  ]
	}`)
	recs := records(t, `
{"scan":1,"duration_ms":10,"inputs":{"I1":true}}
{"scan":2,"duration_ms":25,"inputs":{"I1":true}}
{"scan":3,"duration_ms":10,"inputs":{"I1":true}}
{"scan":4,"duration_ms":10,"inputs":{"I1":false}}
`)
	res, err := p.Run(recs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ts := func(i int) *TimerState { return res[i].State.Timers["T1"] }
	if ts(0).AccumulatedMS != 10 || ts(0).Done {
		t.Fatalf("scan1 timer = %+v", ts(0))
	}
	// 10+25 = 35, capped at preset 30, done set.
	if ts(1).AccumulatedMS != 30 || !ts(1).Done || !res[1].State.Coils["Q"] {
		t.Fatalf("scan2 timer = %+v, Q=%v", ts(1), res[1].State.Coils["Q"])
	}
	// Stays capped while condition holds.
	if ts(2).AccumulatedMS != 30 || !ts(2).Done {
		t.Fatalf("scan3 timer = %+v", ts(2))
	}
	// Condition false: immediate clear.
	if ts(3).AccumulatedMS != 0 || ts(3).Done || res[3].State.Coils["Q"] {
		t.Fatalf("scan4 timer = %+v, Q=%v", ts(3), res[3].State.Coils["Q"])
	}
}

// CTU counts only false->true edges, caps at preset, reset wins over count,
// and edge memory updates even during reset.
func TestCTUEdgeAndReset(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I_count", "I_reset"],
	  "coils": ["Q"],
	  "counters": [{"name":"C1","preset":2}],
	  "statements": [
	    {"type":"ctu","counter":"C1",
	     "condition":{"op":"contact","name":"I_count"},
	     "reset":{"op":"contact","name":"I_reset"}},
	    {"type":"coil","target":"Q","condition":{"op":"done","name":"C1"}}
	  ]
	}`)
	recs := records(t, `
{"scan":1,"duration_ms":10,"inputs":{"I_count":true,"I_reset":false}}
{"scan":2,"duration_ms":10,"inputs":{"I_count":true,"I_reset":false}}
{"scan":3,"duration_ms":10,"inputs":{"I_count":false,"I_reset":false}}
{"scan":4,"duration_ms":10,"inputs":{"I_count":true,"I_reset":false}}
{"scan":5,"duration_ms":10,"inputs":{"I_count":false,"I_reset":false}}
{"scan":6,"duration_ms":10,"inputs":{"I_count":true,"I_reset":false}}
{"scan":7,"duration_ms":10,"inputs":{"I_count":true,"I_reset":true}}
{"scan":8,"duration_ms":10,"inputs":{"I_count":true,"I_reset":false}}
`)
	res, err := p.Run(recs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	val := func(i int) int64 { return res[i].State.Counters["C1"].Value }
	if val(0) != 1 {
		t.Fatalf("scan1 value = %d, want 1 (rising edge)", val(0))
	}
	if val(1) != 1 {
		t.Fatalf("scan2 value = %d, want 1 (held true, no edge)", val(1))
	}
	if val(3) != 2 || !res[3].State.Counters["C1"].Done || !res[3].State.Coils["Q"] {
		t.Fatalf("scan4 value = %d, want 2 with done", val(3))
	}
	// Scan 6: another edge, but value is capped at preset 2.
	if val(5) != 2 {
		t.Fatalf("scan6 value = %d, want 2 (capped)", val(5))
	}
	// Scan 7: reset wins even though condition is true.
	if val(6) != 0 || res[6].State.Counters["C1"].Done {
		t.Fatalf("scan7 value = %d, want 0 (reset priority)", val(6))
	}
	// Scan 8: reset released while condition still true; edge memory was
	// updated during reset, so no spurious count.
	if val(7) != 0 {
		t.Fatalf("scan8 value = %d, want 0 (no re-count after reset release)", val(7))
	}
}
