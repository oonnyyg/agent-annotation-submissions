package main

import (
	"math/big"
	"testing"
)

func cents(v int64) *int64 { return &v }

func buy(id, time, qty string, total, fee int64) Event {
	return Event{ID: id, Time: time, Type: EventBuy, Quantity: qty, TotalCents: cents(total), FeeCents: cents(fee)}
}

func sell(id, time, qty string, total, fee int64) Event {
	return Event{ID: id, Time: time, Type: EventSell, Quantity: qty, TotalCents: cents(total), FeeCents: cents(fee)}
}

func split(id, time, ratio string) Event {
	return Event{ID: id, Time: time, Type: EventSplit, Ratio: ratio}
}

func ratEq(t *testing.T, got *big.Rat, want string) {
	t.Helper()
	w, ok := new(big.Rat).SetString(want)
	if !ok {
		t.Fatalf("bad want value %q", want)
	}
	if got.Cmp(w) != 0 {
		t.Fatalf("got %s, want %s", got.RatString(), w.RatString())
	}
}

func acceptedSeqs(l *Ledger) map[int]bool {
	out := map[int]bool{}
	for _, r := range l.Results() {
		if r.Accepted {
			out[r.Seq] = true
		}
	}
	return out
}

// Partial sell followed by a split: quantities scale, cost basis and lot
// order stay intact, and totals reconcile exactly.
func TestPartialSellThenSplit(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "4", 40000, 100), // cost 40100
		sell("s1", "2024-01-02T00:00:00Z", "1.5", 18000, 0),
		split("x1", "2024-01-03T00:00:00Z", "3/2"),
	}
	l := NewLedger()
	l.Replay(events)
	for _, r := range l.Results() {
		if !r.Accepted {
			t.Fatalf("event #%d rejected: %s", r.Seq, r.Reason)
		}
	}
	snap := l.Snapshot()
	if len(snap.Lots) != 1 {
		t.Fatalf("want 1 lot, got %d", len(snap.Lots))
	}
	lot := snap.Lots[0]
	// remaining qty: (4 - 1.5) * 3/2 = 15/4
	ratEq(t, lot.Qty, "15/4")
	// remaining cost: 40100 * (2.5/4) = 40100 * 5/8 = 250625/10... exact: 200500/8
	ratEq(t, lot.Cost, "100250/4")
	// sale consumed 40100 * 1.5/4 = 150375/1000 -> 120300/8
	sale := snap.Sales[0]
	ratEq(t, sale.Sources[0].Cost, "60150/4")
	ratEq(t, sale.Sources[0].Proceeds, "18000")
	ratEq(t, sale.Sources[0].Realized, "11850/4")
	// reconciliation: remaining cost + consumed cost == original cost
	sum := new(big.Rat).Add(lot.Cost, sale.Sources[0].Cost)
	ratEq(t, sum, "40100")
}

// Exact rational reconciliation with non-terminating decimals: no rounding
// anywhere, totals still tie out.
func TestExactRationalReconciliation(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "3", 100, 0),
		sell("s1", "2024-01-02T00:00:00Z", "1", 40, 1), // net 39
	}
	l := NewLedger()
	l.Replay(events)
	snap := l.Snapshot()
	lot := snap.Lots[0]
	ratEq(t, lot.Qty, "2")
	ratEq(t, lot.Cost, "200/3")
	src := snap.Sales[0].Sources[0]
	ratEq(t, src.Cost, "100/3")
	ratEq(t, src.Proceeds, "39")
	ratEq(t, src.Realized, "17/3")
	qty, cost := snap.Totals()
	ratEq(t, qty, "2")
	ratEq(t, cost, "200/3")
}

// A specific sell exceeding the lot's remaining quantity is rejected
// atomically: the sequence number is consumed but no lot is touched.
func TestSpecificSellInsufficientLotRejected(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "2", 200, 0),
		buy("b2", "2024-01-02T00:00:00Z", "2", 300, 0),
		{ID: "s1", Time: "2024-01-03T00:00:00Z", Type: EventSell, Method: "specific",
			Quantity: "3", TotalCents: cents(450), FeeCents: cents(0),
			Allocations: []Allocation{{Lot: "b1", Quantity: "2"}, {Lot: "b2", Quantity: "1.5"}}},
		buy("b3", "2024-01-04T00:00:00Z", "1", 100, 0),
	}
	l := NewLedger()
	l.Replay(events)
	results := l.Results()
	if len(results) != 4 {
		t.Fatalf("want 4 results, got %d", len(results))
	}
	if results[2].Accepted {
		t.Fatal("oversubscribed specific sell must be rejected")
	}
	if !results[3].Accepted {
		t.Fatalf("subsequent event must still be processed: %s", results[3].Reason)
	}
	snap := l.Snapshot()
	if len(snap.Lots) != 3 {
		t.Fatalf("rejected sell must not deduct; want 3 lots, got %d", len(snap.Lots))
	}
	ratEq(t, snap.Lots[0].Qty, "2")
	ratEq(t, snap.Lots[0].Cost, "200")
	ratEq(t, snap.Lots[1].Qty, "2")
	ratEq(t, snap.Lots[1].Cost, "300")
}

