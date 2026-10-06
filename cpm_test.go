package main

import (
	"strings"
	"testing"
)

func mustProject(t *testing.T, doc string) *Project {
	t.Helper()
	p, err := loadProject(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("loadProject: %v", err)
	}
	return p
}

// Two parallel branches share one project finish: only the longer branch is
// critical, the shorter one keeps positive float.
func TestParallelBranchesSharedFinish(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 0,
	  "activities": [
	    {"id": "a", "duration": 5, "predecessors": []},
	    {"id": "b", "duration": 3, "predecessors": []}
	  ]
	}`)
	s := computeCPM(p, baseDurations(p))
	if s.Finish != 5 {
		t.Fatalf("finish = %d, want 5", s.Finish)
	}
	if s.Float["a"] != 0 {
		t.Fatalf("float(a) = %d, want 0", s.Float["a"])
	}
	if s.Float["b"] != 2 {
		t.Fatalf("float(b) = %d, want 2 (shared finish, not per-branch)", s.Float["b"])
	}
	path := criticalPath(p, s)
	if len(path) != 1 || path[0] != "a" {
		t.Fatalf("critical path = %v, want [a]", path)
	}
}

// Zero-duration milestones participate in the network like any activity.
func TestZeroDurationMilestone(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 0,
	  "activities": [
	    {"id": "start", "duration": 0, "predecessors": []},
	    {"id": "work", "duration": 4, "predecessors": ["start"]},
	    {"id": "done", "duration": 0, "predecessors": ["work"]}
	  ]
	}`)
	s := computeCPM(p, baseDurations(p))
	if s.Finish != 4 {
		t.Fatalf("finish = %d, want 4", s.Finish)
	}
	for _, id := range []string{"start", "work", "done"} {
		if !s.Critical(id) {
			t.Fatalf("activity %s should be critical", id)
		}
	}
	path := criticalPath(p, s)
	want := []string{"start", "work", "done"}
	if !equalIDs(path, want) {
		t.Fatalf("critical path = %v, want %v", path, want)
	}
}

// With two complete critical paths, pick the lexicographically smallest ID
// sequence, and the path must follow real edges.
func TestCriticalPathLexicographic(t *testing.T) {
	p := mustProject(t, `{
	  "budget": 0,
	  "activities": [
	    {"id": "s", "duration": 1, "predecessors": []},
	    {"id": "z", "duration": 2, "predecessors": ["s"]},
	    {"id": "a", "duration": 2, "predecessors": ["s"]},
	    {"id": "e", "duration": 1, "predecessors": ["z", "a"]}
	  ]
	}`)
	s := computeCPM(p, baseDurations(p))
	path := criticalPath(p, s)
	want := []string{"s", "a", "e"}
	if !equalIDs(path, want) {
		t.Fatalf("critical path = %v, want %v", path, want)
	}
}

func TestRejectUnknownDependency(t *testing.T) {
	_, err := loadProject(strings.NewReader(`{
	  "budget": 0,
	  "activities": [{"id": "a", "duration": 1, "predecessors": ["ghost"]}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "未知活动") {
		t.Fatalf("err = %v, want unknown dependency error", err)
	}
}

func TestRejectCycle(t *testing.T) {
	_, err := loadProject(strings.NewReader(`{
	  "budget": 0,
	  "activities": [
	    {"id": "a", "duration": 1, "predecessors": ["b"]},
	    {"id": "b", "duration": 1, "predecessors": ["a"]}
	  ]
	}`))
	if err == nil || !strings.Contains(err.Error(), "环") {
		t.Fatalf("err = %v, want cycle error", err)
	}
}

func TestRejectEmptyAndDuplicates(t *testing.T) {
	if _, err := loadProject(strings.NewReader(`{"budget": 0, "activities": []}`)); err == nil {
		t.Fatal("empty activity set should be rejected")
	}
	_, err := loadProject(strings.NewReader(`{
	  "budget": 0,
	  "activities": [
	    {"id": "a", "duration": 1, "predecessors": []},
	    {"id": "a", "duration": 2, "predecessors": []}
	  ]
	}`))
	if err == nil {
		t.Fatal("duplicate ids should be rejected")
	}
}
