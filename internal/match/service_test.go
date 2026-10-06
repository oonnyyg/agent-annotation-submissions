package match

import (
	"encoding/json"
	"testing"
)

func handle(t *testing.T, s *Service, raw string) Response {
	t.Helper()
	return s.Handle(json.RawMessage(raw))
}

func TestIdempotency(t *testing.T) {
	s, err := OpenService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	order := `{"op":"create_order","id":"PO1","supplier":"SUP1","lines":{"1":{"sku":"W","qty":10,"unit_price":100}}}`
	r1 := handle(t, s, order)
	if !r1.OK {
		t.Fatal(r1.Error)
	}
	r2 := handle(t, s, order)
	if !r2.OK {
		t.Fatalf("duplicate submit must return original result, got %v", r2.Error)
	}
	r3 := handle(t, s, `{"op":"create_order","id":"PO1","supplier":"SUP1","lines":{"1":{"sku":"W","qty":99,"unit_price":100}}}`)
	if r3.OK {
		t.Fatal("same id with different content must be rejected")
	}
	// Failed op is also idempotent.
	bad := `{"op":"receive","id":"R1","order_id":"PO1","line":1,"qty":50,"date":"2026-01-01"}`
	f1 := handle(t, s, bad)
	if f1.OK {
		t.Fatal("over-receipt must fail")
	}
	f2 := handle(t, s, bad)
	if f2.OK || f2.Error != f1.Error {
		t.Fatalf("failed op replay mismatch: %+v vs %+v", f1, f2)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	handle(t, s, `{"op":"create_order","id":"PO1","supplier":"SUP1","lines":{"1":{"sku":"W","qty":10,"unit_price":100}}}`)
	handle(t, s, `{"op":"receive","id":"R1","order_id":"PO1","line":1,"qty":7,"date":"2026-01-01"}`)
	inv := handle(t, s, `{"op":"invoice","id":"INV1","supplier":"SUP1","lines":[{"order_id":"PO1","line":1,"qty":5,"unit_price":100}]}`)
	if !inv.OK {
		t.Fatal(inv.Error)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen: state must survive.
	s2, err := OpenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	b := handle(t, s2, `{"op":"balance","id":"q1","order_id":"PO1","line":1}`)
	if !b.OK {
		t.Fatal(b.Error)
	}
	bal := b.Result.(*LineBalance)
	if bal.ReceivedNet != 7 || bal.InvoicedValid != 5 || bal.ReceivedUninvoiced != 2 || bal.Unreceived != 3 {
		t.Fatalf("restored balance = %+v", bal)
	}
	// Idempotency survives restart too.
	dup := handle(t, s2, `{"op":"receive","id":"R1","order_id":"PO1","line":1,"qty":7,"date":"2026-01-01"}`)
	if !dup.OK {
		t.Fatal("duplicate after restart must return original result")
	}
	d := handle(t, s2, `{"op":"invoice_detail","id":"q2","invoice_id":"INV1"}`)
	if !d.OK {
		t.Fatal(d.Error)
	}
	det := d.Result.(*InvoiceDetail)
	if det.Amount != 500 || len(det.Allocs) != 1 || det.Allocs[0].ReceiptID != "R1" {
		t.Fatalf("restored invoice = %+v", det)
	}
}

func TestQueryNotCached(t *testing.T) {
	s, err := OpenService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	handle(t, s, `{"op":"create_order","id":"PO1","supplier":"SUP1","lines":{"1":{"sku":"W","qty":10,"unit_price":100}}}`)
	q := `{"op":"balance","id":"q1","order_id":"PO1","line":1}`
	b1 := handle(t, s, q)
	if b1.Result.(*LineBalance).ReceivedNet != 0 {
		t.Fatal("expected zero received")
	}
	handle(t, s, `{"op":"receive","id":"R1","order_id":"PO1","line":1,"qty":3,"date":"2026-01-01"}`)
	b2 := handle(t, s, q)
	if b2.Result.(*LineBalance).ReceivedNet != 3 {
		t.Fatal("query must reflect current state, not cached snapshot")
	}
}
