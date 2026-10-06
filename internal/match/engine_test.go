package match

import (
	"testing"
)

func openTemp(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	e, err := OpenEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	return e, dir
}

func TestIdempotentResubmission(t *testing.T) {
	e, _ := openTemp(t)
	defer e.Close()

	op := &Op{Op: "create_order", OrderID: "PO1", LineID: "L1", Supplier: "SUP", Item: "w", Qty: 10, UnitPrice: 100}
	r1 := e.Execute(op)
	r2 := e.Execute(op)
	if !r1.OK || !r2.OK {
		t.Fatalf("duplicate should return original ok result: %+v %+v", r1, r2)
	}
	// Same id, different content -> rejected.
	bad := *op
	bad.Qty = 11
	if r := e.Execute(&bad); r.OK {
		t.Fatal("conflicting resubmission must be rejected")
	}
}

func TestFailureNotRecordedAndRetryable(t *testing.T) {
	e, _ := openTemp(t)
	defer e.Close()

	e.Execute(&Op{Op: "create_order", OrderID: "PO1", LineID: "L1", Supplier: "SUP", Item: "w", Qty: 10, UnitPrice: 100})
	inv := &Op{Op: "invoice", InvoiceID: "INV1", Lines: []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 5, UnitPrice: 100}}}
	if r := e.Execute(inv); r.OK {
		t.Fatal("invoice without receipts must fail")
	}
	// After a receipt arrives, the same invoice op succeeds (not blocked by idempotency).
	e.Execute(&Op{Op: "receive", ReceiptID: "R1", OrderID: "PO1", LineID: "L1", Date: "2026-01-01", Qty: 5})
	if r := e.Execute(inv); !r.OK {
		t.Fatalf("retry after receipt should succeed: %+v", r)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	e, dir := openTemp(t)
	e.Execute(&Op{Op: "create_order", OrderID: "PO1", LineID: "L1", Supplier: "SUP", Item: "w", Qty: 10, UnitPrice: 100})
	e.Execute(&Op{Op: "receive", ReceiptID: "R1", OrderID: "PO1", LineID: "L1", Date: "2026-01-01", Qty: 6})
	e.Execute(&Op{Op: "invoice", InvoiceID: "INV1", Lines: []InvoiceLine{{OrderID: "PO1", LineID: "L1", Qty: 4, UnitPrice: 100}}})
	e.Close()

	e2, err := OpenEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	r := e2.Execute(&Op{Op: "query_balance", OrderID: "PO1", LineID: "L1"})
	if !r.OK || r.Balance.Invoiced != 4 || r.Balance.Uninvoiced != 2 || r.Balance.Unreceived != 4 || r.Balance.Payable != 400 {
		t.Fatalf("balance after reopen = %+v", r)
	}
	// Idempotency survives restart too.
	dup := e2.Execute(&Op{Op: "receive", ReceiptID: "R1", OrderID: "PO1", LineID: "L1", Date: "2026-01-01", Qty: 6})
	if !dup.OK {
		t.Fatalf("duplicate receive after reopen should replay original result: %+v", dup)
	}
	b := e2.Execute(&Op{Op: "query_balance", OrderID: "PO1", LineID: "L1"}).Balance
	if b.Received != 6 {
		t.Fatalf("duplicate receive mutated state: %+v", b)
	}
	// Voided invoice history survives restart.
	e2.Execute(&Op{Op: "void_invoice", InvoiceID: "INV1"})
	e2.Close()
	e3, err := OpenEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e3.Close()
	qr := e3.Execute(&Op{Op: "query_invoice", InvoiceID: "INV1"})
	if !qr.OK || !qr.Invoice.Voided || len(qr.Invoice.Allocs) != 1 || qr.Invoice.Allocs[0].Qty != 4 {
		t.Fatalf("voided invoice history = %+v", qr)
	}
	if bal := e3.Execute(&Op{Op: "query_balance", OrderID: "PO1", LineID: "L1"}).Balance; bal.Invoiced != 0 || bal.Payable != 0 {
		t.Fatalf("balance after void+reopen = %+v", bal)
	}
}
