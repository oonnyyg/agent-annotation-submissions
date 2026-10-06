package workshop

import (
	"strings"
	"testing"
)

func load(t *testing.T, doc string) *Input {
	t.Helper()
	in, err := LoadInput(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("LoadInput: %v", err)
	}
	return in
}

func findRecord(t *testing.T, res *Result, job string, op int) OpRecord {
	t.Helper()
	for _, r := range res.Records {
		if r.JobID == job && r.OpIndex == op {
			return r
		}
	}
	t.Fatalf("no record for job %s op %d", job, op)
	return OpRecord{}
}

func TestZeroBufferBlocking(t *testing.T) {
	in := load(t, `{
	  "stations": [{"id":"S1","buffer":0},{"id":"S2","buffer":0}],
	  "jobs": [
	    {"id":"J1","release":0,"route":[{"station":"S1","duration":2},{"station":"S2","duration":3}]},
	    {"id":"J2","release":0,"route":[{"station":"S1","duration":1},{"station":"S2","duration":3}]}
	  ]
	}`)
	res := Simulate(in)
	if !res.Completed || res.Stalled {
		t.Fatalf("expected completion, got %+v", res)
	}
	// J1: S1 [0,2], S2 [2,5]; J2 waits outside until S1 frees at 2,
	// runs [2,3], then blocks S1 until S2 frees at 5, runs S2 [5,8].
	r := findRecord(t, res, "J2", 0)
	if r.Start != 2 || r.End != 3 || r.Leave != 5 || r.BlockedAfter != 2 {
		t.Fatalf("J2 op0: %+v", r)
	}
	r = findRecord(t, res, "J2", 1)
	if r.Start != 5 || r.End != 8 {
		t.Fatalf("J2 op1: %+v", r)
	}
	if res.Makespan != 8 {
		t.Fatalf("makespan = %d, want 8", res.Makespan)
	}
	var j2 JobSummary
	for _, j := range res.Jobs {
		if j.ID == "J2" {
			j2 = j
		}
	}
	if j2.Queue != 2 || j2.Process != 4 || j2.Blocked != 2 {
		t.Fatalf("J2 summary: %+v", j2)
	}
}

func TestOpposingRoutesDeadlock(t *testing.T) {
	in := load(t, `{
	  "stations": [{"id":"S1","buffer":0},{"id":"S2","buffer":0}],
	  "jobs": [
	    {"id":"J1","release":0,"route":[{"station":"S1","duration":2},{"station":"S2","duration":1}]},
	    {"id":"J2","release":0,"route":[{"station":"S2","duration":2},{"station":"S1","duration":1}]}
	  ]
	}`)
	res := Simulate(in)
	if res.Completed || !res.Stalled {
		t.Fatalf("expected stall, got %+v", res)
	}
	if res.StallTime != 2 {
		t.Fatalf("stall time = %d, want 2", res.StallTime)
	}
	if res.Occupancy["S1"] != "J1" || res.Occupancy["S2"] != "J2" {
		t.Fatalf("occupancy: %v", res.Occupancy)
	}
	for _, j := range res.Jobs {
		if j.Status != "stalled" || j.CompletedOps != 1 {
			t.Fatalf("job summary: %+v", j)
		}
	}
}

func TestQueueOrderAndBufferCapacity(t *testing.T) {
	in := load(t, `{
	  "stations": [{"id":"S1","buffer":1},{"id":"S2","buffer":10}],
	  "jobs": [
	    {"id":"J1","release":0,"route":[{"station":"S1","duration":2},{"station":"S2","duration":1}]},
	    {"id":"J2","release":0,"route":[{"station":"S1","duration":2},{"station":"S2","duration":1}]},
	    {"id":"J3","release":0,"route":[{"station":"S1","duration":2},{"station":"S2","duration":1}]}
	  ]
	}`)
	res := Simulate(in)
	if !res.Completed {
		t.Fatalf("expected completion, got %+v", res)
	}
	// J3 cannot jump the queue: J2 starts at 2, J3 at 4.
	if r := findRecord(t, res, "J2", 0); r.Start != 2 {
		t.Fatalf("J2 op0 start = %d, want 2", r.Start)
	}
	if r := findRecord(t, res, "J3", 0); r.Start != 4 || r.Queue != 4 {
		t.Fatalf("J3 op0: %+v", findRecord(t, res, "J3", 0))
	}
	if res.Makespan != 7 {
		t.Fatalf("makespan = %d, want 7", res.Makespan)
	}
}

func TestSameTimeReleaseOrderByJobID(t *testing.T) {
	in := load(t, `{
	  "stations": [{"id":"S1","buffer":5}],
	  "jobs": [
	    {"id":"B","release":0,"route":[{"station":"S1","duration":1}]},
	    {"id":"A","release":0,"route":[{"station":"S1","duration":1}]}
	  ]
	}`)
	res := Simulate(in)
	// Transfers run in job-id order, so A enters the machine first.
	if r := findRecord(t, res, "A", 0); r.Start != 0 {
		t.Fatalf("A start = %d, want 0", r.Start)
	}
	if r := findRecord(t, res, "B", 0); r.Start != 1 {
		t.Fatalf("B start = %d, want 1", r.Start)
	}
}

func TestValidation(t *testing.T) {
	cases := []string{
		`{"stations":[{"id":"S1","buffer":0},{"id":"S1","buffer":1}],"jobs":[]}`,
		`{"stations":[{"id":"S1","buffer":-1}],"jobs":[]}`,
		`{"stations":[{"id":"S1","buffer":0}],"jobs":[{"id":"J","release":0,"route":[{"station":"S9","duration":1}]}]}`,
		`{"stations":[{"id":"S1","buffer":0}],"jobs":[{"id":"J","release":0,"route":[{"station":"S1","duration":0}]}]}`,
		`{"stations":[{"id":"S1","buffer":0}],"jobs":[{"id":"J","release":-1,"route":[{"station":"S1","duration":1}]}]}`,
		`{"stations":[{"id":"S1","buffer":0}],"jobs":[{"id":"J","release":0,"route":[]}]}`,
		`{"stations":[{"id":"S1","buffer":0}],"jobs":[{"id":"J","release":0,"route":[{"station":"S1","duration":1}]},{"id":"J","release":0,"route":[{"station":"S1","duration":1}]}]}`,
	}
	for i, doc := range cases {
		if _, err := LoadInput(strings.NewReader(doc)); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestRevisitStation(t *testing.T) {
	in := load(t, `{
	  "stations": [{"id":"S1","buffer":0},{"id":"S2","buffer":0}],
	  "jobs": [
	    {"id":"J1","release":0,"route":[
	      {"station":"S1","duration":1},
	      {"station":"S2","duration":1},
	      {"station":"S1","duration":1}
	    ]}
	  ]
	}`)
	res := Simulate(in)
	if !res.Completed || res.Makespan != 3 {
		t.Fatalf("got %+v", res)
	}
	if len(res.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(res.Records))
	}
}
