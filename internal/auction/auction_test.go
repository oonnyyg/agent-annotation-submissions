package auction

import (
	"errors"
	"testing"
)

func mustPlace(t *testing.T, b *Book, id string, side Side, price, qty int64) *Order {
	t.Helper()
	o, err := b.Place(id, side, price, qty)
	if err != nil {
		t.Fatalf("place %s: %v", id, err)
	}
	return o
}

func TestPlaceValidationAndSeq(t *testing.T) {
	b := NewBook()
	cases := []struct {
		id    string
		side  Side
		price int64
		qty   int64
	}{
		{"", Buy, 100, 1},
		{"bad-side", "hold", 100, 1},
		{"zero-price", Buy, 0, 1},
		{"neg-price", Buy, -5, 1},
		{"zero-qty", Buy, 100, 0},
	}
	for _, c := range cases {
		if _, err := b.Place(c.id, c.side, c.price, c.qty); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("expected invalid request for %+v, got %v", c, err)
		}
	}
	o1 := mustPlace(t, b, "a", Buy, 100, 1)
	o2 := mustPlace(t, b, "b", Sell, 101, 2)
	if o1.Seq != 1 || o2.Seq != 2 {
		t.Fatalf("seq = %d,%d, want 1,2", o1.Seq, o2.Seq)
	}
	if _, err := b.Place("a", Buy, 100, 1); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("reused id must be rejected")
	}
	if err := b.Cancel("unknown"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("cancel unknown must be invalid, got %v", err)
	}
}

func TestCanceledIDNotReusable(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "x", Buy, 100, 1)
	if err := b.Cancel("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Place("x", Buy, 100, 1); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("canceled id must not be reusable")
	}
	if err := b.Cancel("x"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("double cancel must be invalid, got %v", err)
	}
	if len(b.ActiveOrders()) != 0 {
		t.Fatalf("canceled order must not be active")
	}
}

func TestChoosePriceTieBreaks(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "b95", Buy, 95, 10)
	mustPlace(t, b, "b105", Buy, 105, 10)
	mustPlace(t, b, "s95", Sell, 95, 10)
	mustPlace(t, b, "s105", Sell, 105, 10)

	stats := CandidateStats(b.ActiveOrders())
	for _, s := range stats {
		if s.Price != 95 && s.Price != 105 {
			continue
		}
		if s.Tradable != 10 || s.Imbalance != 10 {
			t.Fatalf("price %d: tradable=%d imbalance=%d, want 10/10", s.Price, s.Tradable, s.Imbalance)
		}
	}
	if p, ok := ChoosePrice(stats, 100); !ok || p != 95 {
		t.Fatalf("ref=100 equal distance, want lower 95, got %d ok=%v", p, ok)
	}
	if p, ok := ChoosePrice(stats, 103); !ok || p != 105 {
		t.Fatalf("ref=103 want nearest 105, got %d", p)
	}
}

func TestNoMatchWhenMaxTradableZero(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "b", Buy, 100, 5)
	mustPlace(t, b, "s", Sell, 110, 5)
	e := NewEngine(b)
	res, err := e.Preview(105)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched || res.Price != 0 || len(res.Fills) != 0 {
		t.Fatalf("expected no match, got %+v", res)
	}
}

func TestPreviewDoesNotConsume(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "b", Buy, 100, 10)
	mustPlace(t, b, "s", Sell, 100, 4)
	e := NewEngine(b)
	res, _ := e.Preview(100)
	if !res.Matched || res.Price != 100 || res.TotalQty != 4 {
		t.Fatalf("unexpected preview: %+v", res)
	}
	if b.orders["b"].Filled != 0 || b.orders["s"].Filled != 0 {
		t.Fatalf("preview must not consume orders")
	}
}

func TestExecutePartialFillAndAllocation(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "b100", Buy, 100, 3)
	mustPlace(t, b, "b110", Buy, 110, 3)
	mustPlace(t, b, "s", Sell, 100, 5)
	e := NewEngine(b)
	res, err := e.Execute("batch-1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalQty != 5 || len(res.Fills) != 2 {
		t.Fatalf("want 2 fills totaling 5, got %+v", res.Fills)
	}
	if res.Fills[0].BuyOrderID != "b110" || res.Fills[0].Qty != 3 {
		t.Fatalf("higher-priced buy gets filled first: %+v", res.Fills[0])
	}
	if res.Fills[1].BuyOrderID != "b100" || res.Fills[1].Qty != 2 {
		t.Fatalf("remaining 2 goes to next buy: %+v", res.Fills[1])
	}
	for _, f := range res.Fills {
		if f.Price != 100 {
			t.Fatalf("all fills must be at the single auction price, got %d", f.Price)
		}
	}
	if b.orders["b110"].Remaining() != 0 || b.orders["b100"].Remaining() != 1 || b.orders["s"].Remaining() != 0 {
		t.Fatalf("unexpected remaining: b110=%d b100=%d s=%d",
			b.orders["b110"].Remaining(), b.orders["b100"].Remaining(), b.orders["s"].Remaining())
	}
	if len(b.ActiveOrders()) != 1 || b.ActiveOrders()[0].ID != "b100" {
		t.Fatalf("unfilled remainder must stay on the book")
	}
}

func TestExecuteIdempotentAndBatchConflict(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "b", Buy, 100, 10)
	mustPlace(t, b, "s", Sell, 100, 4)
	e := NewEngine(b)
	first, err := e.Execute("B1", 100)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := e.Execute("B1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if retry != first {
		t.Fatalf("retry must return the stored original result")
	}
	if b.orders["b"].Filled != 4 || b.orders["s"].Filled != 4 {
		t.Fatalf("retry must not trade again")
	}
	if _, err := e.Execute("B1", 101); !errors.Is(err, ErrBatchRejected) {
		t.Fatalf("same batch with different ref must be rejected, got %v", err)
	}
	if _, err := e.Execute("", 100); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty batch id must be invalid")
	}
}

func TestSamePriceFIFOBySeq(t *testing.T) {
	b := NewBook()
	mustPlace(t, b, "s1", Sell, 100, 2)
	mustPlace(t, b, "s2", Sell, 100, 2)
	mustPlace(t, b, "bx", Buy, 100, 3)
	e := NewEngine(b)
	res, _ := e.Execute("B", 100)
	if res.Fills[0].SellOrderID != "s1" || res.Fills[0].Qty != 2 {
		t.Fatalf("same price must allocate by seq, got %+v", res.Fills[0])
	}
	if res.Fills[1].SellOrderID != "s2" || res.Fills[1].Qty != 1 {
		t.Fatalf("second order by seq, got %+v", res.Fills[1])
	}
}
