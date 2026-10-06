package main

import (
	"path/filepath"
	"testing"
)

func mustSubmit(t *testing.T, s *Spool, user, doc string, total int, ranges string, copies int, duplex bool) *Job {
	t.Helper()
	j, err := s.Submit(user, doc, total, ranges, copies, duplex)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	return j
}

func TestParseRangesOverlapAndOrder(t *testing.T) {
	pages, err := ParseRanges("5,1-3,2-4", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5}
	if len(pages) != len(want) {
		t.Fatalf("got %v want %v", pages, want)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Fatalf("got %v want %v", pages, want)
		}
	}
}

func TestParseRangesInvalidRejectsAll(t *testing.T) {
	for _, spec := range []string{"0", "1-11", "a-b", "1,,2", "-"} {
		if _, err := ParseRanges(spec, 10); err == nil {
			t.Fatalf("spec %q should be rejected", spec)
		}
	}
}

func TestBuildPlanDuplexOddPages(t *testing.T) {
	plan := BuildPlan([]int{1, 2, 3}, 1, true)
	if len(plan) != 2 {
		t.Fatalf("want 2 sheets, got %d", len(plan))
	}
	if *plan[0].Front != 1 || *plan[0].Back != 2 {
		t.Fatalf("sheet1 wrong: %v", plan[0])
	}
	if *plan[1].Front != 3 || plan[1].Back != nil {
		t.Fatalf("odd last page must leave back blank: %v", plan[1])
	}
}

func TestBuildPlanMultiCopyEachStartsOnFront(t *testing.T) {
	plan := BuildPlan([]int{1, 2, 3}, 2, true)
	if len(plan) != 4 {
		t.Fatalf("want 4 sheets, got %d", len(plan))
	}
	// copy 2 must start on a fresh front side, not share copy 1's blank back
	if plan[2].Copy != 2 || *plan[2].Front != 1 || *plan[2].Back != 2 {
		t.Fatalf("copy 2 must restart on front: %v", plan[2])
	}
	if *plan[3].Front != 3 || plan[3].Back != nil {
		t.Fatalf("copy 2 odd tail wrong: %v", plan[3])
	}
}

func TestBuildPlanSimplex(t *testing.T) {
	plan := BuildPlan([]int{2, 5}, 2, false)
	if len(plan) != 4 {
		t.Fatalf("want 4 sheets, got %d", len(plan))
	}
	for _, sh := range plan {
		if sh.Back != nil {
			t.Fatalf("simplex sheet must have blank back: %v", sh)
		}
	}
}

func TestQuotaReservationAndInsufficient(t *testing.T) {
	s := NewSpool()
	if err := s.RegisterUser("alice", 5); err != nil {
		t.Fatal(err)
	}
	mustSubmit(t, s, "alice", "a", 10, "1-3", 1, true) // 2 sheets
	if _, _, _, rem, _ := s.Report("alice"); rem != 3 {
		t.Fatalf("remaining want 3, got %d", rem)
	}
	if _, err := s.Submit("alice", "b", 10, "1-4", 2, true); err == nil {
		t.Fatal("submit exceeding quota must be rejected")
	}
	if len(s.Jobs) != 1 {
		t.Fatal("rejected job must not be enqueued")
	}
}

