package desk

import "testing"

// 清点记录：同编号同内容幂等，冲突拒绝，不同编号数量相加，可撤销。
func TestCountRecordRules(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 0})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A"}})
	rec := CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 3}
	must(t, st, "count_record", rec)
	must(t, st, "count_record", rec) // 重复导入幂等
	mustErr(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 4}, "冲突")
	must(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r2", SKU: "s1", Qty: 2})
	must(t, st, "count_void", CountVoidEvent{TakeID: "T1", Loc: "A", RecID: "r2"})
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "A"})
	diffs := must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"}).([]DiffLine)
	if len(diffs) != 1 || diffs[0].Counted != 3 {
		t.Fatalf("撤销后实盘应为 3, 实际 %+v", diffs)
	}
	mustErr(t, st, "count_void", CountVoidEvent{TakeID: "T1", Loc: "A", RecID: "r1"}, "已封存")
}

// 未全部确认完成不能封存；重开封存盘点保留记录与明细。
func TestSealRequiresCompletionAndReopen(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 5})
	must(t, st, "inbound", MoveEvent{Loc: "B", SKU: "s1", Qty: 1})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A", "B"}})
	mustErr(t, st, "take_start", TakeStartEvent{TakeID: "T2", Locations: []string{"A"}}, "未结束盘点")
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "A"})
	mustErr(t, st, "take_seal", TakeRefEvent{TakeID: "T1"}, "尚未确认")
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "B"})
	must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"})
	mustErr(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 5}, "已封存")
	must(t, st, "take_reopen", TakeRefEvent{TakeID: "T1"})
	tk := st.State().Takes["T1"]
	if tk.Status != TakeOpen || len(tk.Diffs) == 0 {
		t.Fatalf("重开应回到 open 且保留封存明细, 实际 %+v", tk)
	}
	must(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 5})
	must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"})
	must(t, st, "take_post", TakeRefEvent{TakeID: "T1"})
	// 已入账后库位可参与新盘点
	must(t, st, "take_start", TakeStartEvent{TakeID: "T2", Locations: []string{"A"}})
}

// 重启后状态从 JSONL 日志完整恢复。
func TestPersistenceReplay(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 9})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A"}})
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st2.State().Stock["A"]["s1"]; got != 9 {
		t.Fatalf("重放后库存应为 9, 实际 %d", got)
	}
	if st2.State().Takes["T1"].Baseline["A"]["s1"] != 9 {
		t.Fatalf("重放后基线应为 9")
	}
}
