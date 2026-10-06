package main

import "sort"

// Schedule holds the CPM results for one duration assignment.
type Schedule struct {
	ES, EF map[string]int
	LS, LF map[string]int
	Float  map[string]int
	Finish int
}

// Critical reports whether the activity has zero total float.
func (s *Schedule) Critical(id string) bool { return s.Float[id] == 0 }

// computeCPM runs the forward and backward passes over the network using the
// given durations. All activities share a single project finish day, so
// parallel branches are evaluated against the same horizon.
func computeCPM(p *Project, dur map[string]int) *Schedule {
	order := topoOrder(p)
	es := make(map[string]int, len(order))
	ef := make(map[string]int, len(order))
	byID := make(map[string]Activity, len(p.Activities))
	for _, a := range p.Activities {
		byID[a.ID] = a
	}
	finish := 0
	for _, id := range order {
		start := 0
		for _, dep := range byID[id].Predecessors {
			if ef[dep] > start {
				start = ef[dep]
			}
		}
		es[id] = start
		ef[id] = start + dur[id]
		if ef[id] > finish {
			finish = ef[id]
		}
	}
	succ := make(map[string][]string, len(order))
	for _, a := range p.Activities {
		for _, dep := range a.Predecessors {
			succ[dep] = append(succ[dep], a.ID)
		}
	}
	ls := make(map[string]int, len(order))
	lf := make(map[string]int, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		end := finish
		for _, nxt := range succ[id] {
			if ls[nxt] < end {
				end = ls[nxt]
			}
		}
		lf[id] = end
		ls[id] = end - dur[id]
	}
	floatMap := make(map[string]int, len(order))
	for _, id := range order {
		floatMap[id] = ls[id] - es[id]
	}
	return &Schedule{ES: es, EF: ef, LS: ls, LF: lf, Float: floatMap, Finish: finish}
}

// topoOrder returns activity IDs in a topological order (lexicographic among
// ready nodes for determinism).
func topoOrder(p *Project) []string {
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
	var ready []string
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, nxt := range succ[id] {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				// insert keeping lexicographic order
				pos := sort.SearchStrings(ready, nxt)
				ready = append(ready, "")
				copy(ready[pos+1:], ready[pos:])
				ready[pos] = nxt
			}
		}
	}
	return order
}

// criticalPath returns one complete critical path from a zero-predecessor
// start activity to an activity whose EF equals the project finish. Among all
// such paths it picks the lexicographically smallest ID sequence. The path
// follows real dependency edges only.
func criticalPath(p *Project, s *Schedule) []string {
	byID := make(map[string]Activity, len(p.Activities))
	preds := make(map[string][]string, len(p.Activities))
	for _, a := range p.Activities {
		byID[a.ID] = a
		preds[a.ID] = append(preds[a.ID], a.Predecessors...)
	}
	// memo[id] = lexicographically smallest critical path ending at id.
	memo := make(map[string][]string, len(p.Activities))
	var best func(id string) []string
	best = func(id string) []string {
		if path, ok := memo[id]; ok {
			return path
		}
		var bestPrefix []string
		for _, dep := range preds[id] {
			if !s.Critical(dep) || s.EF[dep] != s.ES[id] {
				continue
			}
			cand := best(dep)
			if cand == nil {
				continue
			}
			if bestPrefix == nil || lessIDs(cand, bestPrefix) {
				bestPrefix = cand
			}
		}
		var path []string
		if bestPrefix == nil {
			if len(byID[id].Predecessors) == 0 || allPredsNonCritical(preds[id], s) {
				path = []string{id}
			}
		} else {
			path = append(append([]string{}, bestPrefix...), id)
		}
		memo[id] = path
		return path
	}
	succ := make(map[string][]string, len(p.Activities))
	for _, a := range p.Activities {
		for _, dep := range a.Predecessors {
			succ[dep] = append(succ[dep], a.ID)
		}
	}
	var result []string
	for _, id := range topoOrder(p) {
		if !s.Critical(id) || s.EF[id] != s.Finish {
			continue
		}
		// A complete path ends at a terminal critical activity: any critical
		// successor would extend the chain to the same finish day.
		terminal := true
		for _, nxt := range succ[id] {
			if s.Critical(nxt) {
				terminal = false
				break
			}
		}
		if !terminal {
			continue
		}
		cand := best(id)
		if cand == nil {
			continue
		}
		if result == nil || lessIDs(cand, result) {
			result = cand
		}
	}
	return result
}

func allPredsNonCritical(preds []string, s *Schedule) bool {
	for _, dep := range preds {
		if s.Critical(dep) {
			return false
		}
	}
	return true
}

// lessIDs compares two ID sequences lexicographically.
func lessIDs(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
