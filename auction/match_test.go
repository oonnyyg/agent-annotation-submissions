package auction

import (
	"errors"
	"testing"
)

func TestPartialFillAndPriority(t *testing.T) {
	b := NewBook()
	mustAdd(t, b, "b1", Buy, 105, 10) // best bid first
	mustAdd(t, b, "b2", Buy, 105, 10) // same price, later seq
	mustAdd(t, b, "b3", Buy, 100, 10)
	mustAdd(t, b, "s1", Sell, 95, 15)
	mustAdd(t, b, "s2", Sell, 100, 20)
	r, err := b.Preview(100)
	if err != nil {
		t.Fatal(err)
	}
	// Candidates: 95,100,105 -> vol 20 at 100 and 105; imbalance
	// min at 100 (buy 30, sell 35 -> imb 5; at 105 imb 15). Price 100.
	if !r.Priced || r.Price != 100 || r.Volume != 30 {
		t.Fatalf("price=%d vol=%d, want 100/30", r.Price, r.Volume)
	}
	// Buy priority: b1, b2, b3. Sell priority: s1(15), s2(20).
	want := []Trade{
		{BuyID: "b1", SellID: "s1", Price: 100, Qty: 10},
		{BuyID: "b2", SellID: "s1", Price: 100, Qty: 5},
		{BuyID: "b2", SellID: "s2", Price: 100, Qty: 5},
		{BuyID: "b3", SellID: "s2", Price: 100, Qty: 10},
	}
	if len(r.Trades) != len(want) {
		t.Fatalf("trades = %+v", r.Trades)
	}
	for i, w := range want {
		if r.Trades[i] != w {
			t.Fatalf("trade %d = %+v, want %+v", i, r.Trades[i], w)
		}
	}
	// Preview must not consume anything.
	r2, _ := b.Preview(100)
	if r2.Volume != 30 {
		t.Fatal("preview consumed quantity")
	}
}

func TestExecuteConservationAndIdempotency(t *testing.T) {
	b := NewBook()
	mustAdd(t, b, "b1", Buy, 110, 10)
	mustAdd(t, b, "s1", Sell, 100, 4)
	mustAdd(t, b, "s2", Sell, 105, 10)
	e := NewExecutor(b)
	r, err := e.Execute("batch-1", 105)
	if err != nil {
		t.Fatal(err)
	}
	if r.Price != 105 || r.Volume != 10 {
		t.Fatalf("price=%d vol=%d, want 105/10", r.Price, r.Volume)
	}
	// Conservation: buy fills == sell fills == volume, per-order.
	var buySum, sellSum int64
	filled := map[string]int64{}
	for _, tr := range r.Trades {
		buySum += tr.Qty
		sellSum += tr.Qty
		filled[tr.BuyID] += tr.Qty
		filled[tr.SellID] += tr.Qty
	}
	if buySum != r.Volume || sellSum != r.Volume {
		t.Fatal("buy/sell totals diverge")
	}
	for _, o := range r.Remaining {
		if filled[o.ID]+o.Remaining != o.Qty {
			t.Fatalf("order %s: filled %d + remaining %d != qty %d",
				o.ID, filled[o.ID], o.Remaining, o.Qty)
		}
	}
	// Retry with same params: identical result, no double trading.
	r2, err := e.Execute("batch-1", 105)
	if err != nil || r2 != r {
		t.Fatalf("retry = %+v, err %v", r2, err)
	}
	if got := b.orders["b1"].Remaining; got != 0 {
		t.Fatalf("b1 remaining = %d after retry, want 0", got)
	}
	// Same batch id, different params: rejected.
	if _, err := e.Execute("batch-1", 100); !errors.Is(err, ErrBatchParamMismatch) {
		t.Fatalf("want ErrBatchParamMismatch, got %v", err)
	}
	// Leftover quantity stays in the book for the next batch.
	r3, err := e.Execute("batch-2", 105)
	if err != nil || r3.Priced {
		t.Fatalf("leftover book should not price, got %+v", r3)
	}
	if b.orders["s2"].Remaining != 4 {
		t.Fatalf("s2 remaining = %d, want 4", b.orders["s2"].Remaining)
	}
}
