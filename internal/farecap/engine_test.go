package farecap

import (
	"encoding/json"
	"strings"
	"testing"
)

func testConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfig(strings.NewReader(`{
		"utcOffsetMinutes": 480,
		"transferWindowMinutes": 90,
		"fares": {"1": 100, "2": 150, "3": 200, "4": 250},
		"dailyCapCents": 400,
		"weeklyCapCents": 1000
	}`))
	if err != nil {
		t.Fatalf("配置加载失败: %v", err)
	}
	return cfg
}

func ride(id, card, board, alight string, from, to int) Ride {
	return Ride{ID: id, Card: card, Board: board, Alight: alight, From: from, To: to}
}

func chargesOf(t *testing.T, e *Engine, card string) []Charge {
	t.Helper()
	ch, _ := e.Query(card)
	return ch
}

func amounts(ch []Charge) []int {
	out := make([]int, len(ch))
	for i, c := range ch {
		out[i] = c.AmountCents
	}
	return out
}

func TestTransferGroupAndWindow(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	// 2026-10-05 是周一。三趟行程，第二趟并入首趟组，第三趟距首趟 100 分钟 > 90，开新组。
	if r := e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1)); r.Status != "ok" || r.Charged != 100 {
		t.Fatalf("r1: %+v", r)
	}
	// 并入组：区间扩到 1..2，组票价 150，增量 50。
	if r := e.AddRide(ride("r2", "c1", "2026-10-05T08:50:00", "2026-10-05T09:10:00", 2, 2)); r.Status != "ok" || r.Charged != 50 {
		t.Fatalf("r2: %+v", r)
	}
	// 窗口不随换乘延长：距首趟 100 分钟，虽距 r2 仅 30 分钟，仍开新组。
	if r := e.AddRide(ride("r3", "c1", "2026-10-05T09:40:00", "2026-10-05T10:00:00", 1, 1)); r.Status != "ok" || r.Charged != 100 {
		t.Fatalf("r3: %+v", r)
	}
}

func TestDailyAndWeeklyCap(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	// 日封顶 400：100 + 50 + 100 + 200(新组 1..3) = 450 -> 最后一笔限到 150。
	e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1))
	e.AddRide(ride("r2", "c1", "2026-10-05T08:50:00", "2026-10-05T09:10:00", 2, 2))
	e.AddRide(ride("r3", "c1", "2026-10-05T09:40:00", "2026-10-05T10:00:00", 1, 1))
	if r := e.AddRide(ride("r4", "c1", "2026-10-05T12:00:00", "2026-10-05T12:30:00", 1, 3)); r.Charged != 150 {
		t.Fatalf("r4 应被日封顶限制为 150: %+v", r)
	}
	// 当天后续行程免费。
	if r := e.AddRide(ride("r5", "c1", "2026-10-05T18:00:00", "2026-10-05T18:30:00", 1, 1)); r.Charged != 0 {
		t.Fatalf("r5 应为 0: %+v", r)
	}
	// 跨天（周二）重新计费，日封顶再充 400，周累计 800。
	if r := e.AddRide(ride("r6", "c1", "2026-10-06T08:00:00", "2026-10-06T08:30:00", 1, 4)); r.Charged != 250 {
		t.Fatalf("r6: %+v", r)
	}
	if r := e.AddRide(ride("r7", "c1", "2026-10-06T12:00:00", "2026-10-06T12:30:00", 1, 4)); r.Charged != 150 {
		t.Fatalf("r7 应被日封顶限制为 150: %+v", r)
	}
	// 周封顶 1000：周三前两趟各 100 后周累计 1000，第三趟免费。
	e.AddRide(ride("r8", "c1", "2026-10-07T08:00:00", "2026-10-07T08:30:00", 1, 1))
	e.AddRide(ride("r9", "c1", "2026-10-07T12:00:00", "2026-10-07T12:30:00", 1, 1))
	if r := e.AddRide(ride("r10", "c1", "2026-10-07T18:00:00", "2026-10-07T18:30:00", 1, 1)); r.Charged != 0 {
		t.Fatalf("r10 应被周封顶限制为 0: %+v", r)
	}
}

func TestServiceDayBoundary(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	// 凌晨 3 点仍属前一服务日；4 点后是新服务日，必开新组。
	e.AddRide(ride("r1", "c1", "2026-10-06T03:00:00", "2026-10-06T03:20:00", 1, 1))
	if r := e.AddRide(ride("r2", "c1", "2026-10-06T04:10:00", "2026-10-06T04:30:00", 1, 1)); r.Charged != 100 {
		t.Fatalf("跨服务日必须开新组: %+v", r)
	}
	ch := chargesOf(t, e, "c1")
	if ch[0].ServiceDay != "2026-10-05" || ch[1].ServiceDay != "2026-10-06" {
		t.Fatalf("服务日划分错误: %+v", ch)
	}
}

func TestDuplicateConflictOverlap(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	r1 := ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1)
	e.AddRide(r1)
	if r := e.AddRide(r1); r.Status != "duplicate" {
		t.Fatalf("同内容重提应幂等: %+v", r)
	}
	bad := r1
	bad.To = 2
	if r := e.AddRide(bad); r.Status != "rejected" {
		t.Fatalf("内容冲突应拒绝: %+v", r)
	}
	if r := e.AddRide(ride("r2", "c1", "2026-10-05T08:10:00", "2026-10-05T08:30:00", 1, 1)); r.Status != "rejected" {
		t.Fatalf("重叠行程应拒绝: %+v", r)
	}
	// 旧记录未被改变。
	ch := chargesOf(t, e, "c1")
	if len(ch) != 1 || ch[0].AmountCents != 100 {
		t.Fatalf("旧记录被改变: %+v", ch)
	}
}

