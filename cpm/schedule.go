package cpm

import (
	"fmt"
	"sort"
)

// ActivityResult 是单个活动的时间参数与关键标记。
type ActivityResult struct {
	ID          string
	Duration    int
	EarlyStart  int
	EarlyFinish int
	LateStart   int
	LateFinish  int
	TotalFloat  int
	Critical    bool
}

// Schedule 是一次完整 CPM 计算的结果。
type Schedule struct {
	Results       map[string]*ActivityResult
	Order         []string // 拓扑顺序
	ProjectFinish int
	CriticalIDs   []string
	CriticalPath  []string
}

type node struct {
	activity *Activity
	preds    []int
	succs    []int
}

type network struct {
	nodes []*node
	index map[string]int
}

func buildNetwork(p *Project) (*network, error) {
	nw := &network{
		nodes: make([]*node, len(p.Activities)),
		index: make(map[string]int, len(p.Activities)),
	}
	for i := range p.Activities {
		nw.nodes[i] = &node{activity: &p.Activities[i]}
		nw.index[p.Activities[i].ID] = i
	}
	for i := range p.Activities {
		for _, dep := range p.Activities[i].Predecessors {
			j := nw.index[dep]
			nw.nodes[i].preds = append(nw.nodes[i].preds, j)
			nw.nodes[j].succs = append(nw.nodes[j].succs, i)
		}
	}
	return nw, nil
}

// topoOrder 基于 Kahn 算法，同层按标识排序保证结果确定；
// 无法排空全部节点时报告环。
func (nw *network) topoOrder() ([]int, error) {
	indeg := make([]int, len(nw.nodes))
	for i, n := range nw.nodes {
		indeg[i] = len(n.preds)
	}
	ready := []int{}
	for i, d := range indeg {
		if d == 0 {
			ready = append(ready, i)
		}
	}
	var order []int
	for len(ready) > 0 {
		sort.Slice(ready, func(a, b int) bool {
			return nw.nodes[ready[a]].activity.ID <
				nw.nodes[ready[b]].activity.ID
		})
		cur := ready[0]
		ready = ready[1:]
		order = append(order, cur)
		for _, s := range nw.nodes[cur].succs {
			indeg[s]--
			if indeg[s] == 0 {
				ready = append(ready, s)
			}
		}
	}
	if len(order) != len(nw.nodes) {
		remaining := []string{}
		for i, d := range indeg {
			if d > 0 {
				remaining = append(remaining, nw.nodes[i].activity.ID)
			}
		}
		sort.Strings(remaining)
		return nil, fmt.Errorf("依赖图存在环，涉及活动: %v", remaining)
	}
	return order, nil
}

// ComputeSchedule 对给定工期（压缩映射）执行完整 CPM 计算。
func ComputeSchedule(p *Project, durations map[string]int) (*Schedule, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	nw, err := buildNetwork(p)
	if err != nil {
		return nil, err
	}
	order, err := nw.topoOrder()
	if err != nil {
		return nil, err
	}

	es := make([]int, len(nw.nodes))
	ef := make([]int, len(nw.nodes))
	finish := 0
	for _, i := range order {
		n := nw.nodes[i]
		start := 0
		for _, j := range n.preds {
			if ef[j] > start {
				start = ef[j]
			}
		}
		dur := n.activity.Duration
		if d, ok := durations[n.activity.ID]; ok {
			dur = d
		}
		es[i] = start
		ef[i] = start + dur
		if ef[i] > finish {
			finish = ef[i]
		}
	}

	ls := make([]int, len(nw.nodes))
	lf := make([]int, len(nw.nodes))
	for k := len(order) - 1; k >= 0; k-- {
		i := order[k]
		n := nw.nodes[i]
		dur := n.activity.Duration
		if d, ok := durations[n.activity.ID]; ok {
			dur = d
		}
		if len(n.succs) == 0 {
			lf[i] = finish // 所有分支共用同一项目终点
		} else {
			lf[i] = ls[n.succs[0]]
			for _, j := range n.succs[1:] {
				if ls[j] < lf[i] {
					lf[i] = ls[j]
				}
			}
		}
		ls[i] = lf[i] - dur
	}

	s := &Schedule{
		Results:       make(map[string]*ActivityResult, len(nw.nodes)),
		ProjectFinish: finish,
	}
	for _, i := range order {
		n := nw.nodes[i]
		tf := ls[i] - es[i]
		s.Results[n.activity.ID] = &ActivityResult{
			ID:          n.activity.ID,
			Duration:    ef[i] - es[i],
			EarlyStart:  es[i],
			EarlyFinish: ef[i],
			LateStart:   ls[i],
			LateFinish:  lf[i],
			TotalFloat:  tf,
			Critical:    tf == 0,
		}
		s.Order = append(s.Order, n.activity.ID)
		if tf == 0 {
			s.CriticalIDs = append(s.CriticalIDs, n.activity.ID)
		}
	}
	sort.Strings(s.CriticalIDs)

	path := criticalPath(nw, order, es, ef, finish)
	s.CriticalPath = path
	return s, nil
}

// criticalPath 在“零总时差 + 边界时间相接”的子图上，
// 以动态规划求标识序列字典序最小的完整路径（起点 ES=0，终点 EF=完工日）。
func criticalPath(nw *network, order, es, ef []int, finish int) []string {
	// best[i] 是以 i 结尾的字典序最小关键路径；nil 表示 i 不可达关键路径。
	best := make([][]string, len(nw.nodes))

	// 零总时差 ⇔ 该节点位于某条从第 0 天到完工日的紧边界链上。
	// 以 pred.EF == succ.ES 且 succ.EF - succ.ES == 原 dur 已天然成立，
	// 还需节点自身松弛为 0；通过可达标记推导：
	// canStart[i]: i 可从第 0 天的源点经紧边界到达，且 es[i]==路径累计。
	canStart := make([]bool, len(nw.nodes))
	canEnd := make([]bool, len(nw.nodes))
	for _, i := range order {
		n := nw.nodes[i]
		if es[i] == 0 && len(n.preds) == 0 {
			canStart[i] = true
		}
		for _, j := range n.preds {
			if canStart[j] && ef[j] == es[i] {
				canStart[i] = true
			}
		}
	}
	for k := len(order) - 1; k >= 0; k-- {
		i := order[k]
		n := nw.nodes[i]
		if ef[i] == finish && len(n.succs) == 0 {
			canEnd[i] = true
		}
		for _, j := range n.succs {
			if canEnd[j] && ef[i] == es[j] {
				canEnd[i] = true
			}
		}
	}

	lexLess := func(a, b []string) bool {
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	}

	for _, i := range order {
		n := nw.nodes[i]
		if !(canStart[i] && canEnd[i]) {
			continue
		}
		var chosen []string
		for _, j := range n.preds {
			if best[j] == nil || ef[j] != es[i] {
				continue
			}
			cand := append(append([]string{}, best[j]...), n.activity.ID)
			if chosen == nil || lexLess(cand, chosen) {
				chosen = cand
			}
		}
		if chosen == nil && es[i] == 0 && len(n.preds) == 0 {
			chosen = []string{n.activity.ID}
		}
		best[i] = chosen
	}

	var result []string
	for _, i := range order {
		if ef[i] != finish || len(nw.nodes[i].succs) != 0 || best[i] == nil {
			continue
		}
		if result == nil || lexLess(best[i], result) {
			result = best[i]
		}
	}
	return result
}
