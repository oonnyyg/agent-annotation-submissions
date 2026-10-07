package main

import (
	"math"
	"testing"
)

func mustSim(t *testing.T, rate float64, cfgs []FlowConfig) *Simulator {
	t.Helper()
	s, err := NewSimulator(rate, cfgs)
	if err != nil {
		t.Fatalf("NewSimulator: %v", err)
	}
	return s
}

func run(t *testing.T, s *Simulator, events []Arrival) ([]SendRecord, []DropRecord, []FlowStat) {
	t.Helper()
	if err := s.Run(events); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return s.Results()
}

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func checkSends(t *testing.T, got []SendRecord, want []SendRecord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("send count: got %d (%v), want %d", len(got), got, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Packet != w.Packet || g.Flow != w.Flow ||
			!almostEq(g.Start, w.Start) || !almostEq(g.End, w.End) {
			t.Errorf("send[%d]: got %+v, want %+v", i, g, w)
		}
	}
}

// Unequal packet lengths: a large head packet accumulates deficit over
// several rounds before it can be sent; smaller packets on other flows
// are served in the meantime.
func TestUnequalPacketsAccumulateDeficit(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 100, WaitLimit: 10000},
		{ID: "B", Quantum: 100, WaitLimit: 10000},
	})
	sends, drops, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 150},
		{Time: 0, Flow: "B", Packet: "b1", Length: 100},
	})
	checkSends(t, sends, []SendRecord{
		{Packet: "b1", Flow: "B", Start: 0, End: 0.1},
		{Packet: "a1", Flow: "A", Start: 0.1, End: 0.25},
	})
	if len(drops) != 0 {
		t.Fatalf("unexpected drops: %v", drops)
	}
}

// Head-of-line blocking: a small packet behind a large one must not jump
// ahead, even when the deficit would cover the small one.
func TestNoHeadOfLineSkip(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 100, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "big", Length: 150},
		{Time: 0, Flow: "A", Packet: "small", Length: 10},
	})
	checkSends(t, sends, []SendRecord{
		{Packet: "big", Flow: "A", Start: 0, End: 0.15},
		{Packet: "small", Flow: "A", Start: 0.15, End: 0.16},
	})
}

// One visit adds the quantum once and sends every head packet the deficit
// covers, back to back.
func TestContinuousSendWithinVisit(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 200, WaitLimit: 10000},
		{ID: "B", Quantum: 100, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 50},
		{Time: 0, Flow: "A", Packet: "a2", Length: 60},
	})
	checkSends(t, sends, []SendRecord{
		{Packet: "a1", Flow: "A", Start: 0, End: 0.05},
		{Packet: "a2", Flow: "A", Start: 0.05, End: 0.11},
	})
}

// Idle recovery: after every flow is idle, the cycle restarts from the
// smallest flow ID and idle flows keep no accumulated deficit.
func TestIdleRecoveryRestartsFromSmallest(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 100, WaitLimit: 10000},
		{ID: "B", Quantum: 100, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "B", Packet: "b1", Length: 100},
		{Time: 5, Flow: "A", Packet: "a1", Length: 100},
		{Time: 5, Flow: "B", Packet: "b2", Length: 100},
	})
	checkSends(t, sends, []SendRecord{
		{Packet: "b1", Flow: "B", Start: 0, End: 0.1},
		{Packet: "a1", Flow: "A", Start: 5, End: 5.1},
		{Packet: "b2", Flow: "B", Start: 5.1, End: 5.2},
	})
}

// Boundary arrival: a packet arriving exactly when a transmission completes
// is admitted before the current visit continues, so the queue is not
// treated as empty and the leftover deficit is preserved.
func TestArrivalExactlyAtCompletion(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 100, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 50},
		{Time: 0.05, Flow: "A", Packet: "a2", Length: 160},
	})
	// a1 leaves deficit 50; a2 (160) needs two more quanta: 50+100+100=250.
	checkSends(t, sends, []SendRecord{
		{Packet: "a1", Flow: "A", Start: 0, End: 0.05},
		{Packet: "a2", Flow: "A", Start: 0.05, End: 0.21},
	})
	// The visit did not end at an empty queue, so the leftover deficit of
	// 250-160=90 survives until the queue drains... it drains here, so the
	// deficit must be reset to zero afterwards.
	if d := s.flows["A"].deficit; d != 0 {
		t.Fatalf("deficit after drain: got %d, want 0", d)
	}
}

// Same-instant arrivals keep input order (stable sort).
func TestSameInstantArrivalsKeepInputOrder(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 1000, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 1, Flow: "A", Packet: "first", Length: 10},
		{Time: 1, Flow: "A", Packet: "second", Length: 10},
		{Time: 1, Flow: "A", Packet: "third", Length: 10},
	})
	checkSends(t, sends, []SendRecord{
		{Packet: "first", Flow: "A", Start: 1, End: 1.01},
		{Packet: "second", Flow: "A", Start: 1.01, End: 1.02},
		{Packet: "third", Flow: "A", Start: 1.02, End: 1.03},
	})
}

