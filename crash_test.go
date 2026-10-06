package main

import "testing"

// Crashing a non-critical activity alone cannot shorten the project; the
// search must pick the combination on the true critical chain.
func TestSearchFindsGlobalOptimum(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 10,
	  "activities": [
	    {"id": "a", "duration": 5, "predecessors": [], "crash": {"duration": 2, "cost": 6}},
	    {"id": "b", "duration": 2, "predecessors": [], "crash": {"duration": 1, "cost": 4}}
	  ]
	}`)
	plan := searchPlans(p)
	// Crashing b (cheaper per day) never helps: a alone governs the finish.
	if plan.Finish != 2 {
		t.Fatalf("finish = %d, want 2", plan.Finish)
	}
	if len(plan.Selected) != 1 || plan.Selected[0] != "a" {
		t.Fatalf("selected = %v, want [a]", plan.Selected)
	}
	if plan.Cost != 6 {
		t.Fatalf("cost = %d, want 6", plan.Cost)
	}
}

// Budget limits exclude the best uncompressed combination.
func TestBudgetConstraint(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 3,
	  "activities": [
	    {"id": "a", "duration": 5, "predecessors": [], "crash": {"duration": 1, "cost": 5}}
	  ]
	}`)
	plan := searchPlans(p)
	if plan.Finish != 5 || len(plan.Selected) != 0 {
		t.Fatalf("plan = %+v, want no crashing within budget 3", plan)
	}
}

// Same finish: lower cost wins; then lexicographically smallest selected set.
func TestTieBreakCostThenIDs(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 10,
	  "activities": [
	    {"id": "x", "duration": 4, "predecessors": [], "crash": {"duration": 2, "cost": 3}},
	    {"id": "y", "duration": 4, "predecessors": [], "crash": {"duration": 2, "cost": 3}}
	  ]
	}`)
	plan := searchPlans(p)
	// Both branches are parallel; crashing one leaves finish at 4, crashing
	// both gives 2 at cost 6. Only one optimum exists here.
	if plan.Finish != 2 || plan.Cost != 6 {
		t.Fatalf("plan = %+v, want finish 2 cost 6", plan)
	}

	// Same finish at different costs: cheaper subset must win.
	p2 := mustProject(t, `{
	  "budget": 10,
	  "activities": [
	    {"id": "a", "duration": 4, "predecessors": [], "crash": {"duration": 2, "cost": 2}},
	    {"id": "b", "duration": 2, "predecessors": [], "crash": {"duration": 1, "cost": 5}}
	  ]
	}`)
	plan2 := searchPlans(p2)
	// Crashing a alone yields finish 2 at cost 2; adding b costs more for the
	// same finish and must not be chosen.
	if plan2.Finish != 2 || plan2.Cost != 2 || len(plan2.Selected) != 1 || plan2.Selected[0] != "a" {
		t.Fatalf("plan2 = %+v, want finish 2 cost 2 selected [a]", plan2)
	}

	// Same finish and same cost: lexicographically smallest ID sequence wins.
	p3 := mustProject(t, `{
	  "budget": 5,
	  "activities": [
	    {"id": "m", "duration": 3, "predecessors": [], "crash": {"duration": 2, "cost": 2}},
	    {"id": "n", "duration": 3, "predecessors": [], "crash": {"duration": 2, "cost": 2}}
	  ]
	}`)
	plan3 := searchPlans(p3)
	if plan3.Finish != 2 || plan3.Cost != 4 {
		t.Fatalf("plan3 = %+v, want finish 2 cost 4", plan3)
	}
	if !equalIDs(plan3.Selected, []string{"m", "n"}) {
		t.Fatalf("plan3 selected = %v, want [m n]", plan3.Selected)
	}
}

// After crashing, the critical path may shift to a formerly slack branch;
// the plan must be re-evaluated with fresh floats.
func TestCriticalPathShiftsAfterCrashing(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 10,
	  "activities": [
	    {"id": "a", "duration": 6, "predecessors": [], "crash": {"duration": 2, "cost": 4}},
	    {"id": "b", "duration": 5, "predecessors": []}
	  ]
	}`)
	base := computeCPM(p, baseDurations(p))
	basePath := criticalPath(p, base)
	if !equalIDs(basePath, []string{"a"}) {
		t.Fatalf("baseline path = %v, want [a]", basePath)
	}
	plan := searchPlans(p)
	if plan.Finish != 5 {
		t.Fatalf("finish = %d, want 5 (b becomes the bottleneck)", plan.Finish)
	}
	if !equalIDs(plan.Path, []string{"b"}) {
		t.Fatalf("crashed path = %v, want [b]", plan.Path)
	}
	if plan.Schedule.Float["a"] != 3 {
		t.Fatalf("float(a) after crash = %d, want 3", plan.Schedule.Float["a"])
	}
}

// Zero-duration activities can carry crash options down to zero days too.
func TestCrashToZeroDuration(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 5,
	  "activities": [
	    {"id": "a", "duration": 2, "predecessors": [], "crash": {"duration": 0, "cost": 1}},
	    {"id": "b", "duration": 0, "predecessors": ["a"]}
	  ]
	}`)
	plan := searchPlans(p)
	if plan.Finish != 0 {
		t.Fatalf("finish = %d, want 0", plan.Finish)
	}
	if !equalIDs(plan.Path, []string{"a", "b"}) {
		t.Fatalf("path = %v, want [a b]", plan.Path)
	}
}
