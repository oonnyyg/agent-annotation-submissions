package auction

import "testing"

func mustAdd(t *testing.T, b *Book, id string, side Side, price, qty int64) {
	t.Helper()
	if _, err := b.AddOrder(id, side, price, qty); err != nil {
		t.Fatalf("AddOrder(%s): %v", id, err)
	}
}

func TestInvalidRequestsKeepBook(t *testing.T) {
	b := NewBook()
	for _, tc := range []struct {
		id, side   string
		price, qty int64
	}{
		{"", "buy", 100, 1},
		{"x", "hold", 100, 1},
		{"x", "buy", 0, 1},
		{"x", "buy", -5, 1},
		{"x", "buy", 100, 0},
	} {
		if _, err := b.AddOrder(tc.id, Side(tc.side), tc.price, tc.qty); err == nil {
			t.Fatalf("expected error for %+v", tc)
		}
	}
	if len(b.liveOrders()) != 0 {
		t.Fatal("invalid requests mutated the book")
	}
}

func TestSeqAndCancelIDReuse(t *testing.T) {
	b := NewBook()
	mustAdd(t, b, "a", Buy, 100, 1)
	o, _ := b.AddOrder("b", Sell, 100, 1)
	if o.Seq != 2 {
		t.Fatalf("seq = %d, want 2", o.Seq)
	}
	if err := b.CancelOrder("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddOrder("a", Buy, 100, 1); err == nil {
		t.Fatal("cancelled id must not be reusable")
	}
	if err := b.CancelOrder("a"); err == nil {
		t.Fatal("double cancel must fail")
	}
}

func TestPricingTieBreaks(t *testing.T) {
	// Prices 100 and 101 both match 10 with imbalance 0; reference 102
	// is closer to 101.
	b := NewBook()
	mustAdd(t, b, "b1", Buy, 101, 10)
	mustAdd(t, b, "s1", Sell, 100, 10)
	p, v, ok := b.SelectPrice(102)
	if !ok || p != 101 || v != 10 {
		t.Fatalf("got (%d,%d,%v), want (101,10,true)", p, v, ok)
	}

	// Same volume and imbalance, reference equidistant: lower price wins.
	b2 := NewBook()
	mustAdd(t, b2, "b1", Buy, 102, 10)
	mustAdd(t, b2, "s1", Sell, 99, 10)
	if p, _, _ := b2.SelectPrice(100); p != 99 {
		t.Fatalf("equidistant tie: got %d, want 99", p)
	}

	// Imbalance beats reference distance.
	b3 := NewBook()
	mustAdd(t, b3, "b1", Buy, 100, 10)
	mustAdd(t, b3, "s1", Sell, 90, 10)
	mustAdd(t, b3, "s2", Sell, 100, 5)
	if p, _, _ := b3.SelectPrice(100); p != 90 {
		t.Fatalf("imbalance tie-break: got %d, want 90", p)
	}
}

func TestNoPriceWhenVolumeZero(t *testing.T) {
	b := NewBook()
	mustAdd(t, b, "b1", Buy, 90, 10)
	mustAdd(t, b, "s1", Sell, 100, 10)
	if _, _, ok := b.SelectPrice(95); ok {
		t.Fatal("crossed book absent: must not force a match")
	}
	r, err := b.Preview(95)
	if err != nil || r.Priced || r.Volume != 0 || len(r.Trades) != 0 {
		t.Fatalf("preview = %+v, err %v", r, err)
	}
}