func TestVoidRecompute(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1))
	e.AddRide(ride("r2", "c1", "2026-10-05T08:50:00", "2026-10-05T09:10:00", 3, 3))
	// r2 并入 r1 组，区间 1..3，组票价 200，r2 增量 100。
	if got := amounts(chargesOf(t, e, "c1")); got[0] != 100 || got[1] != 100 {
		t.Fatalf("撤销前扣费: %v", got)
	}
	// 撤销 r1 后完整重算：r2 自成一组，按宽度 1 收 100，而非直接退回旧扣费。
	if r := e.AddVoid(VoidRecord{ID: "v1", Card: "c1", Target: "r1"}); r.Status != "ok" {
		t.Fatalf("撤销失败: %+v", r)
	}
	ch := chargesOf(t, e, "c1")
	if len(ch) != 1 || ch[0].RideID != "r2" || ch[0].AmountCents != 100 {
		t.Fatalf("撤销后重算错误: %+v", ch)
	}
	// 撤销历史保留，重复撤销幂等，重复目标拒绝。
	if r := e.AddVoid(VoidRecord{ID: "v1", Card: "c1", Target: "r1"}); r.Status != "duplicate" {
		t.Fatalf("重复撤销应幂等: %+v", r)
	}
	if r := e.AddVoid(VoidRecord{ID: "v2", Card: "c1", Target: "r1"}); r.Status != "rejected" {
		t.Fatalf("已撤销记录再撤销应拒绝: %+v", r)
	}
	if len(e.State().Cards["c1"].Voids) != 1 {
		t.Fatalf("撤销历史未保留")
	}
}

func TestCapThenExpandSpan(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	// 日封顶 400。三组各 200（1..3），第三组限到 0；随后第三组扩大区间到 1..4，
	// 组票价 250，增量 50，但日剩余额度为 0，仍扣 0。
	e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 3))
	e.AddRide(ride("r2", "c1", "2026-10-05T10:00:00", "2026-10-05T10:20:00", 1, 3))
	e.AddRide(ride("r3", "c1", "2026-10-05T12:00:00", "2026-10-05T12:20:00", 1, 3))
	if r := e.AddRide(ride("r4", "c1", "2026-10-05T12:30:00", "2026-10-05T12:50:00", 4, 4)); r.Charged != 0 {
		t.Fatalf("封顶后扩大区间不应再扣费: %+v", r)
	}
	ch := chargesOf(t, e, "c1")
	if got := amounts(ch); got[0] != 200 || got[1] != 200 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("扣费流水错误: %v", got)
	}
}

func TestCrossWeekVoid(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	// 周日（属于 10-05 起算的服务周）与下周一（新服务周）各一趟。
	e.AddRide(ride("r1", "c1", "2026-10-11T08:00:00", "2026-10-11T08:30:00", 1, 4))
	e.AddRide(ride("r2", "c1", "2026-10-12T08:00:00", "2026-10-12T08:30:00", 1, 4))
	ch := chargesOf(t, e, "c1")
	if ch[0].ServiceWeek != "2026-10-05" || ch[1].ServiceWeek != "2026-10-12" {
		t.Fatalf("服务周划分错误: %+v", ch)
	}
	// 撤销上周行程，不影响本周扣费。
	e.AddVoid(VoidRecord{ID: "v1", Card: "c1", Target: "r1"})
	ch = chargesOf(t, e, "c1")
	if len(ch) != 1 || ch[0].RideID != "r2" || ch[0].AmountCents != 250 {
		t.Fatalf("跨周撤销重算错误: %+v", ch)
	}
}

func TestMissingFare(t *testing.T) {
	e := NewEngine(testConfig(t), nil)
	if r := e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 5)); r.Status != "rejected" {
		t.Fatalf("缺少票价应拒绝: %+v", r)
	}
	if len(e.State().Cards["c1"].Rides) != 0 {
		t.Fatalf("拒绝的记录不应落库")
	}
}

func TestRestartConsistency(t *testing.T) {
	cfg := testConfig(t)
	e := NewEngine(cfg, nil)
	e.AddRide(ride("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1))
	e.AddRide(ride("r2", "c1", "2026-10-05T08:50:00", "2026-10-05T09:10:00", 2, 2))
	e.AddVoid(VoidRecord{ID: "v1", Card: "c1", Target: "r1"})
	before := chargesOf(t, e, "c1")

	data, err := json.Marshal(e.State())
	if err != nil {
		t.Fatal(err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	e2 := NewEngine(cfg, &st)
	after := chargesOf(t, e2, "c1")
	if len(before) != len(after) {
		t.Fatalf("重开后流水不一致: %v vs %v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("重开后流水不一致: %v vs %v", before, after)
		}
	}
	// 重开后继续导入，结果与一次性导入一致。
	e.AddRide(ride("r3", "c1", "2026-10-05T12:00:00", "2026-10-05T12:20:00", 1, 1))
	e2.AddRide(ride("r3", "c1", "2026-10-05T12:00:00", "2026-10-05T12:20:00", 1, 1))
	b1, b2 := chargesOf(t, e, "c1"), chargesOf(t, e2, "c1")
	if len(b1) != len(b2) {
		t.Fatalf("重开后结果不一致")
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("重开后结果不一致: %v vs %v", b1, b2)
		}
	}
}
