package main

import "sort"

// Plan is one crashing decision: which options are selected.
type Plan struct {
	Selected []string // sorted selected activity IDs
	Cost     int
	Finish   int
	Schedule *Schedule
	Path     []string
}

// baseDurations returns the unmodified duration of every activity.
func baseDurations(p *Project) map[string]int {
	dur := make(map[string]int, len(p.Activities))
	for _, a := range p.Activities {
		dur[a.ID] = a.Duration
	}
	return dur
}

// durationsFor applies the selected crash options to the base durations.
func durationsFor(p *Project, selected map[string]bool) map[string]int {
	dur := baseDurations(p)
	for _, a := range p.Activities {
		if a.Crash != nil && selected[a.ID] {
			dur[a.ID] = a.Crash.Duration
		}
	}
	return dur
}

// better reports whether plan a beats plan b: earlier finish, then lower
// cost, then lexicographically smaller sorted selected-ID sequence.
func better(a, b *Plan) bool {
	if b == nil {
		return true
	}
	if a.Finish != b.Finish {
		return a.Finish < b.Finish
	}
	if a.Cost != b.Cost {
		return a.Cost < b.Cost
	}
	return lessIDs(a.Selected, b.Selected)
}

// searchPlans enumerates every subset of crash options (2^n, n <= 18) and
// returns the best plan within budget. This is a global exact search, not a
// per-activity greedy heuristic.
func searchPlans(p *Project) *Plan {
	var crashable []Activity
	for _, a := range p.Activities {
		if a.Crash != nil {
			crashable = append(crashable, a)
		}
	}
	sort.Slice(crashable, func(i, j int) bool { return crashable[i].ID < crashable[j].ID })
	n := len(crashable)
	var best *Plan
	for mask := 0; mask < (1 << n); mask++ {
		selected := make(map[string]bool, n)
		cost := 0
		var ids []string
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				selected[crashable[i].ID] = true
				cost += crashable[i].Crash.Cost
				ids = append(ids, crashable[i].ID)
			}
		}
		if cost > p.Budget {
			continue
		}
		sort.Strings(ids)
		sched := computeCPM(p, durationsFor(p, selected))
		plan := &Plan{
			Selected: ids,
			Cost:     cost,
			Finish:   sched.Finish,
			Schedule: sched,
		}
		if better(plan, best) {
			best = plan
		}
	}
	best.Path = criticalPath(p, best.Schedule)
	return best
}
