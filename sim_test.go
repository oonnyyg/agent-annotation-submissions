package main

import (
	"strings"
	"testing"
)

func baseInput() *Input {
	return &Input{
		Config: Config{
			MinFloor: 1, MaxFloor: 5, StartFloor: 1,
			Capacity: 2, TravelTime: 2, DwellTime: 3,
		},
	}
}

func runSim(t *testing.T, in *Input) *Sim {
	t.Helper()
	if err := validate(in); err != nil {
		t.Fatalf("validate: %v", err)
	}
	s := NewSim(in)
	s.Run()
	for _, p := range s.all {
		if p.Board < 0 || p.Alight < 0 {
			t.Fatalf("passenger %s never completed: board=%d alight=%d", p.ID, p.Board, p.Alight)
		}
	}
	if s.maxLoad > s.cfg.Capacity {
		t.Fatalf("capacity exceeded: %d > %d", s.maxLoad, s.cfg.Capacity)
	}
	return s
}

func find(s *Sim, id string) *Passenger {
	for _, p := range s.all {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func hasLog(s *Sim, sub string) bool {
	for _, l := range s.logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestSinglePassengerTimeline(t *testing.T) {
	in := baseInput()
	in.Passengers = []PassengerInput{{ID: "p1", Arrive: 0, From: 1, To: 4}}
	s := runSim(t, in)
	p := find(s, "p1")
	if p.Board != 0 {
		t.Fatalf("board=%d, want 0", p.Board)
	}
	// depart t=3, floors 2,3,4 at t=5,7,9
	if p.Alight != 9 {
		t.Fatalf("alight=%d, want 9", p.Alight)
	}
	if !hasLog(s, "BOARD id=p1") || !hasLog(s, "ALIGHT id=p1") {
		t.Fatalf("missing board/alight logs:\n%s", strings.Join(s.logs, "\n"))
	}
}

func TestEndpointReversal(t *testing.T) {
	in := baseInput()
	in.Passengers = []PassengerInput{
		{ID: "up", Arrive: 0, From: 1, To: 5},
		{ID: "down", Arrive: 1, From: 5, To: 1},
	}
	s := runSim(t, in)
	if !hasLog(s, "REVERSE floor=5 dir=down") {
		t.Fatalf("expected reversal at top endpoint:\n%s", strings.Join(s.logs, "\n"))
	}
	up, down := find(s, "up"), find(s, "down")
	if down.Board != up.Alight {
		t.Fatalf("down boards at endpoint stop: board=%d alight=%d", down.Board, up.Alight)
	}
	if down.Alight <= down.Board {
		t.Fatalf("down never rode")
	}
}

func TestFullLoadSkipsPickup(t *testing.T) {
	in := baseInput()
	in.Capacity = 1
	in.Passengers = []PassengerInput{
		{ID: "first", Arrive: 0, From: 1, To: 5},
		{ID: "stranded", Arrive: 0, From: 2, To: 5},
	}
	s := runSim(t, in)
	first, stranded := find(s, "first"), find(s, "stranded")
	if first.Board != 0 {
		t.Fatalf("first board=%d, want 0", first.Board)
	}
	// Car passes floor 2 full without stopping; stranded boards only after
	// the car comes back idle.
	if stranded.Board <= first.Alight {
		t.Fatalf("stranded board=%d should be after first alight=%d", stranded.Board, first.Alight)
	}
	if hasLog(s, "STOP floor=2") && stranded.Board < first.Alight {
		t.Fatalf("car stopped at floor 2 while full")
	}
	if s.maxLoad != 1 {
		t.Fatalf("maxLoad=%d, want 1", s.maxLoad)
	}
}

func TestSameTimeArrivalsBoardInInputOrder(t *testing.T) {
	in := baseInput()
	in.Capacity = 1
	in.Passengers = []PassengerInput{
		{ID: "a", Arrive: 0, From: 1, To: 3},
		{ID: "b", Arrive: 0, From: 1, To: 4},
	}
	s := runSim(t, in)
	a, b := find(s, "a"), find(s, "b")
	if a.Board != 0 {
		t.Fatalf("a board=%d, want 0", a.Board)
	}
	if b.Board <= a.Alight {
		t.Fatalf("b board=%d should wait for the next idle dispatch (a alight=%d)", b.Board, a.Alight)
	}
}

func TestArrivalAtDoorOpenInstantBoards(t *testing.T) {
	in := baseInput()
	in.Passengers = []PassengerInput{
		{ID: "rider", Arrive: 0, From: 1, To: 3},
		// Car stops at floor 2? No stop needed there; instead arrive at the
		// origin floor exactly at door-open time t=0.
		{ID: "lucky", Arrive: 0, From: 1, To: 4},
	}
	s := runSim(t, in)
	if find(s, "lucky").Board != 0 {
		t.Fatalf("same-instant arrival should board at t=0")
	}
}

func TestArrivalDuringDwellWaitsNextChance(t *testing.T) {
	in := baseInput()
	in.Capacity = 1
	in.Passengers = []PassengerInput{
		{ID: "rider", Arrive: 0, From: 1, To: 5},
		// Doors at floor 1 open at t=0 and close at t=3; late arrives mid-dwell.
		{ID: "late", Arrive: 2, From: 1, To: 3},
	}
	s := runSim(t, in)
	late := find(s, "late")
	if late.Board <= 3 {
		t.Fatalf("late board=%d should be after the car returns", late.Board)
	}
}

func TestInvalidInputsRejected(t *testing.T) {
	cases := map[string]func(*Input){
		"duplicate id": func(in *Input) {
			in.Passengers = []PassengerInput{
				{ID: "x", Arrive: 0, From: 1, To: 2},
				{ID: "x", Arrive: 1, From: 1, To: 2},
			}
		},
		"from equals to": func(in *Input) {
			in.Passengers = []PassengerInput{{ID: "x", Arrive: 0, From: 2, To: 2}}
		},
		"floor out of range": func(in *Input) {
			in.Passengers = []PassengerInput{{ID: "x", Arrive: 0, From: 1, To: 9}}
		},
		"zero capacity": func(in *Input) {
			in.Capacity = 0
			in.Passengers = []PassengerInput{{ID: "x", Arrive: 0, From: 1, To: 2}}
		},
		"zero travel time": func(in *Input) {
			in.TravelTime = 0
			in.Passengers = []PassengerInput{{ID: "x", Arrive: 0, From: 1, To: 2}}
		},
		"bad range": func(in *Input) {
			in.MinFloor, in.MaxFloor = 3, 3
			in.Passengers = []PassengerInput{{ID: "x", Arrive: 0, From: 1, To: 2}}
		},
	}
	for name, mutate := range cases {
		in := baseInput()
		mutate(in)
		if err := validate(in); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
	}
}
