package match

import (
	"errors"
	"testing"
)

func mustOrder(t *testing.T, s *State, orderID, lineID, supplier string, qty, price int64) {
	t.Helper()
	if err := s.CreateOrder(Line{OrderID: orderID, LineID: lineID, Supplier: supplier, Item: "widget", Qty: qty, UnitPrice: price}); err != nil {
		t.Fatalf("create order: %v", err)
	}
}

func mustReceive(t *testing.T, s *State, id, orderID, lineID, date string, qty int64) {
	t.Helper()
	if err := s.Receive(Receipt{ID: id, OrderID: orderID, LineID: lineID, Date: date, Qty: qty}); err != nil {
		t.Fatalf("receive %s: %v", id, err)
	}
}

func TestCrossBatchAllocationFIFO(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 100, 500)
	mustReceive(t, s, "R2", "PO1", "L1", "2026-02-01", 30)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 50)

	inv, err := s.PostInvoice("INV1", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 60, UnitPrice: 500}})
	if err != nil {
		t.Fatalf("post invoice: %v", err)
	}
	// FIFO by date: R1 fully (50), then R2 partially (10).
	want := []Allocation{{OrderID: "PO1", LineID: "L1", ReceiptID: "R1", Qty: 50}, {OrderID: "PO1", LineID: "L1", ReceiptID: "R2", Qty: 10}}
	if len(inv.Allocs) != 2 || inv.Allocs[0] != want[0] || inv.Allocs[1] != want[1] {
		t.Fatalf("allocs = %+v, want %+v", inv.Allocs, want)
	}
	if inv.Amount != 60*500 {
		t.Fatalf("amount = %d", inv.Amount)
	}
	b, _ := s.Balance("PO1", "L1")
	if b.Received != 80 || b.Invoiced != 60 || b.Uninvoiced != 20 || b.Unreceived != 20 || b.Payable != 30000 {
		t.Fatalf("balance = %+v", b)
	}
}

func TestInvoiceInsufficientRejectedAtomically(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 10, 100)
	mustOrder(t, s, "PO1", "L2", "SUP", 10, 200)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 10)
	mustReceive(t, s, "R2", "PO1", "L2", "2026-01-01", 3)

	_, err := s.PostInvoice("INV1", []InvoiceLine{
		{OrderID: "PO1", LineID: "L1", Qty: 10, UnitPrice: 100},
		{OrderID: "PO1", LineID: "L2", Qty: 5, UnitPrice: 200}, // only 3 received
	})
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("err = %v", err)
	}
	// Nothing consumed on L1 either.
	if got := s.Receipts["R1"].Invoiced; got != 0 {
		t.Fatalf("R1 invoiced = %d, want 0", got)
	}
	if _, ok := s.Invoices["INV1"]; ok {
		t.Fatal("rejected invoice must not be recorded")
	}
}

func TestInvoiceSupplierAndPriceValidation(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUPA", 10, 100)
	mustOrder(t, s, "PO2", "L1", "SUPB", 10, 100)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 10)
	mustReceive(t, s, "R2", "PO2", "L1", "2026-01-01", 10)

	_, err := s.PostInvoice("INV1", []InvoiceLine{
		{OrderID: "PO1", LineID: "L1", Qty: 1, UnitPrice: 100},
		{OrderID: "PO2", LineID: "L1", Qty: 1, UnitPrice: 100},
	})
	if !errors.Is(err, ErrSupplier) {
		t.Fatalf("mixed suppliers: err = %v", err)
	}
	_, err = s.PostInvoice("INV2", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 1, UnitPrice: 101}})
	if !errors.Is(err, ErrPrice) {
		t.Fatalf("price mismatch: err = %v", err)
	}
}

func TestOverReceiveRejected(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 10, 100)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 8)
	if err := s.Receive(Receipt{ID: "R2", OrderID: "PO1", LineID: "L1", Date: "2026-01-02", Qty: 3}); !errors.Is(err, ErrOverReceive) {
		t.Fatalf("err = %v", err)
	}
}

func TestReturnLimitedByInvoiced(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 10, 100)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 10)
	if _, err := s.PostInvoice("INV1", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 7, UnitPrice: 100}}); err != nil {
		t.Fatal(err)
	}
	// Only 3 uninvoiced remain on the batch.
	if err := s.Return("RET1", "R1", 4); !errors.Is(err, ErrOverReturn) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Return("RET1", "R1", 3); err != nil {
		t.Fatalf("return 3: %v", err)
	}
	if err := s.Return("RET2", "R1", 1); !errors.Is(err, ErrOverReturn) {
		t.Fatalf("err = %v", err)
	}
}

func TestVoidReleasesOnceAndReinvoice(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 10, 100)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-01-01", 6)
	mustReceive(t, s, "R2", "PO1", "L1", "2026-01-02", 4)
	inv, err := s.PostInvoice("INV1", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 8, UnitPrice: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.VoidInvoice("INV1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VoidInvoice("INV1"); err == nil {
		t.Fatal("second void must fail")
	}
	// History preserved with original allocations.
	got := s.Invoices["INV1"]
	if !got.Voided || len(got.Allocs) != len(inv.Allocs) || got.Amount != 800 {
		t.Fatalf("voided invoice = %+v", got)
	}
	b, _ := s.Balance("PO1", "L1")
	if b.Invoiced != 0 || b.Uninvoiced != 10 || b.Payable != 0 {
		t.Fatalf("balance after void = %+v", b)
	}
	// Re-invoice under a new id consumes the batches again.
	if _, err := s.PostInvoice("INV2", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 8, UnitPrice: 100}}); err != nil {
		t.Fatalf("re-invoice: %v", err)
	}
	b, _ = s.Balance("PO1", "L1")
	if b.Invoiced != 8 || b.Payable != 800 {
		t.Fatalf("balance after re-invoice = %+v", b)
	}
}

func TestBackdatedReceiptKeepsOldAllocations(t *testing.T) {
	s := NewState()
	mustOrder(t, s, "PO1", "L1", "SUP", 10, 100)
	mustReceive(t, s, "R1", "PO1", "L1", "2026-02-01", 5)
	inv, err := s.PostInvoice("INV1", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 5, UnitPrice: 100}})
	if err != nil {
		t.Fatal(err)
	}
	// Later, an earlier-dated receipt is back-entered.
	mustReceive(t, s, "R0", "PO1", "L1", "2026-01-01", 5)
	if inv.Allocs[0].ReceiptID != "R1" {
		t.Fatalf("old allocation changed: %+v", inv.Allocs)
	}
	// New invoice allocates the earlier-dated batch first.
	inv2, err := s.PostInvoice("INV2", []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 5, UnitPrice: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if inv2.Allocs[0].ReceiptID != "R0" {
		t.Fatalf("new allocs = %+v", inv2.Allocs)
	}
}
