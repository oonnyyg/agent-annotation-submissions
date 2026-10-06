package desk

import (
	"strings"
	"testing"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func must(t *testing.T, st *Store, op string, data any) any {
	t.Helper()
	res, err := st.Append(op, data)
	if err != nil {
		t.Fatalf("%s 失败: %v", op, err)
	}
	return res
}

func mustErr(t *testing.T, st *Store, op string, data any, want string) {
	t.Helper()
	_, err := st.Append(op, data)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("%s 应失败且包含 %q, 实际: %v", op, want, err)
	}
}

func stock(t *testing.T, st *Store, loc, sku string) int {
	t.Helper()
	return st.State().Stock[loc][sku]
}

func TestOutboundRejectsNegative(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 5})
	mustErr(t, st, "outbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 6}, "负")
	mustErr(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: -1}, "非负")
	if got := stock(t, st, "A", "s1"); got != 5 {
		t.Fatalf("库存应为 5, 实际 %d", got)
	}
}

// 盘中出库：冻结基线不受后续出入库影响，入账只加差异。
func TestOutboundDuringTake(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 10})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A"}})
	must(t, st, "outbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 4}) // 盘中出库，当前 6
	must(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 6})
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "A"})
	res := must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"})
	diffs := res.([]DiffLine)
	if len(diffs) != 1 || diffs[0].Baseline != 10 || diffs[0].Counted != 6 || diffs[0].Diff != -4 {
		t.Fatalf("差异应为 基线10 实盘6 差-4, 实际 %+v", diffs)
	}
	must(t, st, "take_post", TakeRefEvent{TakeID: "T1"})
	if got := stock(t, st, "A", "s1"); got != 2 { // 6 + (-4)
		t.Fatalf("入账后库存应为 2, 实际 %d", got)
	}
}

// 漏盘归零：基线商品未清点按零实盘处理。
func TestMissedCountTreatedAsZero(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 3})
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s2", Qty: 7})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A"}})
	must(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 3})
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "A"})
	res := must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"}).([]DiffLine)
	if len(res) != 2 {
		t.Fatalf("应有 2 行差异, 实际 %d", len(res))
	}
	must(t, st, "take_post", TakeRefEvent{TakeID: "T1"})
	if got := stock(t, st, "A", "s2"); got != 0 {
		t.Fatalf("漏盘商品应归零, 实际 %d", got)
	}
}

// 重试入账：第一次因负库存整次拒绝，补足后重试成功，再次入账幂等。
func TestPostRetryAndIdempotent(t *testing.T) {
	st := openStore(t)
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 10})
	must(t, st, "take_start", TakeStartEvent{TakeID: "T1", Locations: []string{"A"}})
	must(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r1", SKU: "s1", Qty: 2})
	must(t, st, "count_complete", CountCompleteEvent{TakeID: "T1", Loc: "A"})
	must(t, st, "take_seal", TakeRefEvent{TakeID: "T1"})            // 差异 -8
	must(t, st, "outbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 5}) // 当前 5
	mustErr(t, st, "take_post", TakeRefEvent{TakeID: "T1"}, "变负")   // 5-8<0 整次拒绝
	if got := stock(t, st, "A", "s1"); got != 5 {
		t.Fatalf("拒绝后库存不变, 实际 %d", got)
	}
	must(t, st, "inbound", MoveEvent{Loc: "A", SKU: "s1", Qty: 10}) // 当前 15
	must(t, st, "take_post", TakeRefEvent{TakeID: "T1"})            // 重试成功 -> 7
	if got := stock(t, st, "A", "s1"); got != 7 {
		t.Fatalf("入账后应为 7, 实际 %d", got)
	}
	res := must(t, st, "take_post", TakeRefEvent{TakeID: "T1"}).([]DiffLine) // 幂等
	if len(res) != 1 || res[0].Diff != -8 {
		t.Fatalf("重复入账应返回原差异, 实际 %+v", res)
	}
	if got := stock(t, st, "A", "s1"); got != 7 {
		t.Fatalf("重复入账不应改变库存, 实际 %d", got)
	}
	mustErr(t, st, "count_record", CountRecordEvent{TakeID: "T1", Loc: "A", RecID: "r2", SKU: "s1", Qty: 1}, "已入账")
	mustErr(t, st, "take_reopen", TakeRefEvent{TakeID: "T1"}, "已入账")
}
