package main

import (
	"math/big"
	"testing"
)

func TestRejectedEventKeepsSeqAndState(t *testing.T) {
	l := NewLedger()
	events := []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "1", AmountCents: 10},
		{ID: "b1", Timestamp: "2026-01-02T00:00:00Z", Type: "buy", Qty: "9", AmountCents: 90}, // dup id
		{ID: "b2", Timestamp: "2025-12-31T23:00:00Z", Type: "buy", Qty: "9", AmountCents: 90}, // ts backwards
		{ID: "b2", Timestamp: "2026-01-03T00:00:00Z", Type: "buy", Qty: "2", AmountCents: 20},
	}
	for i, ev := range events {
		err := l.Apply(ev)
		if i == 1 || i == 2 {
			if err == nil {
				t.Fatalf("event %d should be rejected", i)
			}
		} else if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
	if l.seq != 4 {
		t.Fatalf("seq = %d, want 4 (rejected events still count)", l.seq)
	}
	if len(l.lots) != 2 || l.lots[1].Seq != 3 {
		t.Fatalf("lots=%d second lot seq=%d", len(l.lots), l.lots[1].Seq)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	events := []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "4", AmountCents: 40},
		{ID: "bad", Timestamp: "2026-01-02T00:00:00Z", Type: "sell", Qty: "99", AmountCents: 1},
		{ID: "s1", Timestamp: "2026-01-03T00:00:00Z", Type: "sell", Qty: "1", AmountCents: 15},
	}
	main := NewLedger()
	for _, ev := range events {
		main.Apply(ev)
	}
	snap, err := Snapshot(events, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot after index 1: only the buy is in effect (sell was rejected).
	if len(snap.lots) != 1 || snap.lots[0].Open.Cmp(mustRat(t, "4")) != 0 {
		t.Fatalf("snapshot qty = %s", ratString(snap.lots[0].Open))
	}
	if len(snap.sales) != 0 {
		t.Fatal("snapshot should not contain the rejected sale")
	}
	// Main replay must be unaffected by the snapshot query.
	if main.lots[0].Open.Cmp(mustRat(t, "3")) != 0 || len(main.sales) != 1 {
		t.Fatal("snapshot polluted main replay")
	}
	if _, err := Snapshot(events, 5); err == nil {
		t.Fatal("expected out-of-range snapshot error")
	}
}

func TestFractionalCostReconciliation(t *testing.T) {
	// Buy 1/3 unit for 100 cents; sell in three equal explicit slices.
	// Per-slice cost 100/3 must sum back to exactly 100.
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "1/3", AmountCents: 100},
	})
	for i, ts := range []string{"2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z", "2026-01-04T00:00:00Z"} {
		err := l.Apply(Event{
			ID: "s" + string(rune('1'+i)), Timestamp: ts, Type: "sell",
			Qty: "1/9", AmountCents: 40, Method: "explicit",
			Allocations: []Allocation{{LotID: "b1", Qty: "1/9"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !l.lots[0].Closed || l.lots[0].Cost.Sign() != 0 {
		t.Fatalf("residual cost %s", ratString(l.lots[0].Cost))
	}
	total := new(big.Rat)
	for _, s := range l.sales {
		total.Add(total, s.Realized)
	}
	// proceeds 120 - cost 100 = 20 exactly, no rounding drift.
	if total.Cmp(mustRat(t, "20")) != 0 {
		t.Fatalf("total realized = %s", ratString(total))
	}
}

func TestSplitRejectsBadRatio(t *testing.T) {
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "2", AmountCents: 10},
	})
	for _, ratio := range []string{"0", "-2", "abc"} {
		if err := l.Apply(Event{ID: "x" + ratio, Timestamp: "2026-01-02T00:00:00Z", Type: "split", Ratio: ratio}); err == nil {
			t.Fatalf("ratio %q should be rejected", ratio)
		}
	}
	if l.lots[0].Open.Cmp(mustRat(t, "2")) != 0 {
		t.Fatal("state mutated by rejected split")
	}
}
