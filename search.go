package main

import "fmt"

// SearchBest 枚举全部允许的步数组合，返回成本最低的可行方案。
// 同成本先取抑制较少者，再按配置列顺序取字典序最小的步数向量。
func SearchBest(ds *Dataset, hier map[string]*Hierarchy, cfg *Config) (*Evaluation, error) {
	n := len(ds.QICols)
	maxSteps := make([]int, n)
	for i, col := range ds.QICols {
		maxSteps[i] = hier[col].MaxSteps()
	}
	var best *Evaluation
	steps := make([]int, n)
	var walk func(pos int) error
	walk = func(pos int) error {
		if pos == n {
			ev, err := Evaluate(ds, hier, cfg, steps)
			if err != nil {
				return err
			}
			if ev.Feasible && better(ev, best) {
				best = ev
			}
			return nil
		}
		for s := 0; s <= maxSteps[pos]; s++ {
			steps[pos] = s
			if err := walk(pos + 1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(0); err != nil {
		return nil, err
	}
	if best == nil {
		return nil, fmt.Errorf("不存在满足约束的方案：所有步数组合均超出抑制预算或未保留任何记录")
	}
	return best, nil
}

func better(a, b *Evaluation) bool {
	if b == nil {
		return true
	}
	if a.TotalCost != b.TotalCost {
		return a.TotalCost < b.TotalCost
	}
	if a.Suppressed != b.Suppressed {
		return a.Suppressed < b.Suppressed
	}
	for i := range a.Steps {
		if a.Steps[i] != b.Steps[i] {
			return a.Steps[i] < b.Steps[i]
		}
	}
	return false
}
