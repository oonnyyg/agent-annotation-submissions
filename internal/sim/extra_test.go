package sim

import "testing"

func TestValidation(t *testing.T) {
	cases := []struct {
		name string
		in   *Input
	}{
		{"duplicate station", &Input{Stations: []Station{{ID: "A"}, {ID: "A"}}}},
		{"negative buffer", &Input{Stations: []Station{{ID: "A", Buffer: -1}}}},
		{"duplicate job", &Input{
			Stations: []Station{{ID: "A"}},
			Jobs:     []Job{{ID: "J", Route: []Operation{{"A", 1}}}, {ID: "J", Route: []Operation{{"A", 1}}}},
		}},
		{"negative release", &Input{
			Stations: []Station{{ID: "A"}},
			Jobs:     []Job{{ID: "J", Release: -1, Route: []Operation{{"A", 1}}}},
		}},
		{"empty route", &Input{
			Stations: []Station{{ID: "A"}},
			Jobs:     []Job{{ID: "J", Route: nil}},
		}},
		{"unknown station", &Input{
			Stations: []Station{{ID: "A"}},
			Jobs:     []Job{{ID: "J", Route: []Operation{{"B", 1}}}},
		}},
		{"zero duration", &Input{
			Stations: []Station{{ID: "A"}},
			Jobs:     []Job{{ID: "J", Route: []Operation{{"A", 0}}}},
		}},
	}
	for _, tc := range cases {
		if err := Validate(tc.in); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
}

// Buffer queue is ordered by enqueue time, then by job id.
func TestQueueOrdering(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 2}, {ID: "B", Buffer: 0}},
		Jobs: []Job{
			{ID: "J9", Release: 0, Route: []Operation{{Station: "A", Duration: 3}}},
			{ID: "J2", Release: 0, Route: []Operation{{Station: "A", Duration: 1}}},
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 1}}},
		},
	}
	s, sum := runCase(t, in)
	if sum.Status != "completed" {
		t.Fatalf("status = %s", sum.Status)
	}
	// All three enqueue at 0; the machine takes them in job id order.
	starts := map[string]int{}
	for _, r := range s.Records() {
		starts[r.JobID] = r.Start
	}
	if starts["J1"] != 0 || starts["J2"] != 1 || starts["J9"] != 2 {
		t.Errorf("starts = %v, want J1@0 J2@1 J9@2", starts)
	}
}

// A job may revisit the same station; each visit is a separate op.
func TestReentrantRoute(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 1}},
		Jobs: []Job{
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 2}, {Station: "A", Duration: 3}}},
		},
	}
	s, sum := runCase(t, in)
	if sum.Status != "completed" {
		t.Fatalf("status = %s", sum.Status)
	}
	j1 := findJob(sum, "J1")
	if j1.Processing != 5 || j1.Completion != 5 {
		t.Errorf("J1 stats = %+v", j1)
	}
	if len(s.Records()) != 2 || s.Records()[1].OpIndex != 1 {
		t.Errorf("records = %+v", s.Records())
	}
}

// Releases and completions at the same instant are registered before
// any transfer or start in that round.
func TestSameInstantReleaseAndCompletion(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 0}},
		Jobs: []Job{
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 2}}},
			{ID: "J2", Release: 2, Route: []Operation{{Station: "A", Duration: 1}}},
		},
	}
	_, sum := runCase(t, in)
	if sum.Status != "completed" {
		t.Fatalf("status = %s", sum.Status)
	}
	j2 := findJob(sum, "J2")
	if j2.Queue != 0 || j2.Completion != 3 {
		t.Errorf("J2 stats = %+v, want queue 0 completion 3", j2)
	}
}

// A released job stuck outside a full zero-buffer first station with
// no future events is reported as waiting at the stall.
func TestStallWithExternalWaiting(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 0}, {ID: "B", Buffer: 0}},
		Jobs: []Job{
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 1}, {Station: "B", Duration: 1}}},
			{ID: "J2", Release: 0, Route: []Operation{{Station: "B", Duration: 1}, {Station: "A", Duration: 1}}},
			{ID: "J3", Release: 0, Route: []Operation{{Station: "A", Duration: 1}}},
		},
	}
	_, sum := runCase(t, in)
	if sum.Status != "stalled" {
		t.Fatalf("status = %s", sum.Status)
	}
	if len(sum.Stall.Waiting) != 1 || sum.Stall.Waiting[0] != "J3" {
		t.Errorf("waiting = %v, want [J3]", sum.Stall.Waiting)
	}
}
