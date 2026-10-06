package engine

import (
	"fmt"
	"sort"
)

// Op 是一条批量操作：Content 为原始内容；空串表示清空该单元格。
type Op struct {
	Addr    string `json:"addr"`
	Content string `json:"content"`
}

// Change 表示本次重算后值或错误发生变化的单元格。
type Change struct {
	Addr   string `json:"addr"`
	Before Result `json:"before"`
	After  Result `json:"after"`
}

// Apply 批量设值或清空。
// 规则：同批地址不得重复；先检查全部语法和地址，全部通过后才整体应用；
// 任何输入错误都原样保留当前工作簿。
func (w *Workbook) Apply(ops []Op) ([]Change, error) {
	parsed := make(map[string]*cell, len(ops))
	for i, op := range ops {
		addr, err := ParseAddr(op.Addr)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条操作: %w", i+1, err)
		}
		key := addr.String()
		if _, dup := parsed[key]; dup {
			return nil, fmt.Errorf("第 %d 条操作: 地址 %s 在同批中重复", i+1, key)
		}
		c, err := validateContent(op.Content)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条操作（%s）: %w", i+1, key, err)
		}
		parsed[key] = c
	}

	// 快照应用前的结果（仅需要本次涉及地址，但受影响链可能扩散，
	// 因此对全簿公式单元格拍快照，上限 2600，成本可接受）。
	before := map[string]Result{}
	for addr, c := range w.cells {
		before[addr] = c.result
	}

	for key, c := range parsed {
		w.applyOne(key, c)
	}
	w.recalc()

	var changes []Change
	for addr := range w.cells {
		after := w.cells[addr].result
		old, existed := before[addr]
		if !existed {
			old = valueResult(0)
		}
		if !sameResult(old, after) {
			changes = append(changes, Change{Addr: addr, Before: old, After: after})
		}
		delete(before, addr)
	}
	// 被清空的单元格此前可能持有结果，从空视为 0。
	for addr, old := range before {
		if !sameResult(old, valueResult(0)) {
			changes = append(changes, Change{Addr: addr, Before: old, After: valueResult(0)})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Addr < changes[j].Addr })
	return changes, nil
}

func (w *Workbook) applyOne(key string, c *cell) {
	if c == nil {
		delete(w.cells, key)
	} else {
		// 保留旧结果引用由调用方快照处理，这里直接替换。
		if c.formula == nil {
			c.result = valueResult(c.number)
		}
		w.cells[key] = c
	}
	w.rebuildDepsFor(key, c)
}

// rebuildDepsFor 重写单个单元格的出边：修改公式后旧依赖即被移除。
func (w *Workbook) rebuildDepsFor(key string, c *cell) {
	delete(w.deps, key)
	if c == nil || c.formula == nil {
		return
	}
	set := map[string]bool{}
	for _, dep := range c.formula.deps() {
		set[dep] = true
	}
	w.deps[key] = set
}
