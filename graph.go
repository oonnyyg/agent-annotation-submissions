package main

import (
	"fmt"
	"sort"
	"strings"
)

// closure 返回指定目标的依赖闭包（含目标自身）。
func (m *Manifest) closure(want []string) (map[string]bool, error) {
	set := map[string]bool{}
	var visit func(string) error
	visit = func(n string) error {
		if set[n] {
			return nil
		}
		t, ok := m.Targets[n]
		if !ok {
			return fmt.Errorf("未知目标 %q", n)
		}
		set[n] = true
		for _, d := range t.Deps {
			if err := visit(d); err != nil {
				return err
			}
		}
		return nil
	}
	for _, n := range want {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// topoOrder 对给定目标集合做拓扑排序。
// 无依赖先后关系的目标按名称字典序排列，保证顺序确定。
// 发现环时返回错误，且在执行任何命令之前调用。
func (m *Manifest) topoOrder(set map[string]bool) ([]string, error) {
	indeg := map[string]int{}
	for n := range set {
		indeg[n] = 0
	}
	for n := range set {
		for _, d := range m.Targets[n].Deps {
			if set[d] {
				indeg[n]++
			}
		}
	}
	var ready []string
	for n, d := range indeg {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		for other := range set {
			for _, d := range m.Targets[other].Deps {
				if d == n {
					indeg[other]--
					if indeg[other] == 0 {
						ready = append(ready, other)
						sort.Strings(ready)
					}
				}
			}
		}
	}
	if len(order) != len(set) {
		var inCycle []string
		for n, d := range indeg {
			if d > 0 {
				inCycle = append(inCycle, n)
			}
		}
		sort.Strings(inCycle)
		return nil, fmt.Errorf("依赖图存在环，涉及目标: %s", strings.Join(inCycle, ", "))
	}
	return order, nil
}
