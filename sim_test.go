package main

import (
	"math/big"
	"strings"
	"testing"
)

func rat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("bad rat " + s)
	}
	return r
}

func testCfg(rate int64, streams ...StreamConfig) *Config {
	return &Config{LinkRateBytesPerSec: rate, Streams: streams}
}

func arrivalsFrom(t *testing.T, cfg *Config, csvText string) []Arrival {
	t.Helper()
	ev, err := ParseArrivals(strings.NewReader(csvText), cfg)
	if err != nil {
		t.Fatalf("parse arrivals: %v", err)
	}
	return ev
}

func sentByID(res *Result) map[string]SentRecord {
	m := map[string]SentRecord{}
	for _, s := range res.Sent {
		m[s.Packet] = s
	}
	return m
}

// Unequal lengths: a big head-of-line packet must not be skipped;
// its deficit accumulates across rounds until it fits.
func TestUnequalPacketsHeadOfLineBlocking(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 100, MaxWaitingBytes: 10000},
		StreamConfig{ID: "b", Quantum: 100, MaxWaitingBytes: 10000},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,p-big,150
0,b,p-small,50
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sent) != 2 {
		t.Fatalf("want 2 sent, got %d", len(res.Sent))
	}
	// b's small packet goes first; a's big packet waits one extra round.
	if res.Sent[0].Packet != "p-small" || res.Sent[1].Packet != "p-big" {
		t.Fatalf("order: %v, %v", res.Sent[0].Packet, res.Sent[1].Packet)
	}
	m := sentByID(res)
	if m["p-small"].Start.Cmp(rat("0")) != 0 || m["p-small"].Finish.Cmp(rat("1/20")) != 0 {
		t.Fatalf("p-small window %s..%s", m["p-small"].Start, m["p-small"].Finish)
	}
	if m["p-big"].Start.Cmp(rat("1/20")) != 0 || m["p-big"].Finish.Cmp(rat("1/5")) != 0 {
		t.Fatalf("p-big window %s..%s", m["p-big"].Start, m["p-big"].Finish)
	}
}

// After all streams go idle, the clock jumps to the next arrival and the
// round restarts from the lexicographically smallest stream. A packet
// larger than one quantum still completes by accumulating over rounds.
func TestIdleRecoveryAndMultiRoundAccumulation(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 100, MaxWaitingBytes: 10000},
		StreamConfig{ID: "b", Quantum: 100, MaxWaitingBytes: 10000},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,b,p1,50
10,a,p2,150
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	m := sentByID(res)
	if m["p1"].Start.Cmp(rat("0")) != 0 || m["p1"].Finish.Cmp(rat("1/20")) != 0 {
		t.Fatalf("p1 window %s..%s", m["p1"].Start, m["p1"].Finish)
	}
	// p2 needs two rounds of quantum (100+100 >= 150); no idle credit
	// from the gap at t in (0.05, 10) may leak in.
	if m["p2"].Start.Cmp(rat("10")) != 0 || m["p2"].Finish.Cmp(rat("203/20")) != 0 {
		t.Fatalf("p2 window %s..%s", m["p2"].Start, m["p2"].Finish)
	}
	if res.Finished.Cmp(rat("203/20")) != 0 {
		t.Fatalf("finished %s", res.Finished)
	}
}

// An arrival exactly at a send-completion instant is admitted before the
// current visit continues, and the current stream gets no extra quantum.
func TestBoundaryArrivalAtCompletion(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 1000, MaxWaitingBytes: 10000},
		StreamConfig{ID: "b", Quantum: 1000, MaxWaitingBytes: 10000},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,p1,100
0.1,b,p2,60
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	m := sentByID(res)
	if m["p1"].Finish.Cmp(rat("1/10")) != 0 {
		t.Fatalf("p1 finish %s", m["p1"].Finish)
	}
	// p2 is admitted at t=0.1 and sent immediately when b is visited.
	if m["p2"].Start.Cmp(rat("1/10")) != 0 || m["p2"].Finish.Cmp(rat("4/25")) != 0 {
		t.Fatalf("p2 window %s..%s", m["p2"].Start, m["p2"].Finish)
	}
}

// Same-instant arrivals keep input order (stable sort).
func TestSameInstantStableOrder(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 1000, MaxWaitingBytes: 10000},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,first,10
0,a,second,10
0,a,third,10
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if res.Sent[i].Packet != w {
			t.Fatalf("send %d = %s, want %s", i, res.Sent[i].Packet, w)
		}
	}
}

// Over-capacity arrivals are dropped with a reason; existing queue content
// is untouched and the byte books balance.
func TestCapacityDropAndReconciliation(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 1000, MaxWaitingBytes: 100},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,p1,60
0,a,p2,60
0.5,a,p3,40
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 1 || res.Dropped[0].Packet != "p2" {
		t.Fatalf("drops: %+v", res.Dropped)
	}
	if !strings.Contains(res.Dropped[0].Reason, "waiting room full") {
		t.Fatalf("drop reason: %q", res.Dropped[0].Reason)
	}
	st := res.Stats["a"]
	if st.AdmittedByte != 100 || st.SentBytes != 100 || st.DroppedBytes != 60 {
		t.Fatalf("stats: admitted=%d sent=%d dropped=%d",
			st.AdmittedByte, st.SentBytes, st.DroppedBytes)
	}
	if st.AdmittedByte != st.SentBytes {
		t.Fatal("admitted != sent after drain")
	}
	// p1 is sent 0..0.06; p3 arrives at 0.5 into an empty room and is sent.
	m := sentByID(res)
	if m["p3"].Start.Cmp(rat("1/2")) != 0 {
		t.Fatalf("p3 start %s", m["p3"].Start)
	}
}

// A packet already in transmission does not count against waiting capacity.
func TestInFlightNotCountedInCapacity(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 1000, MaxWaitingBytes: 80},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,p1,80
0.05,a,p2,80
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("unexpected drops: %+v", res.Dropped)
	}
	m := sentByID(res)
	if m["p2"].Start.Cmp(rat("2/25")) != 0 {
		t.Fatalf("p2 start %s", m["p2"].Start)
	}
}

// Queue drained at completion: deficit resets, so the next burst starts
// from zero credit rather than leftover balance.
func TestDeficitResetAfterDrain(t *testing.T) {
	cfg := testCfg(1000,
		StreamConfig{ID: "a", Quantum: 100, MaxWaitingBytes: 10000},
		StreamConfig{ID: "b", Quantum: 100, MaxWaitingBytes: 10000},
	)
	ev := arrivalsFrom(t, cfg, `time,stream,packet,length
0,a,p1,50
0.05,a,p2,150
0.05,b,p3,50
`)
	res, err := Simulate(cfg, ev)
	if err != nil {
		t.Fatal(err)
	}
	m := sentByID(res)
	// p1 sent 0..0.05, deficit reset to 0. At t=0.05 p2/p3 arrive.
	// Round: a gets 100 (p2=150 too big), b sends p3, then a reaches 200.
	if m["p3"].Start.Cmp(rat("1/20")) != 0 {
		t.Fatalf("p3 start %s", m["p3"].Start)
	}
	if m["p2"].Start.Cmp(rat("1/10")) != 0 {
		t.Fatalf("p2 start %s (leftover deficit leaked?)", m["p2"].Start)
	}
}
