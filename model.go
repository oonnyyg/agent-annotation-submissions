package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// CrashOption describes an optional crashing plan for one activity.
type CrashOption struct {
	Duration int `json:"duration"` // shorter non-negative duration in days
	Cost     int `json:"cost"`     // positive integer cost
}

// Activity is one node of the activity-on-node network.
type Activity struct {
	ID           string       `json:"id"`
	Duration     int          `json:"duration"`
	Predecessors []string     `json:"predecessors"`
	Crash        *CrashOption `json:"crash,omitempty"`
}

// Project is the top-level JSON input document.
type Project struct {
	Budget     int        `json:"budget"`
	Activities []Activity `json:"activities"`
}

func loadProject(r io.Reader) (*Project, error) {
	var p Project
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("解析输入 JSON 失败: %w", err)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Project) validate() error {
	if len(p.Activities) == 0 {
		return fmt.Errorf("活动集合不能为空")
	}
	if p.Budget < 0 {
		return fmt.Errorf("预算必须为非负整数, 得到 %d", p.Budget)
	}
	seen := make(map[string]bool, len(p.Activities))
	for _, a := range p.Activities {
		if a.ID == "" {
			return fmt.Errorf("活动标识不能为空")
		}
		if seen[a.ID] {
			return fmt.Errorf("活动标识 %q 重复", a.ID)
		}
		seen[a.ID] = true
		if a.Duration < 0 {
			return fmt.Errorf("活动 %q 工期必须为非负整数, 得到 %d", a.ID, a.Duration)
		}
		if a.Crash != nil {
			if a.Crash.Duration < 0 {
				return fmt.Errorf("活动 %q 压缩工期必须为非负整数", a.ID)
			}
			if a.Crash.Duration >= a.Duration {
				return fmt.Errorf("活动 %q 压缩工期 %d 必须短于原工期 %d", a.ID, a.Crash.Duration, a.Duration)
			}
			if a.Crash.Cost <= 0 {
				return fmt.Errorf("活动 %q 压缩成本必须为正整数, 得到 %d", a.ID, a.Crash.Cost)
			}
		}
	}
	for _, a := range p.Activities {
		for _, dep := range a.Predecessors {
			if !seen[dep] {
				return fmt.Errorf("活动 %q 依赖未知活动 %q", a.ID, dep)
			}
			if dep == a.ID {
				return fmt.Errorf("活动 %q 不能依赖自身", a.ID)
			}
		}
	}
	if err := p.checkAcyclic(); err != nil {
		return err
	}
	return nil
}

// checkAcyclic runs Kahn's algorithm and reports a cycle if one exists.
func (p *Project) checkAcyclic() error {
	indeg := make(map[string]int, len(p.Activities))
	succ := make(map[string][]string, len(p.Activities))
	for _, a := range p.Activities {
		if _, ok := indeg[a.ID]; !ok {
			indeg[a.ID] = 0
		}
		for _, dep := range a.Predecessors {
			succ[dep] = append(succ[dep], a.ID)
			indeg[a.ID]++
		}
	}
	var queue []string
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, nxt := range succ[id] {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				queue = append(queue, nxt)
			}
		}
	}
	if visited != len(p.Activities) {
		return fmt.Errorf("活动网络存在环")
	}
	return nil
}

// sortedIDs returns activity IDs in lexicographic order.
func (p *Project) sortedIDs() []string {
	ids := make([]string, len(p.Activities))
	for i, a := range p.Activities {
		ids[i] = a.ID
	}
	sort.Strings(ids)
	return ids
}
