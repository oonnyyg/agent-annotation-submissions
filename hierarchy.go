package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Hierarchy is a single-root generalization tree for one quasi-identifier
// column. parents maps each node to its parent; the root maps to "".
type Hierarchy struct {
	parents map[string]string
	depth   map[string]int
	leaves  map[string]bool
}

// hierarchyFile is the JSON shape: {"columns": {"col": {"root": ..., "edges": ...}}}
// Each column value is a nested object: {"value": "rootLabel", "children": [...]}.
type treeNode struct {
	Value    string     `json:"value"`
	Children []treeNode `json:"children"`
}

type hierarchyJSON struct {
	Columns map[string]treeNode `json:"columns"`
}

// LoadHierarchies parses and validates the hierarchy JSON file.
func LoadHierarchies(path string) (map[string]*Hierarchy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hierarchy file: %w", err)
	}
	var hj hierarchyJSON
	if err := json.Unmarshal(raw, &hj); err != nil {
		return nil, fmt.Errorf("parse hierarchy JSON: %w", err)
	}
	if len(hj.Columns) == 0 {
		return nil, fmt.Errorf("hierarchy file defines no columns")
	}
	out := make(map[string]*Hierarchy, len(hj.Columns))
	for col, root := range hj.Columns {
		h, err := buildHierarchy(col, root)
		if err != nil {
			return nil, err
		}
		out[col] = h
	}
	return out, nil
}

func buildHierarchy(col string, root treeNode) (*Hierarchy, error) {
	if root.Value == "" {
		return nil, fmt.Errorf("column %q: root value must not be empty", col)
	}
	h := &Hierarchy{
		parents: map[string]string{},
		depth:   map[string]int{},
		leaves:  map[string]bool{},
	}
	seen := map[string]bool{}
	var walk func(n treeNode, parent string, depth int) error
	walk = func(n treeNode, parent string, depth int) error {
		if n.Value == "" {
			return fmt.Errorf("column %q: empty node value", col)
		}
		if seen[n.Value] {
			// A node appearing twice means a shared child (not a tree)
			// or a duplicated label; both are rejected.
			return fmt.Errorf("column %q: node %q appears more than once; hierarchy must be a single tree", col, n.Value)
		}
		seen[n.Value] = true
		if depth > 4 {
			return fmt.Errorf("column %q: hierarchy depth exceeds 4", col)
		}
		h.parents[n.Value] = parent
		h.depth[n.Value] = depth
		if len(n.Children) == 0 {
			h.leaves[n.Value] = true
		}
		for _, c := range n.Children {
			if err := walk(c, n.Value, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, "", 0); err != nil {
		return nil, err
	}
	if len(h.leaves) == 0 {
		return nil, fmt.Errorf("column %q: hierarchy has no leaves", col)
	}
	return h, nil
}

// Generalize moves value up steps levels toward the root. If the value is
// already at the root (or steps exceed the remaining depth) it stays at root.
func (h *Hierarchy) Generalize(value string, steps int) (string, error) {
	if !h.leaves[value] {
		if _, ok := h.parents[value]; ok {
			return "", fmt.Errorf("value %q is an internal node, not a leaf", value)
		}
		return "", fmt.Errorf("unknown leaf value %q", value)
	}
	cur := value
	for i := 0; i < steps; i++ {
		p := h.parents[cur]
		if p == "" {
			break
		}
		cur = p
	}
	return cur, nil
}

// MaxSteps is the number of upward steps from the deepest leaf to the root.
func (h *Hierarchy) MaxSteps() int {
	max := 0
	for leaf := range h.leaves {
		if d := h.depth[leaf]; d > max {
			max = d
		}
	}
	return max
}

// LeafValues returns the sorted list of leaf values.
func (h *Hierarchy) LeafValues() []string {
	vals := make([]string, 0, len(h.leaves))
	for v := range h.leaves {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	return vals
}
