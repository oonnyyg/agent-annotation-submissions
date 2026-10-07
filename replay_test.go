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
		People: []Person{
			{ID: "alice", InitialZone: "outdoor"},
			{ID: "bob", InitialZone: "outdoor"},
			{ID: "carol", InitialZone: "outdoor"},
		},
		Credentials: []Credential{
			{ID: "card-a1", Person: "alice", Enabled: true,
				ValidFrom: ts("2026-01-01T00:00:00Z"), ValidTo: ts("2027-01-01T00:00:00Z"),
				Zones: []string{"lobby", "lab"}},
			{ID: "card-a2", Person: "alice", Enabled: true,
				ValidFrom: ts("2026-01-01T00:00:00Z"), ValidTo: ts("2027-01-01T00:00:00Z"),
				Zones: []string{"lobby"}},
			{ID: "card-b1", Person: "bob", Enabled: true,
				ValidFrom: ts("2026-01-01T00:00:00Z"), ValidTo: ts("2026-06-01T00:00:00Z"),
				Zones: []string{"lobby", "lab"}},
			{ID: "card-c1", Person: "carol", Enabled: false,
				ValidFrom: ts("2026-01-01T00:00:00Z"), ValidTo: ts("2027-01-01T00:00:00Z"),
				Zones: []string{"lobby"}},
		},
		Readers: []Reader{
			{ID: "r-in", From: "outdoor", To: "lobby"},
			{ID: "r-out", From: "lobby", To: "outdoor"},
			{ID: "r-lab", From: "lobby", To: "lab"},
		},
	}
}

func swipe(t, cred, reader string) Event {
	return Event{Time: ts(t), Type: "swipe", Credential: cred, Reader: reader}
}

func TestValidateOK(t *testing.T) {
	if err := baseConfig().validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBadReferences(t *testing.T) {
	cfg := baseConfig()
	cfg.Readers[0].To = "nowhere"
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown reader target")
	}
	cfg = baseConfig()
	cfg.Credentials[0].Person = "ghost"
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown person")
	}
	cfg = baseConfig()
	cfg.People[0].InitialZone = "nowhere"
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown initial zone")
	}
}

func TestMultiCredentialSamePerson(t *testing.T) {
	events := []Event{
		swipe("2026-03-01T08:00:00Z", "card-a1", "r-in"),
		// Second card of the same person: still one location, now in lobby.
		swipe("2026-03-01T08:05:00Z", "card-a2", "r-out"),
	}
	d := Replay(baseConfig(), events)
	if !d[0].Allowed || !d[1].Allowed {
		t.Fatalf("expected both allowed: %+v", d)
	}
	if d[1].FromZone != "lobby" || d[1].ToZone != "outdoor" {
		t.Fatalf("unexpected move: %+v", d[1])
	}
	if d[1].Occupancy["lobby"] != 0 || d[1].Occupancy["outdoor"] != 3 {
		t.Fatalf("bad occupancy: %v", d[1].Occupancy)
	}
}

func TestDenyReasons(t *testing.T) {
	events := []Event{
		// Disabled credential.
		swipe("2026-03-01T08:00:00Z", "card-c1", "r-in"),
		// Expired credential (valid_to is exclusive).
		swipe("2026-06-01T00:00:00Z", "card-b1", "r-in"),
		// Valid swipe in.
		swipe("2026-03-01T08:01:00Z", "card-a1", "r-in"),
		// Repeat entry from wrong origin.
		swipe("2026-03-01T08:02:00Z", "card-a1", "r-in"),
		// Exit from wrong zone handled by origin check: bob never entered.
		swipe("2026-03-01T08:03:00Z", "card-b1", "r-out"),
	}
	d := Replay(baseConfig(), events)
	want := []bool{false, false, true, false, false}
	for i, w := range want {
		if d[i].Allowed != w {
			t.Errorf("event %d: allowed=%v want %v (%s)", i, d[i].Allowed, w, d[i].Reason)
		}
	}
	// Denials must not change occupancy.
	if d[0].Occupancy["lobby"] != 0 || d[1].Occupancy["lobby"] != 0 {
		t.Errorf("denial changed occupancy: %v / %v", d[0].Occupancy, d[1].Occupancy)
	}
	if d[4].Occupancy["lobby"] != 1 || d[4].Occupancy["outdoor"] != 2 {
		t.Errorf("final occupancy wrong: %v", d[4].Occupancy)
	}
}

