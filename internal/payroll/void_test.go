package payroll

import "testing"

func TestCloseMonthBlocksNewRecords(t *testing.T) {
	svc := newSvc(t)
	svc.Handle(schemeReq())
	svc.Handle(batchReq("B1", "2026-01", 10))
	if r := svc.Handle(Request{Op: "close_month", Month: "2026-01"}); !r.OK {
		t.Fatal(r.Error)
	}
	if r := svc.Handle(batchReq("B2", "2026-01", 10)); r.OK {
		t.Fatal("closed month must reject new batches")
	}
	if r := svc.Handle(Request{Op: "void_batch", BatchID: "B1", Month: "2026-01"}); r.OK {
		t.Fatal("void into closed month must fail")
	}
	// 同内容重提仍返回原结果
	if r := svc.Handle(batchReq("B1", "2026-01", 10)); !r.OK {
		t.Fatal("idempotent replay should still succeed after close")
	}
}

func TestCrossMonthVoid(t *testing.T) {
	svc := newSvc(t)
	svc.Handle(schemeReq())
	r := svc.Handle(batchReq("B1", "2026-01", 120))
	if !r.OK {
		t.Fatal(r.Error)
	}
	// 撤销前修改权重与价格，验证冲销按原金额而非重算
	upd := schemeReq()
	upd.Version = 2
	upd.Tiers = []Tier{{UpTo: 0, UnitPrice: 1}}
	svc.Handle(upd)
	vr := svc.Handle(Request{Op: "void_batch", BatchID: "B1", Month: "2026-02"})
	if !vr.OK {
		t.Fatal(vr.Error)
	}
	for i, sh := range vr.Shares {
		if sh.Amount != -r.Shares[i].Amount {
			t.Errorf("void share %+v should negate %+v", sh, r.Shares[i])
		}
	}
	if r := svc.Handle(Request{Op: "void_batch", BatchID: "B1", Month: "2026-02"}); r.OK {
		t.Fatal("double void must fail")
	}
	rep, err := svc.queryMonth(Request{Month: "2026-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Additions) != 0 || len(rep.Reversals) != 2 {
		t.Fatalf("report %+v", rep)
	}
	if rep.Reversals[0].SchemeVersion != 1 || rep.Reversals[0].BatchID != "B1" {
		t.Errorf("reversal must trace original batch and scheme version: %+v", rep.Reversals[0])
	}
	for _, n := range rep.Net {
		if n.Amount >= 0 {
			t.Errorf("net in void month should be negative: %+v", n)
		}
	}
	// 一月净额不受撤销影响
	rep1, _ := svc.queryMonth(Request{Month: "2026-01"})
	total := 0
	for _, n := range rep1.Net {
		total += n.Amount
	}
	if total != r.Total {
		t.Errorf("jan net %d != batch total %d", total, r.Total)
	}
}

func TestReopenConsistency(t *testing.T) {
	dir := t.TempDir()
	svc, _ := Open(dir)
	svc.Handle(schemeReq())
	svc.Handle(batchReq("B1", "2026-01", 120))
	svc.Handle(Request{Op: "void_batch", BatchID: "B1", Month: "2026-02"})
	before, _ := svc.queryMonth(Request{Month: "2026-02"})

	svc2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := svc2.queryMonth(Request{Month: "2026-02"})
	if len(before.Additions) != len(after.Additions) ||
		len(before.Reversals) != len(after.Reversals) ||
		len(before.Net) != len(after.Net) {
		t.Fatal("reopen changed report shape")
	}
	for i := range before.Net {
		if before.Net[i] != after.Net[i] {
			t.Errorf("net mismatch after reopen: %+v vs %+v", before.Net[i], after.Net[i])
		}
	}
	// 重开后重复撤销仍被拒绝
	if r := svc2.Handle(Request{Op: "void_batch", BatchID: "B1", Month: "2026-03"}); r.OK {
		t.Fatal("voided flag must persist across reopen")
	}
}
