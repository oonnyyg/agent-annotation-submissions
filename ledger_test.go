package main

import (
	"math/big"
	"testing"
)

func mustRat(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("bad rat %q", s)
	}
	return r
}

func applyAll(t *testing.T, l *Ledger, events []Event) {
	t.Helper()
	for _, ev := range events {
		if err := l.Apply(ev); err != nil {
			t.Fatalf("event %s rejected: %v", ev.ID, err)
		}
	}
}

func TestBuySellFIFOExact(t *testing.T) {
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "3", AmountCents: 100, FeeCents: 2},
		{ID: "b2", Timestamp: "2026-01-02T00:00:00Z", Type: "buy", Qty: "1/2", AmountCents: 30},
		{ID: "s1", Timestamp: "2026-01-03T00:00:00Z", Type: "sell", Qty: "2", AmountCents: 200, FeeCents: 3},
	})
	if len(l.sales) != 1 {
		t.Fatalf("want 1 sale, got %d", len(l.sales))
	}
	s := l.sales[0]
	// proceeds 197 split 2:0 across one lot draw of qty 2 from b1.
	if s.Proceeds.Cmp(mustRat(t, "197")) != 0 {
		t.Fatalf("proceeds = %s", ratString(s.Proceeds))
	}
	// cost consumed: 102 * 2/3 = 68
	if s.Sources[0].Cost.Cmp(mustRat(t, "68")) != 0 {
		t.Fatalf("cost out = %s", ratString(s.Sources[0].Cost))
	}
	if s.Realized.Cmp(mustRat(t, "129")) != 0 {
		t.Fatalf("realized = %s", ratString(s.Realized))
	}
	// remaining: b1 qty 1 cost 34, b2 qty 1/2 cost 30
	if l.lots[0].Open.Cmp(mustRat(t, "1")) != 0 || l.lots[0].Cost.Cmp(mustRat(t, "34")) != 0 {
		t.Fatalf("b1 remaining qty=%s cost=%s", ratString(l.lots[0].Open), ratString(l.lots[0].Cost))
	}
	if l.lots[1].Open.Cmp(mustRat(t, "1/2")) != 0 {
		t.Fatalf("b2 remaining qty=%s", ratString(l.lots[1].Open))
	}
}

func TestPartialSellThenSplit(t *testing.T) {
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "10", AmountCents: 1000},
		{ID: "s1", Timestamp: "2026-01-02T00:00:00Z", Type: "sell", Qty: "3", AmountCents: 330},
		{ID: "x1", Timestamp: "2026-01-03T00:00:00Z", Type: "split", Ratio: "3/2"},
	})
	lot := l.lots[0]
	// 7 remaining * 3/2 = 21/2; cost stays 700.
	if lot.Open.Cmp(mustRat(t, "21/2")) != 0 {
		t.Fatalf("qty after split = %s", ratString(lot.Open))
	}
	if lot.Cost.Cmp(mustRat(t, "700")) != 0 {
		t.Fatalf("cost after split = %s", ratString(lot.Cost))
	}
	// Past sale must be untouched by the split.
	if l.sales[0].Qty.Cmp(mustRat(t, "3")) != 0 || l.sales[0].Realized.Cmp(mustRat(t, "30")) != 0 {
		t.Fatalf("sale rewritten: qty=%s realized=%s",
			ratString(l.sales[0].Qty), ratString(l.sales[0].Realized))
	}
	// Sell everything after the split: books must reconcile exactly.
	if err := l.Apply(Event{ID: "s2", Timestamp: "2026-01-04T00:00:00Z", Type: "sell", Qty: "10.5", AmountCents: 1155}); err != nil {
		t.Fatal(err)
	}
	if !l.lots[0].Closed || l.lots[0].Cost.Sign() != 0 {
		t.Fatalf("lot not fully closed: qty=%s cost=%s", ratString(l.lots[0].Open), ratString(l.lots[0].Cost))
	}
	totalRealized := new(big.Rat)
	for _, s := range l.sales {
		totalRealized.Add(totalRealized, s.Realized)
	}
	// total proceeds 330+1155=1485, total cost 1000 -> realized 485.
	if totalRealized.Cmp(mustRat(t, "485")) != 0 {
		t.Fatalf("total realized = %s", ratString(totalRealized))
	}
}

func TestExplicitAllocationInsufficient(t *testing.T) {
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "2", AmountCents: 100},
		{ID: "b2", Timestamp: "2026-01-02T00:00:00Z", Type: "buy", Qty: "2", AmountCents: 200},
	})
	err := l.Apply(Event{
		ID: "s1", Timestamp: "2026-01-03T00:00:00Z", Type: "sell", Qty: "3",
		AmountCents: 300, Method: "explicit",
		Allocations: []Allocation{{LotID: "b1", Qty: "3"}},
	})
	if err == nil {
		t.Fatal("expected rejection for over-allocated lot")
	}
	// No partial deduction: both lots intact, no sale recorded.
	if l.lots[0].Open.Cmp(mustRat(t, "2")) != 0 || l.lots[1].Open.Cmp(mustRat(t, "2")) != 0 {
		t.Fatal("state mutated by rejected sell")
	}
	if len(l.sales) != 0 {
		t.Fatal("rejected sell recorded")
	}
}

func TestAllocationSumMismatch(t *testing.T) {
	l := NewLedger()
	applyAll(t, l, []Event{
		{ID: "b1", Timestamp: "2026-01-01T00:00:00Z", Type: "buy", Qty: "5", AmountCents: 100},
	})
	err := l.Apply(Event{
		ID: "s1", Timestamp: "2026-01-02T00:00:00Z", Type: "sell", Qty: "2",
		AmountCents: 50, Method: "explicit",
		Allocations: []Allocation{{LotID: "b1", Qty: "1"}},
	})
	if err == nil {
		t.Fatal("expected rejection for allocation sum mismatch")
	}
	if l.lots[0].Open.Cmp(mustRat(t, "5")) != 0 {
		t.Fatal("state mutated by rejected sell")
	}
}
