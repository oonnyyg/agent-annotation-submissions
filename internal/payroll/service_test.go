package payroll

import "testing"

func newSvc(t *testing.T) *Service {
	t.Helper()
	svc, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func schemeReq() Request {
	return Request{
		Op: "register_scheme", SchemeID: "S1", Version: 1,
		Tiers: []Tier{{UpTo: 100, UnitPrice: 50}, {UpTo: 0, UnitPrice: 80}},
	}
}

func batchReq(id, month string, qty int) Request {
	return Request{
		Op: "register_batch", BatchID: id, SchemeID: "S1",
		Month: month, Quantity: qty,
		Members: []MemberWeight{{EmpID: "E1", Weight: 1}, {EmpID: "E2", Weight: 1}},
	}
}

func TestBatchIdempotentAndConflict(t *testing.T) {
	svc := newSvc(t)
	if r := svc.Handle(schemeReq()); !r.OK {
		t.Fatal(r.Error)
	}
	r1 := svc.Handle(batchReq("B1", "2026-01", 120))
	if !r1.OK || r1.Total != 5000+20*80 {
		t.Fatalf("unexpected %+v", r1)
	}
	r2 := svc.Handle(batchReq("B1", "2026-01", 120))
	if !r2.OK || r2.Total != r1.Total {
		t.Fatalf("replay should return original result: %+v", r2)
	}
	diff := batchReq("B1", "2026-01", 121)
	if r := svc.Handle(diff); r.OK {
		t.Fatal("different content with same id must be rejected")
	}
	// 失败后编号不被占用以外的状态污染：条目数仍为 2
	if len(svc.state.Entries) != 2 {
		t.Fatalf("partial entries left: %d", len(svc.state.Entries))
	}
}

func TestFailedBatchLeavesNoPartialState(t *testing.T) {
	svc := newSvc(t)
	svc.Handle(schemeReq())
	bad := batchReq("B9", "2026-01", 10)
	bad.Members = append(bad.Members, MemberWeight{EmpID: "E1", Weight: 1})
	if r := svc.Handle(bad); r.OK {
		t.Fatal("duplicate member should fail")
	}
	if _, ok := svc.state.Batches["B9"]; ok {
		t.Fatal("failed batch occupied the id")
	}
	if len(svc.state.Entries) != 0 {
		t.Fatal("failed batch left entries")
	}
	// 修正后可以正常使用该编号
	good := batchReq("B9", "2026-01", 10)
	if r := svc.Handle(good); !r.OK {
		t.Fatalf("id should be reusable after failure: %s", r.Error)
	}
}

func TestSchemeUpdateKeepsOldBatches(t *testing.T) {
	svc := newSvc(t)
	svc.Handle(schemeReq())
	svc.Handle(batchReq("B1", "2026-01", 50))
	upd := schemeReq()
	upd.Version = 2
	upd.Tiers = []Tier{{UpTo: 0, UnitPrice: 999}}
	if r := svc.Handle(upd); !r.OK {
		t.Fatal(r.Error)
	}
	svc.Handle(batchReq("B2", "2026-01", 50))
	rep, err := svc.queryMonth(Request{Month: "2026-01"})
	if err != nil {
		t.Fatal(err)
	}
	var b1, b2 int
	for _, e := range rep.Additions {
		if e.BatchID == "B1" {
			b1 += e.Amount
			if e.SchemeVersion != 1 {
				t.Errorf("B1 should keep scheme version 1, got %d", e.SchemeVersion)
			}
		}
		if e.BatchID == "B2" {
			b2 += e.Amount
		}
	}
	if b1 != 50*50 || b2 != 50*999 {
		t.Errorf("b1=%d b2=%d", b1, b2)
	}
}
