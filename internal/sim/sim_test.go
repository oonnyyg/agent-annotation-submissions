package sim

import "testing"

func runCase(t *testing.T, in *Input) (*Simulator, *Summary) {
	t.Helper()
	if err := Validate(in); err != nil {
		t.Fatalf("validate: %v", err)
	}
	s := New(in)
	return s, s.Run()
}

func findJob(sum *Summary, id string) *JobStats {
	for i := range sum.Jobs {
		if sum.Jobs[i].JobID == id {
			return &sum.Jobs[i]
		}
	}
	return nil
}

// Zero buffer: a finished part cannot leave while the next machine is
// busy, so it blocks its own machine and delays the following job.
func TestZeroBufferBlocking(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 0}, {ID: "B", Buffer: 0}},
		Jobs: []Job{
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 2}, {Station: "B", Duration: 1}}},
			{ID: "J2", Release: 0, Route: []Operation{{Station: "A", Duration: 1}}},
			{ID: "J3", Release: 0, Route: []Operation{{Station: "B", Duration: 5}}},
		},
	}
	s, sum := runCase(t, in)
	if sum.Status != "completed" {
		t.Fatalf("status = %s, want completed", sum.Status)
	}
	// J1 ends A at 2, B is busy until 5: blocked 3 time units.
	if got := findJob(sum, "J1").Blocked; got != 3 {
		t.Errorf("J1 blocked = %d, want 3", got)
	}
	// J2 cannot enter A until J1 leaves at 5.
	j2 := findJob(sum, "J2")
	if j2.Queue != 5 {
		t.Errorf("J2 queue = %d, want 5", j2.Queue)
	}
	if j2.Completion != 6 {
		t.Errorf("J2 completion = %d, want 6", j2.Completion)
	}
	// Direct handoff at t=5: J1 leaves A and J2 starts immediately.
	var j1op0, j2op0 *OpRecord
	for i := range s.records {
		r := &s.records[i]
		if r.JobID == "J1" && r.OpIndex == 0 {
			j1op0 = r
		}
		if r.JobID == "J2" {
			j2op0 = r
		}
	}
	if j1op0.Leave != 5 || j2op0.Start != 5 {
		t.Errorf("handoff: J1 leave=%d J2 start=%d, want both 5", j1op0.Leave, j2op0.Start)
	}
}

// Opposite routes with zero buffers deadlock: each machine holds a
// part that needs the other machine, and no swap is allowed.
func TestOppositeRoutesStall(t *testing.T) {
	in := &Input{
		Stations: []Station{{ID: "A", Buffer: 0}, {ID: "B", Buffer: 0}},
		Jobs: []Job{
			{ID: "J1", Release: 0, Route: []Operation{{Station: "A", Duration: 1}, {Station: "B", Duration: 1}}},
			{ID: "J2", Release: 0, Route: []Operation{{Station: "B", Duration: 1}, {Station: "A", Duration: 1}}},
		},
	}
	_, sum := runCase(t, in)
	if sum.Status != "stalled" {
		t.Fatalf("status = %s, want stalled", sum.Status)
	}
	if sum.Stall.Time != 1 {
		t.Errorf("stall time = %d, want 1", sum.Stall.Time)
	}
	if len(sum.Stall.Occupancy) != 2 {
		t.Fatalf("occupancy = %+v, want 2 entries", sum.Stall.Occupancy)
	}
	occ := map[string]Occupancy{}
	for _, o := range sum.Stall.Occupancy {
		occ[o.StationID] = o
	}
	if occ["A"].JobID != "J1" || occ["A"].Next != "B" {
		t.Errorf("station A occupancy = %+v", occ["A"])
	}
	if occ["B"].JobID != "J2" || occ["B"].Next != "A" {
		t.Errorf("station B occupancy = %+v", occ["B"])
	}
	if findJob(sum, "J1").Finished || findJob(sum, "J2").Finished {
		t.Error("stalled jobs must not be marked finished")
	}
}