// Allocation total must equal the sell quantity exactly.
func TestAllocationSumMismatchRejected(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "2", 200, 0),
		{ID: "s1", Time: "2024-01-02T00:00:00Z", Type: EventSell, Method: "specific",
			Quantity: "1", TotalCents: cents(100), FeeCents: cents(0),
			Allocations: []Allocation{{Lot: "b1", Quantity: "0.5"}}},
	}
	l := NewLedger()
	l.Replay(events)
	if l.Results()[1].Accepted {
		t.Fatal("mismatched allocation sum must be rejected")
	}
	ratEq(t, l.Snapshot().Lots[0].Qty, "2")
}

// FIFO consumes the oldest lot first and proceeds are split pro-rata.
func TestFIFOSpanningLots(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "1", 100, 0),
		buy("b2", "2024-01-02T00:00:00Z", "1", 200, 0),
		sell("s1", "2024-01-03T00:00:00Z", "1.5", 300, 0),
	}
	l := NewLedger()
	l.Replay(events)
	sale := l.Snapshot().Sales[0]
	if len(sale.Sources) != 2 {
		t.Fatalf("want 2 sources, got %d", len(sale.Sources))
	}
	if sale.Sources[0].LotID != "b1" || sale.Sources[1].LotID != "b2" {
		t.Fatal("FIFO order violated")
	}
	ratEq(t, sale.Sources[0].Qty, "1")
	ratEq(t, sale.Sources[0].Cost, "100")
	ratEq(t, sale.Sources[0].Proceeds, "200") // 300 * 1/1.5
	ratEq(t, sale.Sources[1].Qty, "1/2")
	ratEq(t, sale.Sources[1].Cost, "100")
	ratEq(t, sale.Sources[1].Proceeds, "100")
	snap := l.Snapshot()
	ratEq(t, snap.Lots[0].Qty, "1/2")
	ratEq(t, snap.Lots[0].Cost, "100")
}

// Historical snapshots replay a prefix on a fresh ledger and never disturb
// the live replay; rejected events occupy their sequence numbers.
func TestSnapshotIsolationAndSequence(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-01T00:00:00Z", "2", 200, 0),
		sell("bad", "2024-01-02T00:00:00Z", "5", 500, 0), // rejected: insufficient
		split("x1", "2024-01-03T00:00:00Z", "2"),
	}
	snap1, results, err := SnapshotAt(events, 1)
	if err != nil {
		t.Fatal(err)
	}
	if results[1].Accepted {
		t.Fatal("event #1 should be rejected")
	}
	ratEq(t, snap1.Lots[0].Qty, "2") // split at #2 not yet applied

	snap2, _, err := SnapshotAt(events, 2)
	if err != nil {
		t.Fatal(err)
	}
	ratEq(t, snap2.Lots[0].Qty, "4")
	ratEq(t, snap2.Lots[0].Cost, "200") // split never changes cost

	// Querying history again still shows the pre-split state.
	snap1b, _, err := SnapshotAt(events, 1)
	if err != nil {
		t.Fatal(err)
	}
	ratEq(t, snap1b.Lots[0].Qty, "2")
	if _, _, err := SnapshotAt(events, 3); err == nil {
		t.Fatal("out-of-range snapshot must fail")
	}
}

// Header validation: duplicate ids and backwards timestamps are rejected
// without disturbing state.
func TestHeaderValidation(t *testing.T) {
	events := []Event{
		buy("b1", "2024-01-02T00:00:00Z", "1", 100, 0),
		buy("b1", "2024-01-03T00:00:00Z", "1", 100, 0), // duplicate id
		buy("b2", "2024-01-01T00:00:00Z", "1", 100, 0), // time backwards
		buy("b3", "2024-01-04T00:00:00Z", "1", 100, 0), // ok
	}
	l := NewLedger()
	l.Replay(events)
	got := acceptedSeqs(l)
	if got[1] || got[2] || !got[0] || !got[3] {
		t.Fatalf("unexpected accept pattern: %v", got)
	}
	qty, _ := l.Snapshot().Totals()
	ratEq(t, qty, "2")
}
