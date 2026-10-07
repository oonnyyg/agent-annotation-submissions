package main

import (
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func baseConfig() *Config {
	return &Config{
		Zones: []Zone{
			{ID: "outdoor", Outdoor: true},
			{ID: "lobby", Capacity: 2},
			{ID: "lab", Capacity: 1},
		},
		Persons: []Person{
			{ID: "alice", Initial: "outdoor"},
			{ID: "bob", Initial: "outdoor"},
			{ID: "carol", Initial: "outdoor"},
		},
		Credentials: []Credential{
			{ID: "card-a1", Person: "alice", Active: true, From: ts("2026-01-01T00:00:00Z"), To: ts("2027-01-01T00:00:00Z"), Zones: []string{"lobby", "lab"}},
			{ID: "card-a2", Person: "alice", Active: true, From: ts("2026-01-01T00:00:00Z"), To: ts("2027-01-01T00:00:00Z"), Zones: []string{"lobby"}},
			{ID: "card-b1", Person: "bob", Active: false, From: ts("2026-01-01T00:00:00Z"), To: ts("2027-01-01T00:00:00Z"), Zones: []string{"lobby"}},
			{ID: "card-c1", Person: "carol", Active: true, From: ts("2026-06-01T00:00:00Z"), To: ts("2026-07-01T00:00:00Z"), Zones: []string{"lobby", "lab"}},
		},
		Readers: []Reader{
			{ID: "r-in", From: "outdoor", To: "lobby"},
			{ID: "r-out", From: "lobby", To: "outdoor"},
			{ID: "r-lab", From: "lobby", To: "lab"},
		},
	}
}

func swipe(t, cred, reader string) EventRecord {
	return EventRecord{Kind: "swipe", Swipe: &Event{Time: ts(t), Credential: cred, Reader: reader}}
}

func TestMultiCardSamePerson(t *testing.T) {
	eng := NewEngine(baseConfig())
	eng.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-06-10T09:00:00Z", "card-a2", "r-out"), // second card, same person
	})
	if !eng.Results[0].OK || !eng.Results[1].OK {
		t.Fatalf("expected both granted: %+v", eng.Results)
	}
	if eng.loc["alice"] != "outdoor" {
		t.Fatalf("alice should be outdoor, got %q", eng.loc["alice"])
	}
	if eng.occ["lobby"] != 0 || eng.occ["outdoor"] != 3 {
		t.Fatalf("occupancy drift: %v", eng.occ)
	}
}

func TestZoneFullAndAtomicDeny(t *testing.T) {
	eng := NewEngine(baseConfig())
	eng.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-06-10T08:01:00Z", "card-c1", "r-in"),
		swipe("2026-06-10T08:02:00Z", "card-a1", "r-lab"), // lab cap 1, ok
		swipe("2026-06-10T08:03:00Z", "card-c1", "r-lab"), // lab full -> deny
	})
	if eng.Results[3].OK {
		t.Fatal("expected lab-full denial")
	}
	if eng.occ["lab"] != 1 || eng.occ["lobby"] != 1 {
		t.Fatalf("denial changed occupancy: %v", eng.occ)
	}
	if eng.loc["carol"] != "lobby" {
		t.Fatalf("carol moved on denial: %q", eng.loc["carol"])
	}
}

func TestSameTimestampStableOrder(t *testing.T) {
	// lab capacity 1: alice and carol both swipe r-lab at the same time after
	// entering the lobby; input order decides who gets in.
	eng := NewEngine(baseConfig())
	same := "2026-06-10T08:00:00Z"
	eng.Replay([]EventRecord{
		swipe(same, "card-c1", "r-in"),
		swipe(same, "card-a1", "r-in"),
		swipe(same, "card-c1", "r-lab"),
		swipe(same, "card-a1", "r-lab"),
	})
	if !eng.Results[2].OK || eng.Results[3].OK {
		t.Fatalf("stable order broken: %+v", eng.Results)
	}
	if eng.loc["carol"] != "lab" || eng.loc["alice"] != "lobby" {
		t.Fatalf("locations: %v", eng.loc)
	}
}

func TestDisabledAndExpiredCards(t *testing.T) {
	eng := NewEngine(baseConfig())
	eng.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-b1", "r-in"), // disabled
		swipe("2026-05-31T23:59:59Z", "card-c1", "r-in"), // before validity
		swipe("2026-07-01T00:00:00Z", "card-c1", "r-in"), // at To (half-open) -> invalid
	})
	for i, d := range eng.Results {
		if d.OK {
			t.Fatalf("event %d should be denied", i)
		}
	}
	if eng.occ["lobby"] != 0 {
		t.Fatalf("denials changed occupancy: %v", eng.occ)
	}
}

func TestWrongOriginAndExitNoPermission(t *testing.T) {
	cfg := baseConfig()
	eng := NewEngine(cfg)
	eng.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-a1", "r-out"), // alice is outdoor, not lobby
	})
	if eng.Results[0].OK {
		t.Fatal("expected wrong-origin denial")
	}
	// card-a2 has no lab permission but exit to outdoor never checks target permission.
	eng2 := NewEngine(cfg)
	eng2.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-a2", "r-in"),
		swipe("2026-06-10T09:00:00Z", "card-a2", "r-out"),
	})
	if !eng2.Results[1].OK {
		t.Fatalf("exit should not check target permission: %+v", eng2.Results[1])
	}
}

func TestCorrection(t *testing.T) {
	eng := NewEngine(baseConfig())
	eng.Replay([]EventRecord{
		{Kind: "correction", Correction: &Correction{Time: ts("2026-06-10T08:00:00Z"), Person: "alice", To: "lab", Reason: "guard escort"}},
		swipe("2026-06-10T09:00:00Z", "card-a1", "r-in"), // alice confirmed in lab, not outdoor
		{Kind: "correction", Correction: &Correction{Time: ts("2026-06-10T10:00:00Z"), Person: "bob", To: "lab", Reason: "audit"}},
	})
	if !eng.Results[0].OK {
		t.Fatalf("correction failed: %+v", eng.Results[0])
	}
	if eng.Results[1].OK {
		t.Fatal("swipe from wrong origin should be denied after correction")
	}
	if eng.Results[2].OK {
		t.Fatal("correction into full lab should be denied")
	}
	if eng.loc["alice"] != "lab" || eng.occ["lab"] != 1 {
		t.Fatalf("state: loc=%v occ=%v", eng.loc, eng.occ)
	}
}

func TestRosterMatchesOccupancy(t *testing.T) {
	eng := NewEngine(baseConfig())
	eng.Replay([]EventRecord{
		swipe("2026-06-10T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-06-10T08:01:00Z", "card-c1", "r-in"),
	})
	roster := eng.Roster()
	for z, people := range roster {
		if len(people) != eng.occ[z] {
			t.Fatalf("zone %s roster %d != occupancy %d", z, len(people), eng.occ[z])
		}
	}
}

func TestValidateRejectsBadReferences(t *testing.T) {
	cfg := baseConfig()
	cfg.Credentials[0].Zones = []string{"nope"}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected config validation error")
	}
	cfg2 := baseConfig()
	ef := &EventFile{Events: []EventRecord{swipe("2026-06-10T08:00:00Z", "ghost", "r-in")}}
	if err := validateEvents(cfg2, ef); err == nil {
		t.Fatal("expected event validation error")
	}
}
