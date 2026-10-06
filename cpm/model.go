package cpm

import (
	"encoding/json"
	"fmt"
	"os"
)

// CrashOption 是某活动“最多压缩一次”的可选方案：
// 将该活动工期替换为更短的 ShorterDuration，并付出 Cost。
type CrashOption struct {
	ShorterDuration int `json:"shorter_duration"`
	Cost            int `json:"cost"`
}

// Activity 描述一项活动（允许 Duration 为 0 的里程碑）。
type Activity struct {
	ID           string       `json:"id"`
	Duration     int          `json:"duration"`
	Predecessors []string     `json:"predecessors"`
	Crash        *CrashOption `json:"crash,omitempty"`
}

// Project 是 JSON 输入的根结构。
type Project struct {
	Activities []Activity `json:"activities"`
	Budget     int        `json:"budget"`
}

// LoadProject 从 JSON 文件读取项目定义。
func LoadProject(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取输入文件: %w", err)
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("解析 JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate 检查非空、标识唯一、工期与预算合法、依赖存在。
// 环检测在 Schedule 中基于拓扑排序完成。
func (p *Project) Validate() error {
	if len(p.Activities) == 0 {
		return fmt.Errorf("活动集合不能为空")
	}
	if p.Budget < 0 {
		return fmt.Errorf("预算必须是非负整数，得到 %d", p.Budget)
	}
	index := make(map[string]int, len(p.Activities))
	for i := range p.Activities {
		a := &p.Activities[i]
		if a.ID == "" {
			return fmt.Errorf("第 %d 项活动缺少标识", i+1)
		}
		if _, dup := index[a.ID]; dup {
			return fmt.Errorf("活动标识重复: %q", a.ID)
		}
		index[a.ID] = i
		if a.Duration < 0 {
			return fmt.Errorf("活动 %q 工期必须是非负整数", a.ID)
		}
		if a.Crash != nil {
			c := a.Crash
			if c.ShorterDuration < 0 {
				return fmt.Errorf("活动 %q 的压缩工期必须是非负整数", a.ID)
			}
			if c.ShorterDuration >= a.Duration {
				return fmt.Errorf("活动 %q 的压缩工期 %d 必须严格短于原工期 %d",
					a.ID, c.ShorterDuration, a.Duration)
			}
			if c.Cost <= 0 {
				return fmt.Errorf("活动 %q 的压缩成本必须是正整数", a.ID)
			}
		}
	}
	crashCount := 0
	for i := range p.Activities {
		a := &p.Activities[i]
		if a.Crash != nil {
			crashCount++
		}
		seen := make(map[string]bool, len(a.Predecessors))
		for _, dep := range a.Predecessors {
			if _, ok := index[dep]; !ok {
				return fmt.Errorf("活动 %q 依赖未知活动 %q", a.ID, dep)
			}
			if dep == a.ID {
				return fmt.Errorf("活动 %q 不能依赖自身", a.ID)
			}
			if seen[dep] {
				return fmt.Errorf("活动 %q 的依赖 %q 重复", a.ID, dep)
			}
			seen[dep] = true
		}
	}
	if crashCount > 18 {
		return fmt.Errorf("可压缩活动至多 18 项，当前为 %d 项", crashCount)
	}
	return nil
}
