package match

import "testing"

func newOrder(t *testing.T, e *Engine) {
	t.Helper()
	err := e.CreateOrder("PO1", "SUP1", map[int]OrderLine{
		1: {SKU: "WIDGET", Qty: 100, UnitPrice: 250},
		2: {SKU: "GADGET", Qty: 50, UnitPrice: 900},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCrossBatchAllocation(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	// Record batches out of date order to prove allocation sorts by date.
	if err := e.Receive("R2", "PO1", 1, 30, "2026-02-01"); err != nil {
		t.Fatal(err)
	}
	if err := e.Receive("R1", "PO1", 1, 50, "2026-01-15"); err != nil {
		t.Fatal(err)
	}
	inv, err := e.MatchInvoice("INV1", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 60, UnitPrice: 250},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Amount != 60*250 {
		t.Fatalf("amount = %d, want %d", inv.Amount, 60*250)
	}
	want := []Allocation{
		{ReceiptID: "R1", OrderID: "PO1", Line: 1, Qty: 50, UnitPrice: 250},
		{ReceiptID: "R2", OrderID: "PO1", Line: 1, Qty: 10, UnitPrice: 250},
	}
	if len(inv.Allocs) != len(want) {
		t.Fatalf("allocs = %+v", inv.Allocs)
	}
	for i, a := range want {
		if inv.Allocs[i] != a {
			t.Fatalf("alloc[%d] = %+v, want %+v", i, inv.Allocs[i], a)
		}
	}
	b, err := e.Balance("PO1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if b.ReceivedNet != 80 || b.InvoicedValid != 60 || b.ReceivedUninvoiced != 20 || b.Unreceived != 20 {
		t.Fatalf("balance = %+v", b)
	}
	if b.PayableCents != 60*250 {
		t.Fatalf("payable = %d", b.PayableCents)
	}
}

func TestLateBackdatedReceiptKeepsOldAllocation(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	if err := e.Receive("R1", "PO1", 1, 10, "2026-03-10"); err != nil {
		t.Fatal(err)
	}
	inv, err := e.MatchInvoice("INV1", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 10, UnitPrice: 250},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Backdated receipt recorded after the match must not change INV1.
	if err := e.Receive("R0", "PO1", 1, 5, "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if len(inv.Allocs) != 1 || inv.Allocs[0].ReceiptID != "R1" || inv.Allocs[0].Qty != 10 {
		t.Fatalf("allocation changed: %+v", inv.Allocs)
	}
}

func TestVoidThenReinvoice(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	if err := e.Receive("R1", "PO1", 1, 40, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.MatchInvoice("INV1", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 40, UnitPrice: 250},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.VoidInvoice("INV1"); err != nil {
		t.Fatal(err)
	}
	if err := e.VoidInvoice("INV1"); err == nil {
		t.Fatal("second void must fail")
	}
	b, _ := e.Balance("PO1", 1)
	if b.InvoicedValid != 0 || b.ReceivedUninvoiced != 40 {
		t.Fatalf("after void: %+v", b)
	}
	// History still queryable.
	d, err := e.InvoiceReport("INV1")
	if err != nil || !d.Voided || len(d.Allocs) != 1 {
		t.Fatalf("voided history: %+v err=%v", d, err)
	}
	// Re-invoice the released quantity.
	inv2, err := e.MatchInvoice("INV2", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 40, UnitPrice: 250},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv2.Allocs[0].ReceiptID != "R1" || inv2.Allocs[0].Qty != 40 {
		t.Fatalf("re-invoice allocs: %+v", inv2.Allocs)
	}
}

func TestReturnRestrictedByFrozenQty(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	if err := e.Receive("R1", "PO1", 1, 30, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.MatchInvoice("INV1", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 25, UnitPrice: 250},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Return("RET1", "R1", 6); err == nil {
		t.Fatal("return of frozen qty must fail")
	}
	if err := e.Return("RET1", "R1", 5); err != nil {
		t.Fatal(err)
	}
	b, _ := e.Balance("PO1", 1)
	if b.ReceivedNet != 25 || b.InvoicedValid != 25 || b.ReceivedUninvoiced != 0 {
		t.Fatalf("balance = %+v", b)
	}
}

func TestOverReceiptRejected(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	if err := e.Receive("R1", "PO1", 1, 100, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	if err := e.Receive("R2", "PO1", 1, 1, "2026-01-11"); err == nil {
		t.Fatal("over-receipt must fail")
	}
	if err := e.Return("RET1", "R1", 10); err != nil {
		t.Fatal(err)
	}
	if err := e.Receive("R2", "PO1", 1, 10, "2026-01-11"); err != nil {
		t.Fatal(err)
	}
}

func TestInvoiceRejectedAtomically(t *testing.T) {
	e := NewEngine()
	newOrder(t, e)
	if err := e.Receive("R1", "PO1", 1, 10, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	if err := e.Receive("R2", "PO1", 2, 50, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	// Wrong price on line 2: whole invoice rejected, line 1 untouched.
	_, err := e.MatchInvoice("INV1", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 10, UnitPrice: 250},
		{OrderID: "PO1", Line: 2, Qty: 5, UnitPrice: 901},
	})
	if err == nil {
		t.Fatal("price mismatch must reject invoice")
	}
	b, _ := e.Balance("PO1", 1)
	if b.InvoicedValid != 0 || b.ReceivedUninvoiced != 10 {
		t.Fatalf("failed invoice changed balance: %+v", b)
	}
	// Mixed suppliers rejected.
	if err := e.CreateOrder("PO2", "SUP2", map[int]OrderLine{
		1: {SKU: "X", Qty: 5, UnitPrice: 100},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Receive("R3", "PO2", 1, 5, "2026-01-10"); err != nil {
		t.Fatal(err)
	}
	_, err = e.MatchInvoice("INV2", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 1, UnitPrice: 250},
		{OrderID: "PO2", Line: 1, Qty: 1, UnitPrice: 100},
	})
	if err == nil {
		t.Fatal("mixed-supplier invoice must be rejected")
	}
	// Insufficient received quantity rejected.
	_, err = e.MatchInvoice("INV3", "SUP1", []InvoiceLine{
		{OrderID: "PO1", Line: 1, Qty: 11, UnitPrice: 250},
	})
	if err == nil {
		t.Fatal("insufficient quantity must be rejected")
	}
}
