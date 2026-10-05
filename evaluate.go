package main

import (
	"fmt"
	"sort"
	"strings"
)

// GroupReport 是单个泛化等价组的核查结果。
type GroupReport struct {
	Key          []string `json:"key"`          // 泛化后的准标识组合
	Size         int      `json:"size"`         // 组内记录数
	DistinctSens int      `json:"distinctSens"` // 组内敏感值种类数
	Kept         bool     `json:"kept"`         // 是否保留（否则整组抑制）
}

// RowReport 是逐行去留结果。
type RowReport struct {
	ID         string `json:"id"`
	Suppressed bool   `json:"suppressed"`
}

// Evaluation 是一个步数方案下的完整评估结果。
type Evaluation struct {
	Steps      []int         `json:"steps"`
	Feasible   bool          `json:"feasible"`
	Reason     string        `json:"reason,omitempty"`
	Suppressed int           `json:"suppressed"`
	Kept       int           `json:"kept"`
	GenCost    int           `json:"generalizationCost"`
	SupCost    int           `json:"suppressionCost"`
	TotalCost  int           `json:"totalCost"`
	Groups     []GroupReport `json:"groups"`
	Rows       []RowReport   `json:"rows"`
}

// Evaluate 在给定步数方案下泛化全部记录、分组并判定去留与成本。
// 结果按确定性顺序输出，输入记录顺序不影响评估。
func Evaluate(ds *Dataset, hier map[string]*Hierarchy, cfg *Config, steps []int) (*Evaluation, error) {
	if len(steps) != len(ds.QICols) {
		return nil, fmt.Errorf("步数向量长度 %d 与准标识列数 %d 不一致", len(steps), len(ds.QICols))
	}
	genCost := 0
	for i, s := range steps {
		if s < 0 || s > hier[ds.QICols[i]].MaxSteps() {
			return nil, fmt.Errorf("列 %q 的步数 %d 超出允许范围 [0,%d]", ds.QICols[i], s, hier[ds.QICols[i]].MaxSteps())
		}
		genCost += s * cfg.Weights[ds.QICols[i]]
	}
	type group struct {
		key  []string
		rows []int
		sens map[string]bool
	}
	groups := map[string]*group{}
	keys := make([][]string, len(ds.Records))
	for i, r := range ds.Records {
		key := make([]string, len(ds.QICols))
		for j, v := range r.QI {
			key[j] = hier[ds.QICols[j]].Generalize(v, steps[j])
		}
		keys[i] = key
		gk := strings.Join(key, "\x00")
		g, ok := groups[gk]
		if !ok {
			g = &group{key: key, sens: map[string]bool{}}
			groups[gk] = g
		}
		g.rows = append(g.rows, i)
		g.sens[r.Sensitive] = true
	}
	ev := &Evaluation{Steps: append([]int(nil), steps...), GenCost: genCost}
	// 确定性排序组。
	sortedKeys := make([]string, 0, len(groups))
	for gk := range groups {
		sortedKeys = append(sortedKeys, gk)
	}
	sort.Strings(sortedKeys)
	rowSup := make([]bool, len(ds.Records))
	for _, gk := range sortedKeys {
		g := groups[gk]
		keep := len(g.rows) >= cfg.K && len(g.sens) >= cfg.L
		ev.Groups = append(ev.Groups, GroupReport{
			Key:          g.key,
			Size:         len(g.rows),
			DistinctSens: len(g.sens),
			Kept:         keep,
		})
		if keep {
			ev.Kept += len(g.rows)
		} else {
			ev.Suppressed += len(g.rows)
			for _, ri := range g.rows {
				rowSup[ri] = true
			}
		}
	}
	for i, r := range ds.Records {
		ev.Rows = append(ev.Rows, RowReport{ID: r.ID, Suppressed: rowSup[i]})
	}
	sort.Slice(ev.Rows, func(a, b int) bool { return ev.Rows[a].ID < ev.Rows[b].ID })
	ev.SupCost = ev.Suppressed * cfg.SuppressionCost
	ev.TotalCost = ev.GenCost + ev.SupCost
	switch {
	case ev.Suppressed > cfg.Budget:
		ev.Reason = fmt.Sprintf("抑制条数 %d 超过预算 %d", ev.Suppressed, cfg.Budget)
	case ev.Kept == 0:
		ev.Reason = "方案未保留任何记录"
	default:
		ev.Feasible = true
	}
	return ev, nil
}
