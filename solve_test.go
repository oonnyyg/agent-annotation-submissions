package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func trip(id, from, to, dep, arr string) Trip {
	return Trip{ID: id, From: from, To: to, Depart: mustTime(dep), Arrive: mustTime(arr)}
}

func testProblem(trips []Trip, turnaroundMin int64, arcs ...DeadheadArc) *Problem {
	return buildProblem(trips, &Network{TurnaroundMin: turnaroundMin, Arcs: arcs})
}

// verifyChains 逐链核对：每班恰好一次、无时刻冲突、里程一致。
func verifyChains(t *testing.T, p *Problem, sol *Solution) {
	t.Helper()
	seen := map[int]int{}
	totalTrips := 0
	var totalDist int64
	for _, chain := range sol.Chains {
		for pos, idx := range chain {
			seen[idx]++
			totalTrips++
			if pos+1 < len(chain) {
				c, ok := sol.Conns[[2]int{idx, chain[pos+1]}]
				if !ok {
					t.Fatalf("链内接续 (%d,%d) 缺少接续信息", idx, chain[pos+1])
				}
				a, b := p.Trips[idx], p.Trips[chain[pos+1]]
				ready := a.Arrive.Add(p.Turnaround).Add(c.Travel)
				if ready.After(b.Depart) {
					t.Fatalf("链内时刻冲突: %s 后无法接 %s", a.ID, b.ID)
				}
				totalDist += c.Dist
			}
		}
	}
	if totalTrips != len(p.Trips) {
		t.Fatalf("链中班次数 %d != 输入班次数 %d", totalTrips, len(p.Trips))
	}
	for idx, cnt := range seen {
		if cnt != 1 {
			t.Fatalf("班次 %s 出现 %d 次", p.Trips[idx].ID, cnt)
		}
	}
	if len(sol.Chains) != sol.Vehicles {
		t.Fatalf("链数 %d != 用车数 %d", len(sol.Chains), sol.Vehicles)
	}
	if totalDist != sol.TotalDist {
		t.Fatalf("链内里程合计 %d != 总空驶里程 %d", totalDist, sol.TotalDist)
	}
}

// TestGlobalNotGreedy 构造局部贪心会出错的场景：
// A(08:00-09:00@X) 可接 B(09:30@X) 或 C(09:20@Y)；
// 若贪心给 A 选里程最小的接续，会破坏全局最少车辆/里程。
func TestGlobalNotGreedy(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "X", "S2", "2026-01-01T09:30:00Z", "2026-01-01T10:00:00Z"),
		trip("C", "Y", "S3", "2026-01-01T09:20:00Z", "2026-01-01T09:50:00Z"),
		trip("D", "S3", "S1", "2026-01-01T10:10:00Z", "2026-01-01T10:40:00Z"),
	}
	// A->B 同站 0km；A->C 需 X->Y 空驶 5km；C->D 同站 0km。
	// 全局最优：A->B 与 C->D 两条接续（2 车，5km 都不用）；
	// 若 A->C 则 B、D 落单成 3 车。这里验证车辆数优先。
	p := testProblem(trips, 10, DeadheadArc{From: "X", To: "Y", TimeMin: 5, Distance: 5})
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 2 {
		t.Fatalf("期望 2 车，实际 %d", sol.Vehicles)
	}
	if sol.TotalDist != 0 {
		t.Fatalf("期望总空驶 0 km，实际 %d", sol.TotalDist)
	}
}

// TestMinVehiclesPriority 验证先最少车辆、再最小里程的优先级。
func TestMinVehiclesPriority(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "Y", "S2", "2026-01-01T09:30:00Z", "2026-01-01T10:00:00Z"),
		trip("C", "Z", "S3", "2026-01-01T09:30:00Z", "2026-01-01T10:00:00Z"),
	}
	// A 可接 B（5km）或 C（1km），但 B、C 同时出发只能接一个 => 2 车。
	p := testProblem(trips, 10,
		DeadheadArc{From: "X", To: "Y", TimeMin: 10, Distance: 5},
		DeadheadArc{From: "X", To: "Z", TimeMin: 10, Distance: 1},
	)
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 2 {
		t.Fatalf("期望 2 车，实际 %d", sol.Vehicles)
	}
	if sol.TotalDist != 1 {
		t.Fatalf("期望最小空驶 1 km，实际 %d", sol.TotalDist)
	}
}

// TestBoundaryExactArrival 恰好在后班出发前到达（ready == depart）允许接续。
func TestBoundaryExactArrival(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "Y", "S2", "2026-01-01T09:25:00Z", "2026-01-01T10:00:00Z"),
	}
	// 整备 10 分 + 空驶 15 分 = 09:25 恰好到达。
	p := testProblem(trips, 10, DeadheadArc{From: "X", To: "Y", TimeMin: 15, Distance: 7})
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 1 {
		t.Fatalf("恰好到达应允许接续（1 车），实际 %d", sol.Vehicles)
	}
	if sol.TotalDist != 7 {
		t.Fatalf("期望空驶 7 km，实际 %d", sol.TotalDist)
	}
}

// TestBoundaryOneMinuteLate 差一分钟则不可接续。
func TestBoundaryOneMinuteLate(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "Y", "S2", "2026-01-01T09:24:00Z", "2026-01-01T10:00:00Z"),
	}
	p := testProblem(trips, 10, DeadheadArc{From: "X", To: "Y", TimeMin: 15, Distance: 7})
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 2 {
		t.Fatalf("差一分钟不可接续（2 车），实际 %d", sol.Vehicles)
	}
}

