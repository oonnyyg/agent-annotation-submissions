package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const maxHierarchyDepth = 4

// Hierarchy 表示单个准标识列的单根泛化层级树。
// parent[v] 给出 v 的父节点；根节点的父节点为空。
type Hierarchy struct {
	parent map[string]string
	depth  map[string]int // 各节点到根的距离（最大可泛化步数）
	root   string
}

type hierarchyFile struct {
	Hierarchies map[string]map[string]string `json:"hierarchies"`
}

// LoadHierarchies 从 JSON 文件读取各列层级。
// JSON 形如 {"hierarchies": {"列名": {"子值": "父值", ...}}}，
// 根节点不出现为任何值的父节点映射的键（或以 "" 为父）。
func LoadHierarchies(path string) (map[string]*Hierarchy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取层级文件失败: %w", err)
	}
	var hf hierarchyFile
	if err := json.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("层级文件不是合法 JSON: %w", err)
	}
	if len(hf.Hierarchies) == 0 {
		return nil, fmt.Errorf("层级文件不含任何列")
	}
	out := make(map[string]*Hierarchy, len(hf.Hierarchies))
	for col, edges := range hf.Hierarchies {
		h, err := buildHierarchy(col, edges)
		if err != nil {
			return nil, err
		}
		out[col] = h
	}
	return out, nil
}

func buildHierarchy(col string, edges map[string]string) (*Hierarchy, error) {
	if len(edges) == 0 {
		return nil, fmt.Errorf("列 %q 的层级为空", col)
	}
	parent := make(map[string]string, len(edges))
	nodes := map[string]bool{}
	for child, par := range edges {
		if child == "" {
			return nil, fmt.Errorf("列 %q 的层级含空节点名", col)
		}
		if child == par {
			return nil, fmt.Errorf("列 %q 的层级含自环 %q", col, child)
		}
		parent[child] = par
		nodes[child] = true
		if par != "" {
			nodes[par] = true
		}
	}
	// 找根：出现为父节点但自身无父映射（或父为 ""）的节点。
	roots := []string{}
	for n := range nodes {
		p, ok := parent[n]
		if !ok || p == "" {
			roots = append(roots, n)
		}
	}
	if len(roots) != 1 {
		return nil, fmt.Errorf("列 %q 的层级应有单根，实际发现 %d 个根", col, len(roots))
	}
	root := roots[0]
	// 每个节点沿父链必须能到达根且无环（父映射天然单父，故只需查环与连通）。
	depth := make(map[string]int, len(nodes))
	for n := range nodes {
		d, err := chainDepth(parent, n, root)
		if err != nil {
			return nil, fmt.Errorf("列 %q 的层级不是树: %w", col, err)
		}
		if d > maxHierarchyDepth {
			return nil, fmt.Errorf("列 %q 的层级深度 %d 超过上限 %d", col, d, maxHierarchyDepth)
		}
		depth[n] = d
	}
	return &Hierarchy{parent: parent, depth: depth, root: root}, nil
}

func chainDepth(parent map[string]string, node, root string) (int, error) {
	seen := map[string]bool{}
	d := 0
	cur := node
	for cur != root {
		if seen[cur] {
			return 0, fmt.Errorf("节点 %q 位于环上", cur)
		}
		seen[cur] = true
		p, ok := parent[cur]
		if !ok || p == "" {
			return 0, fmt.Errorf("节点 %q 无法到达根 %q", node, root)
		}
		cur = p
		d++
	}
	return d, nil
}

// IsLeaf 报告值是否为叶节点（出现为子节点但从不作为父节点）。
func (h *Hierarchy) IsLeaf(v string) bool {
	if _, ok := h.parent[v]; !ok {
		return false
	}
	for _, p := range h.parent {
		if p == v {
			return false
		}
	}
	return true
}

// MaxSteps 返回该列允许的最大泛化步数（叶到根的最长链长）。
func (h *Hierarchy) MaxSteps() int {
	max := 0
	for _, d := range h.depth {
		if d > max {
			max = d
		}
	}
	return max
}

// Generalize 将值向上泛化 steps 步；已到根则保持。
func (h *Hierarchy) Generalize(v string, steps int) string {
	cur := v
	for i := 0; i < steps; i++ {
		if cur == h.root {
			break
		}
		cur = h.parent[cur]
	}
	return cur
}
