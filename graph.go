package main

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateGraph checks that all deps exist and the graph is acyclic.
// It must run before any command is executed.
func ValidateGraph(m *Manifest) error {
	for _, name := range m.Order {
		for _, d := range m.Targets[name].Deps {
			if _, ok := m.Targets[d]; !ok {
				return fmt.Errorf("target %q depends on unknown target %q", name, d)
			}
		}
	}
	// Cycle detection via DFS with colors.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var visit func(n string) error
	visit = func(n string) error {
		color[n] = gray
		stack = append(stack, n)
		for _, d := range m.Targets[n].Deps {
			switch color[d] {
			case gray:
				i := 0
				for i < len(stack) && stack[i] != d {
					i++
				}
				return fmt.Errorf("dependency cycle: %s", strings.Join(append(stack[i:], d), " -> "))
			case white:
				if err := visit(d); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return nil
	}
	for _, name := range m.Order {
		if color[name] == white {
			if err := visit(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Closure returns the dependency closure (including goals) of the given goals.
func Closure(m *Manifest, goals []string) (map[string]bool, error) {
	set := map[string]bool{}
	var add func(n string) error
	add = func(n string) error {
		t, ok := m.Targets[n]
		if !ok {
			return fmt.Errorf("unknown target %q", n)
		}
		if set[n] {
			return nil
		}
		set[n] = true
		for _, d := range t.Deps {
			if err := add(d); err != nil {
				return err
			}
		}
		return nil
	}
	for _, g := range goals {
		if err := add(g); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// TopoOrder orders the given set of targets so deps come first; targets with
// no dependency relation are ordered lexicographically by name.
func TopoOrder(m *Manifest, set map[string]bool) []string {
	indeg := map[string]int{}
	dependents := map[string][]string{}
	for n := range set {
		indeg[n] = 0
	}
	for n := range set {
		for _, d := range m.Targets[n].Deps {
			if set[d] {
				indeg[n]++
				dependents[d] = append(dependents[d], n)
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
		for _, dep := range dependents[n] {
			indeg[dep]--
			if indeg[dep] == 0 {
				// insert keeping lexicographic order
				i := sort.SearchStrings(ready, dep)
				ready = append(ready, "")
				copy(ready[i+1:], ready[i:])
				ready[i] = dep
			}
		}
	}
	return order
}
