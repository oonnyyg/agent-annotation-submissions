package cpm

import (
	"fmt"
	"sort"
)

// CrashResult 是预算内最优压缩方案的完整对照结果。
type CrashResult struct {
	Budget          int
	Selected        []string // 已选活动标识，已排序
	TotalCost       int
	BaseFinish      int
	CrashedFinish   int
	BaseSchedule    *Schedule
	CrashedSchedule *Schedule
	Combinations    int // 枚举的（含预算内与超预算）组合总数
	FeasibleChecked int // 预算内实际计算的组合数
}

// EvaluateCrashing 对全部 2^k 压缩组合做全局搜索（k≤18），
// 不使用按单项收益的贪心。
func EvaluateCrashing(p *Project) (*CrashResult, error) {
	base, err := ComputeSchedule(p, nil)
	if err != nil {
		return nil, err
	}

	type option struct {
		id   string
		dur  int
		cost int
	}
	var options []option
	for i := range p.Activities {
		a := &p.Activities[i]
		if a.Crash != nil {
			options = append(options, option{
				id:   a.ID,
				dur:  a.Crash.ShorterDuration,
				cost: a.Crash.Cost,
			})
		}
	}
	// 按标识排序，保证枚举与平局比较确定。
	sort.Slice(options, func(i, j int) bool { return options[i].id < options[j].id })

	k := len(options)
	if k > 18 {
		return nil, fmt.Errorf("可压缩活动至多 18 项，当前为 %d 项", k)
	}

	best := &CrashResult{
		Budget:       p.Budget,
		BaseSchedule: base,
		BaseFinish:   base.ProjectFinish,
	}

	total := 1 << k
	for mask := 0; mask < total; mask++ {
		cost := 0
		durations := make(map[string]int, k)
		selected := make([]string, 0, k)
		for b := 0; b < k; b++ {
			if mask&(1<<b) != 0 {
				cost += options[b].cost
				if cost > p.Budget {
					break
				}
				durations[options[b].id] = options[b].dur
				selected = append(selected, options[b].id)
			}
		}
		if cost > p.Budget {
			continue // 超预算组合不可行，不参与比较
		}

		sched, err := ComputeSchedule(p, durations)
		if err != nil {
			return nil, err
		}
		best.FeasibleChecked++

		take := false
		switch {
		case best.CrashedSchedule == nil:
			take = true
		case sched.ProjectFinish < best.CrashedFinish:
			take = true
		case sched.ProjectFinish == best.CrashedFinish && cost < best.TotalCost:
			take = true
		case sched.ProjectFinish == best.CrashedFinish && cost == best.TotalCost &&
			lexSequenceLess(selected, best.Selected):
			take = true
		}
		if take {
			best.CrashedSchedule = sched
			best.CrashedFinish = sched.ProjectFinish
			best.TotalCost = cost
			best.Selected = selected
		}
	}

	best.Combinations = total
	if best.CrashedSchedule == nil {
		return nil, fmt.Errorf("未找到可行压缩组合")
	}
	return best, nil
}

// lexSequenceLess 比较两个已排序的标识序列：
// 逐元素字典序，完全相同的前缀下较短者更小。
func lexSequenceLess(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
