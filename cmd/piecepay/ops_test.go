package main

import "testing"

func mustOK(t *testing.T, st *State, req Request) Response {
	t.Helper()
	resp, _ := dispatch(st, req)
	if !resp.OK {
		t.Fatalf("操作 %+v 失败: %s", req, resp.Error)
	}
	return resp
}

func mustFail(t *testing.T, st *State, req Request) {
	t.Helper()
	if resp, _ := dispatch(st, req); resp.OK {
		t.Fatalf("操作 %+v 应被拒绝", req)
	}
}

func schemeReq(id string) Request {
	return Request{Op: "register_scheme", SchemeID: id, Tiers: tierSet}
}

func batchReq(id, month string, qty int64, members map[string]int64) Request {
	return Request{Op: "register_batch", BatchID: id, SchemeID: "s1",
		Month: month, Qty: qty, Members: members}
}

func TestSchemeUpdateKeepsBatchVersion(t *testing.T) {
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	r1 := mustOK(t, st, batchReq("b1", "2026-09", 100, map[string]int64{"a": 1}))
	if r1.SchemeVersion != 1 || r1.TotalCents != 1000 {
		t.Fatalf("首批应使用版本1: %+v", r1)
	}
	// 更新方案后，新批次用新版本，旧批次快照不变。
	mustOK(t, st, Request{Op: "register_scheme", SchemeID: "s1",
		Tiers: []Tier{{UpTo: nil, RateCents: 50}}})
	r2 := mustOK(t, st, batchReq("b2", "2026-09", 100, map[string]int64{"a": 1}))
	if r2.SchemeVersion != 2 || r2.TotalCents != 5000 {
		t.Fatalf("更新后应使用版本2: %+v", r2)
	}
	if st.Batches["b1"].TotalCents != 1000 || st.Batches["b1"].SchemeVersion != 1 {
		t.Fatalf("方案更新不应改变已有分配: %+v", st.Batches["b1"])
	}
}

func TestBatchIdempotency(t *testing.T) {
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	req := batchReq("b1", "2026-09", 100, map[string]int64{"a": 1, "b": 1})
	first := mustOK(t, st, req)
	again := mustOK(t, st, req)
	if !again.Duplicate {
		t.Fatal("相同内容重提应标记 duplicate")
	}
	if again.TotalCents != first.TotalCents {
		t.Fatal("重提应返回原结果")
	}
	// 内容不同则拒绝，且编号不被新内容占用。
	mustFail(t, st, batchReq("b1", "2026-09", 101, map[string]int64{"a": 1, "b": 1}))
	if st.Batches["b1"].Qty != 100 {
		t.Fatal("拒绝后原批次不应被修改")
	}
}

func TestFailedRegisterLeavesNoTrace(t *testing.T) {
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	// 权重非法导致登记失败。
	mustFail(t, st, batchReq("b1", "2026-09", 100, map[string]int64{"a": 0}))
	if _, ok := st.Batches["b1"]; ok {
		t.Fatal("失败登记不应留下批次")
	}
	if _, ok := st.Requests["b1"]; ok {
		t.Fatal("失败登记不应占用编号")
	}
	// 方案不存在也应失败。
	mustFail(t, st, Request{Op: "register_batch", BatchID: "b2", SchemeID: "nope",
		Month: "2026-09", Qty: 1, Members: map[string]int64{"a": 1}})
}

func TestCloseMonthBlocksNewRecords(t *testing.T) {
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	mustOK(t, st, batchReq("b1", "2026-09", 10, map[string]int64{"a": 1}))
	mustOK(t, st, Request{Op: "close_month", Month: "2026-09"})
	mustFail(t, st, Request{Op: "close_month", Month: "2026-09"})
	mustFail(t, st, batchReq("b2", "2026-09", 10, map[string]int64{"a": 1}))
	mustFail(t, st, Request{Op: "void_batch", BatchID: "b1", Month: "2026-09"})
	// 其他月份不受影响。
	mustOK(t, st, batchReq("b3", "2026-10", 10, map[string]int64{"a": 1}))
}

func TestCrossMonthVoid(t *testing.T) {
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	members := map[string]int64{"a01": 1, "a02": 1, "a03": 1}
	mustOK(t, st, batchReq("b1", "2026-09", 10, members)) // 100 分
	mustOK(t, st, Request{Op: "close_month", Month: "2026-09"})
	// 原月已封账，撤销记入 10 月。
	v := mustOK(t, st, Request{Op: "void_batch", BatchID: "b1", Month: "2026-10"})
	if v.TotalCents != -100 || v.Shares["a01"] != -34 {
		t.Fatalf("冲销应按原份额取负: %+v", v)
	}
	// 不可重复撤销。
	mustFail(t, st, Request{Op: "void_batch", BatchID: "b1", Month: "2026-10"})
	// 方案更新与权重变化不影响已发生的冲销。
	mustOK(t, st, Request{Op: "register_scheme", SchemeID: "s1",
		Tiers: []Tier{{UpTo: nil, RateCents: 99}}})

	sep := mustOK(t, st, Request{Op: "query_month", Month: "2026-09"}).Report
	if sep.Added != 100 || sep.Reversed != 0 || sep.Net != 100 {
		t.Fatalf("9 月只应有新增: %+v", sep)
	}
	oct := mustOK(t, st, Request{Op: "query_month", Month: "2026-10"}).Report
	if oct.Added != 0 || oct.Reversed != -100 || oct.Net != -100 {
		t.Fatalf("10 月只应有冲销: %+v", oct)
	}
	if oct.Members["a01"].Net != -34 || oct.Members["a02"].Net != -33 {
		t.Fatalf("逐人冲销应等于原份额取负: %+v", oct.Members)
	}
	// 冲销流水可追查原批次与方案版本。
	if len(oct.Entries) != 1 || oct.Entries[0].Kind != "reversal" ||
		oct.Entries[0].BatchID != "b1" || oct.Entries[0].SchemeVersion != 1 ||
		oct.Entries[0].BatchMonth != "2026-09" {
		t.Fatalf("冲销流水缺少追查信息: %+v", oct.Entries)
	}
}

func TestReopenConsistency(t *testing.T) {
	dir := t.TempDir()
	st := newState()
	mustOK(t, st, schemeReq("s1"))
	mustOK(t, st, batchReq("b1", "2026-09", 250, map[string]int64{"a": 2, "b": 3}))
	mustOK(t, st, Request{Op: "void_batch", BatchID: "b1", Month: "2026-10"})
	if err := saveState(dir, st); err != nil {
		t.Fatal(err)
	}
	reopened, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	q1, _ := dispatch(st, Request{Op: "query_month", Month: "2026-10"})
	q2, _ := dispatch(reopened, Request{Op: "query_month", Month: "2026-10"})
	if q1.Report.Net != q2.Report.Net || q1.Report.Reversed != q2.Report.Reversed {
		t.Fatalf("重开后查询结果不一致: %+v vs %+v", q1.Report, q2.Report)
	}
	// 重开后幂等重提仍返回原结果。
	dup, _ := dispatch(reopened, batchReq("b1", "2026-09", 250, map[string]int64{"a": 2, "b": 3}))
	if !dup.OK || !dup.Duplicate {
		t.Fatalf("重开后重提应命中原记录: %+v", dup)
	}
}