// Over-limit arrivals drop only the new packet; the existing queue is
// untouched and the byte accounting reconciles.
func TestWaitingLimitDropsNewPacketOnly(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 1000, WaitLimit: 100},
	})
	sends, drops, stats := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 60},
		{Time: 0, Flow: "A", Packet: "a2", Length: 60}, // 60+60 > 100: dropped
		{Time: 0, Flow: "A", Packet: "a3", Length: 40}, // 60+40 = 100: admitted
	})
	if len(drops) != 1 || drops[0].Packet != "a2" {
		t.Fatalf("drops: got %v, want only a2", drops)
	}
	checkSends(t, sends, []SendRecord{
		{Packet: "a1", Flow: "A", Start: 0, End: 0.06},
		{Packet: "a3", Flow: "A", Start: 0.06, End: 0.1},
	})
	st := stats[0]
	if st.Arrived != 3 || st.Admitted != 2 || st.Sent != 2 || st.Dropped != 1 {
		t.Fatalf("stats: got %+v", st)
	}
	if st.ArrivedByte != st.AdmitBytes+st.DropBytes {
		t.Fatalf("byte reconciliation: arrived %d != admitted %d + dropped %d",
			st.ArrivedByte, st.AdmitBytes, st.DropBytes)
	}
	if st.AdmitBytes != st.SentBytes {
		t.Fatalf("byte reconciliation: admitted %d != sent %d", st.AdmitBytes, st.SentBytes)
	}
	// a1 waited 0s, a3 waited 0.06s (arrival 0, send start 0.06).
	if !almostEq(st.TotalWait, 0.06) || !almostEq(st.MaxWait, 0.06) {
		t.Fatalf("wait stats: got total %v max %v", st.TotalWait, st.MaxWait)
	}
}

// A packet already in service does not count against the waiting-byte
// limit, so a new arrival can be admitted while the link is busy.
func TestInServicePacketNotCountedAsWaiting(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 1000, WaitLimit: 100},
	})
	sends, drops, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 100}, // in service [0, 0.1]
		{Time: 0.02, Flow: "A", Packet: "a2", Length: 100},
	})
	if len(drops) != 0 {
		t.Fatalf("unexpected drops: %v", drops)
	}
	checkSends(t, sends, []SendRecord{
		{Packet: "a1", Flow: "A", Start: 0, End: 0.1},
		{Packet: "a2", Flow: "A", Start: 0.1, End: 0.2},
	})
}

// Arrivals during a transmission join the queue but the visit never adds
// a second quantum for them.
func TestArrivalDuringTransmissionNoExtraQuantum(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{
		{ID: "A", Quantum: 150, WaitLimit: 10000},
	})
	sends, _, _ := run(t, s, []Arrival{
		{Time: 0, Flow: "A", Packet: "a1", Length: 100},
		{Time: 0.05, Flow: "A", Packet: "a2", Length: 100},
	})
	// Visit adds 150 once: a1 (100) sends, deficit 50 < 100 stops the visit.
	// Next round adds another 150: 200 >= 100, a2 sends.
	checkSends(t, sends, []SendRecord{
		{Packet: "a1", Flow: "A", Start: 0, End: 0.1},
		{Packet: "a2", Flow: "A", Start: 0.1, End: 0.2},
	})
}

func TestValidation(t *testing.T) {
	if _, err := NewSimulator(0, []FlowConfig{{ID: "A", Quantum: 1, WaitLimit: 1}}); err == nil {
		t.Error("want error for non-positive rate")
	}
	if _, err := NewSimulator(1, []FlowConfig{{ID: "A", Quantum: 0, WaitLimit: 1}}); err == nil {
		t.Error("want error for non-positive quantum")
	}
	if _, err := NewSimulator(1, []FlowConfig{{ID: "A", Quantum: 1, WaitLimit: 0}}); err == nil {
		t.Error("want error for non-positive wait limit")
	}
	if _, err := NewSimulator(1, []FlowConfig{
		{ID: "A", Quantum: 1, WaitLimit: 1},
		{ID: "A", Quantum: 1, WaitLimit: 1},
	}); err == nil {
		t.Error("want error for duplicate flow ID")
	}
	s := mustSim(t, 1000, []FlowConfig{{ID: "A", Quantum: 100, WaitLimit: 100}})
	if err := s.Run([]Arrival{
		{Time: 0, Flow: "A", Packet: "p", Length: 10},
		{Time: 1, Flow: "A", Packet: "p", Length: 10},
	}); err == nil {
		t.Error("want error for duplicate packet ID")
	}
	s = mustSim(t, 1000, []FlowConfig{{ID: "A", Quantum: 100, WaitLimit: 100}})
	if err := s.Run([]Arrival{{Time: 0, Flow: "A", Packet: "p", Length: 0}}); err == nil {
		t.Error("want error for non-positive length")
	}
}

// Packets for an unconfigured flow are dropped with a reason and still
// appear in the accounting.
func TestUnknownFlowDropped(t *testing.T) {
	s := mustSim(t, 1000, []FlowConfig{{ID: "A", Quantum: 100, WaitLimit: 100}})
	_, drops, stats := run(t, s, []Arrival{
		{Time: 0, Flow: "ZZZ", Packet: "x1", Length: 10},
	})
	if len(drops) != 1 || drops[0].Reason != "unknown flow" {
		t.Fatalf("drops: got %v", drops)
	}
	found := false
	for _, st := range stats {
		if st.Flow == "ZZZ" {
			found = true
			if st.Dropped != 1 || st.DropBytes != 10 {
				t.Fatalf("stats: got %+v", st)
			}
		}
	}
	if !found {
		t.Fatal("no stats row for unknown flow")
	}
}
