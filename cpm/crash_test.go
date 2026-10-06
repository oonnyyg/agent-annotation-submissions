package cpm

import "testing"

// 平局：压缩 A 或压缩 B 都能把完工日从 9 提前到 7，成本相同，
// 应取排序后标识序列字典序更小的 ["A"]。
func TestCrashingBudgetTie(t *testing.T) {
	p := &Project{
		Budget: 5,
		Activities: []Activity{
			{ID: "A", Duration: 3, Crash: &CrashOption{ShorterDuration: 1, Cost: 5}},
			{ID: "B", Duration: 3, Predecessors: []string{"A"},
				Crash: &CrashOption{ShorterDuration: 1, Cost: 5}},
			{ID: "C", Duration: 3, Predecessors: []string{"B"}},
		},
	}
	r, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	if r.CrashedFinish != 7 {
		t.Fatalf("最优完工日 = %d, 期望 7", r.CrashedFinish)
	}
	if !equalSlice(r.Selected, []string{"A"}) {
		t.Fatalf("已选 = %v, 期望 [A]", r.Selected)
	}
	if r.TotalCost != 5 {
		t.Fatalf("成本 = %d, 期望 5", r.TotalCost)
	}
}

// 并行关键分支：只压其中一条不会提前整体完工，必须两条都压；
// 预算只够一条时保持基准方案（不花钱，成本平局取更短标识序列——空集最小）。
func TestCrashingParallelBranches(t *testing.T) {
	mk := func(budget int) *Project {
		return &Project{
			Budget: budget,
			Activities: []Activity{
				{ID: "A", Duration: 5, Crash: &CrashOption{ShorterDuration: 3, Cost: 4}},
				{ID: "B", Duration: 5, Crash: &CrashOption{ShorterDuration: 3, Cost: 4}},
				{ID: "C", Duration: 2, Predecessors: []string{"A", "B"}},
			},
		}
	}

	r1, err := EvaluateCrashing(mk(4))
	if err != nil {
		t.Fatal(err)
	}
	if r1.CrashedFinish != 7 || len(r1.Selected) != 0 {
		t.Fatalf("预算 4: 完工 %d 已选 %v, 期望 7 与空集", r1.CrashedFinish, r1.Selected)
	}

	r2, err := EvaluateCrashing(mk(8))
	if err != nil {
		t.Fatal(err)
	}
	if r2.CrashedFinish != 5 {
		t.Fatalf("预算 8: 完工 %d, 期望 5", r2.CrashedFinish)
	}
	if !equalSlice(r2.Selected, []string{"A", "B"}) {
		t.Fatalf("已选 = %v, 期望 [A B]", r2.Selected)
	}
	// 关键路径可能转移：压缩后两条分支仍等长，选字典序最小路径。
	if !equalSlice(r2.CrashedSchedule.CriticalPath, []string{"A", "C"}) {
		t.Fatalf("压缩后关键路径 = %v", r2.CrashedSchedule.CriticalPath)
	}
}

// 全局选择而非单项贪心：压缩非关键活动不会改变完工日，
// 即使更便宜也不应选择；同完工日下低成本的空集胜出。
func TestCrashingNonCriticalNoBenefit(t *testing.T) {
	p := &Project{
		Budget: 10,
		Activities: []Activity{
			{ID: "X", Duration: 5},
			{ID: "Y", Duration: 2, Crash: &CrashOption{ShorterDuration: 0, Cost: 1}},
		},
	}
	r, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	if r.CrashedFinish != 5 {
		t.Fatalf("完工日 = %d, 期望 5", r.CrashedFinish)
	}
	if len(r.Selected) != 0 || r.TotalCost != 0 {
		t.Fatalf("不应选择无收益压缩, 已选 %v 成本 %d", r.Selected, r.TotalCost)
	}
}

// 预算超限时排除昂贵组合：最优压缩负担不起时，
// 花钱压非关键分支无收益，平局规则选择零成本空集。
func TestCrashingBudgetExcludesBest(t *testing.T) {
	p := &Project{
		Budget: 6,
		Activities: []Activity{
			{ID: "L", Duration: 10, Crash: &CrashOption{ShorterDuration: 1, Cost: 10}},
			{ID: "S", Duration: 4, Crash: &CrashOption{ShorterDuration: 0, Cost: 6}},
			{ID: "E", Duration: 0, Predecessors: []string{"L", "S"}},
		},
	}
	r, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	// L 压缩不可负担；S 是非关键分支，压它不缩短整体，故维持 10 天且不花钱。
	if r.CrashedFinish != 10 || len(r.Selected) != 0 {
		t.Fatalf("完工 %d 已选 %v, 期望 10 与空集", r.CrashedFinish, r.Selected)
	}

	// 预算足够时压 L：L 缩到 1 天后 S(4) 反成关键分支，完工日 4 天。
	p.Budget = 10
	r2, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	if r2.CrashedFinish != 4 || !equalSlice(r2.Selected, []string{"L"}) {
		t.Fatalf("完工 %d 已选 %v, 期望 4 与 [L]", r2.CrashedFinish, r2.Selected)
	}
	if !equalSlice(r2.CrashedSchedule.CriticalPath, []string{"S", "E"}) {
		t.Fatalf("关键路径应转移到 S 分支, 实际 %v",
			r2.CrashedSchedule.CriticalPath)
	}
}