// TestUnreachableArc 未提供的点对不可达，不得绕行第三站。
func TestUnreachableArc(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "Y", "S2", "2026-01-01T10:00:00Z", "2026-01-01T11:00:00Z"),
	}
	// 只有 X->Z 与 Z->Y，没有 X->Y：不允许经 Z 绕行。
	p := testProblem(trips, 10,
		DeadheadArc{From: "X", To: "Z", TimeMin: 5, Distance: 1},
		DeadheadArc{From: "Z", To: "Y", TimeMin: 5, Distance: 1},
	)
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 2 {
		t.Fatalf("未提供 X->Y 弧，不可接续（2 车），实际 %d", sol.Vehicles)
	}
}

// TestSameStationZero 同站空驶为零。
func TestSameStationZero(t *testing.T) {
	trips := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "X", "S2", "2026-01-01T09:10:00Z", "2026-01-01T10:00:00Z"),
	}
	p := testProblem(trips, 10)
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 1 || sol.TotalDist != 0 {
		t.Fatalf("同站接续应为 1 车 0 km，实际 %d 车 %d km", sol.Vehicles, sol.TotalDist)
	}
}

// TestDeterministicUnderShuffle 输入重排后结果一致。
func TestDeterministicUnderShuffle(t *testing.T) {
	base := []Trip{
		trip("A", "S1", "X", "2026-01-01T08:00:00Z", "2026-01-01T09:00:00Z"),
		trip("B", "X", "S2", "2026-01-01T09:30:00Z", "2026-01-01T10:00:00Z"),
		trip("C", "Y", "S3", "2026-01-01T09:20:00Z", "2026-01-01T09:50:00Z"),
		trip("D", "S3", "S1", "2026-01-01T10:10:00Z", "2026-01-01T10:40:00Z"),
	}
	arcs := []DeadheadArc{{From: "X", To: "Y", TimeMin: 5, Distance: 5}}
	ref := solve(testProblem(base, 10, arcs...))
	rev := []Trip{base[3], base[2], base[1], base[0]}
	got := solve(testProblem(rev, 10, arcs...))
	if got.Vehicles != ref.Vehicles || got.TotalDist != ref.TotalDist {
		t.Fatalf("重排后结果不一致: (%d,%d) vs (%d,%d)",
			got.Vehicles, got.TotalDist, ref.Vehicles, ref.TotalDist)
	}
	if fmt.Sprint(got.Chains) != fmt.Sprint(ref.Chains) {
		t.Fatalf("重排后链结构不一致: %v vs %v", got.Chains, ref.Chains)
	}
}

// TestInvalidInputs 非法输入必须报错而非输出部分计划。
func TestInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		csv  string
	}{
		{"零持续", "id,from,to,depart,arrive\nA,S1,S2,2026-01-01T08:00:00Z,2026-01-01T08:00:00Z\n"},
		{"负持续", "id,from,to,depart,arrive\nA,S1,S2,2026-01-01T09:00:00Z,2026-01-01T08:00:00Z\n"},
		{"重复编号", "id,from,to,depart,arrive\nA,S1,S2,2026-01-01T08:00:00Z,2026-01-01T09:00:00Z\nA,S1,S2,2026-01-01T10:00:00Z,2026-01-01T11:00:00Z\n"},
		{"坏时刻", "id,from,to,depart,arrive\nA,S1,S2,not-a-time,2026-01-01T09:00:00Z\n"},
		{"空站点", "id,from,to,depart,arrive\nA,,S2,2026-01-01T08:00:00Z,2026-01-01T09:00:00Z\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseTripsCSV(strings.NewReader(tc.csv)); err == nil {
				t.Fatalf("应拒绝非法输入: %s", tc.name)
			}
		})
	}
	if _, err := parseNetworkJSON(strings.NewReader(`{"turnaround_min":-5,"arcs":[]}`)); err == nil {
		t.Fatal("应拒绝负整备时长")
	}
	if _, err := parseNetworkJSON(strings.NewReader(`{"turnaround_min":0,"arcs":[{"from":"A","to":"B","time_min":-1,"distance_km":3}]}`)); err == nil {
		t.Fatal("应拒绝负空驶时间")
	}
	if _, err := parseNetworkJSON(strings.NewReader(`{"turnaround_min":0,"arcs":[{"from":"A","to":"A","time_min":0,"distance_km":0}]}`)); err == nil {
		t.Fatal("应拒绝同站空驶弧")
	}
}

// TestTooManyTrips 超过 120 班应拒绝。
func TestTooManyTrips(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("id,from,to,depart,arrive\n")
	for i := 0; i < 121; i++ {
		fmt.Fprintf(&sb, "T%03d,S1,S2,2026-01-01T08:00:00Z,2026-01-01T09:00:00Z\n", i)
	}
	if _, err := parseTripsCSV(strings.NewReader(sb.String())); err == nil {
		t.Fatal("应拒绝超过 120 班")
	}
}

// TestScale120 一百二十班规模可解且结果自洽。
func TestScale120(t *testing.T) {
	var trips []Trip
	for i := 0; i < 120; i++ {
		dep := mustTime("2026-01-01T06:00:00Z").Add(time.Duration(i*10) * time.Minute)
		arr := dep.Add(5 * time.Minute)
		trips = append(trips, Trip{
			ID: fmt.Sprintf("T%03d", i), From: "S1", To: "S1",
			Depart: dep, Arrive: arr,
		})
	}
	p := testProblem(trips, 5)
	sol := solve(p)
	verifyChains(t, p, sol)
	if sol.Vehicles != 1 {
		t.Fatalf("同站间隔 10 分钟应串成 1 车，实际 %d", sol.Vehicles)
	}
}