func TestConfirmSequenceAndAccounting(t *testing.T) {
	s := NewSpool()
	s.RegisterUser("alice", 10)
	j := mustSubmit(t, s, "alice", "a", 10, "1-3", 1, true) // 2 sheets

	if err := s.Confirm(j.ID, 2); err == nil {
		t.Fatal("skipping unconfirmed sheet must be rejected")
	}
	if err := s.Confirm(j.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(j.ID, 1); err == nil {
		t.Fatal("duplicate confirm must be rejected")
	}
	_, reserved, used, _, _ := s.Report("alice")
	if reserved != 1 || used != 1 {
		t.Fatalf("reserved=%d used=%d, want 1/1", reserved, used)
	}
	if err := s.Confirm(j.ID, 2); err != nil {
		t.Fatal(err)
	}
	if j.Status != StatusDone {
		t.Fatalf("job should be done, got %s", j.Status)
	}
	if err := s.Confirm(j.ID, 3); err == nil {
		t.Fatal("confirming a finished job must be rejected")
	}
}

func TestPauseDoesNotBlockQueue(t *testing.T) {
	s := NewSpool()
	s.RegisterUser("alice", 100)
	j1 := mustSubmit(t, s, "alice", "a", 10, "1", 1, false)
	j2 := mustSubmit(t, s, "alice", "b", 10, "2", 1, false)

	if err := s.Pause(j1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(j1.ID, 1); err == nil {
		t.Fatal("paused job cannot be confirmed")
	}
	if err := s.Confirm(j2.ID, 1); err != nil {
		t.Fatalf("paused job must not block later jobs: %v", err)
	}
	if err := s.Resume(j1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(j1.ID, 1); err != nil {
		t.Fatal(err)
	}
}

func TestPauseOnlyBeforeStart(t *testing.T) {
	s := NewSpool()
	s.RegisterUser("alice", 100)
	j := mustSubmit(t, s, "alice", "a", 10, "1-2", 1, false)
	if err := s.Confirm(j.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(j.ID); err == nil {
		t.Fatal("pausing a started job must be rejected")
	}
}

func TestInterruptResumeAndCancel(t *testing.T) {
	s := NewSpool()
	s.RegisterUser("alice", 10)
	j := mustSubmit(t, s, "alice", "a", 10, "1-5", 1, true) // 3 sheets
	if err := s.Confirm(j.ID, 1); err != nil {
		t.Fatal(err)
	}

	// simulate restart: persist and reload, progress must survive
	path := filepath.Join(t.TempDir(), "state.json")
	if err := save(path, s); err != nil {
		t.Fatal(err)
	}
	s2 := load(path)
	j2, err := s2.findJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j2.Confirmed != 1 {
		t.Fatalf("progress lost after reload: %d", j2.Confirmed)
	}
	if err := s2.Confirm(j.ID, 1); err == nil {
		t.Fatal("re-confirming printed sheet after restart must be rejected")
	}
	if err := s2.Confirm(j.ID, 2); err != nil {
		t.Fatal(err)
	}

	// cancel after interruption: only unprinted reservation is released
	if err := s2.Cancel(j.ID); err != nil {
		t.Fatal(err)
	}
	quota, reserved, used, remaining, err := s2.Report("alice")
	if err != nil {
		t.Fatal(err)
	}
	if reserved != 0 || used != 2 || remaining != quota-2 {
		t.Fatalf("reserved=%d used=%d remaining=%d, want 0/2/%d", reserved, used, remaining, quota-2)
	}
	if err := s2.Confirm(j.ID, 3); err == nil {
		t.Fatal("cancelled job cannot be confirmed")
	}
	if err := s2.Cancel(j.ID); err == nil {
		t.Fatal("double cancel must be rejected")
	}
}

func TestInvalidOpsDoNotMutate(t *testing.T) {
	s := NewSpool()
	s.RegisterUser("alice", 10)
	j := mustSubmit(t, s, "alice", "a", 10, "1-2", 1, false)
	before := len(s.Jobs)
	if err := s.Pause("nope"); err == nil {
		t.Fatal("unknown job must fail")
	}
	if _, err := s.Submit("ghost", "x", 5, "1", 1, false); err == nil {
		t.Fatal("unknown user must fail")
	}
	if len(s.Jobs) != before {
		t.Fatal("invalid ops changed the queue")
	}
	_, reserved, used, _, _ := s.Report("alice")
	if reserved != 2 || used != 0 {
		t.Fatalf("quota changed by invalid ops: reserved=%d used=%d", reserved, used)
	}
	_ = j
}