func TestCapacityFull(t *testing.T) {
	events := []Event{
		swipe("2026-03-01T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-03-01T08:01:00Z", "card-b1", "r-in"),
		// lobby capacity is 2; carol's card is disabled, use correction below.
	}
	d := Replay(baseConfig(), events)
	if d[1].Occupancy["lobby"] != 2 {
		t.Fatalf("lobby should be full: %v", d[1].Occupancy)
	}
	// Lab capacity 1: alice enters, bob denied.
	events = append(events,
		swipe("2026-03-01T08:02:00Z", "card-a1", "r-lab"),
		swipe("2026-03-01T08:03:00Z", "card-b1", "r-lab"),
	)
	d = Replay(baseConfig(), events)
	if !d[2].Allowed {
		t.Fatalf("alice should enter lab: %s", d[2].Reason)
	}
	if d[3].Allowed {
		t.Fatal("bob should be denied: lab full")
	}
	if d[3].Occupancy["lab"] != 1 || d[3].Occupancy["lobby"] != 1 {
		t.Fatalf("denial changed counts: %v", d[3].Occupancy)
	}
}

func TestSameTimestampOrder(t *testing.T) {
	// Lab capacity 1; both swipe at the same time. Input order decides.
	events := []Event{
		swipe("2026-03-01T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-03-01T08:00:00Z", "card-b1", "r-in"),
		swipe("2026-03-01T09:00:00Z", "card-b1", "r-lab"),
		swipe("2026-03-01T09:00:00Z", "card-a1", "r-lab"),
	}
	d := Replay(baseConfig(), events)
	if !d[2].Allowed || d[3].Allowed {
		t.Fatalf("stable order violated: d2=%v d3=%v", d[2].Allowed, d[3].Allowed)
	}
}

func TestCorrection(t *testing.T) {
	events := []Event{
		{Time: ts("2026-03-01T10:00:00Z"), Type: "correct", Person: "carol", To: "lobby", Reason: "guard escort"},
		// Correction without reason denied.
		{Time: ts("2026-03-01T10:01:00Z"), Type: "correct", Person: "bob", To: "lobby"},
		// Carol now exits via reader from her confirmed position.
		swipe("2026-03-01T10:02:00Z", "card-c1", "r-out"),
	}
	d := Replay(baseConfig(), events)
	if !d[0].Allowed || d[0].FromZone != "outdoor" || d[0].ToZone != "lobby" {
		t.Fatalf("correction failed: %+v", d[0])
	}
	if d[1].Allowed {
		t.Fatal("correction without reason must be denied")
	}
	// card-c1 is disabled, so exit swipe is denied; position stays lobby.
	if d[2].Allowed {
		t.Fatal("disabled card must be denied even for exit")
	}
	if d[2].Occupancy["lobby"] != 1 {
		t.Fatalf("position must stay after denial: %v", d[2].Occupancy)
	}
}

func TestRosterAt(t *testing.T) {
	cfg := baseConfig()
	events := []Event{
		swipe("2026-03-01T08:00:00Z", "card-a1", "r-in"),
		swipe("2026-03-01T08:01:00Z", "card-b1", "r-in"),
		swipe("2026-03-01T09:00:00Z", "card-a1", "r-out"),
	}
	roster, occ := RosterAt(cfg, events, ts("2026-03-01T08:30:00Z"))
	if len(roster["lobby"]) != 2 || occ["lobby"] != 2 {
		t.Fatalf("roster at 08:30 wrong: %v %v", roster, occ)
	}
	roster, occ = RosterAt(cfg, events, ts("2026-03-01T10:00:00Z"))
	if len(roster["lobby"]) != 1 || roster["lobby"][0] != "bob" {
		t.Fatalf("final roster wrong: %v", roster)
	}
	total := 0
	for _, z := range cfg.Zones {
		total += occ[z.ID]
	}
	if total != len(cfg.People) {
		t.Fatalf("occupancy total %d != people %d", total, len(cfg.People))
	}
}
