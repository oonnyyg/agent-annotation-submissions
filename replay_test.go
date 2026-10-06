package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const drillInput = `{"op":"begin"}
{"op":"begin"}
{"op":"lock","tx":1,"resource":"a","mode":"S"}
{"op":"lock","tx":2,"resource":"a","mode":"S"}
{"op":"lock","tx":1,"resource":"a","mode":"X"}
{"op":"lock","tx":2,"resource":"a","mode":"X"}
{"op":"status"}
{"op":"commit","tx":1}
{"op":"status"}
`

func runReplay(t *testing.T, input string) []Result {
	t.Helper()
	var buf bytes.Buffer
	if err := Replay(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("replay: %v", err)
	}
	var results []Result
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var r Result
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("decode result %q: %v", line, err)
		}
		results = append(results, r)
	}
	return results
}

func TestReplayDeterministic(t *testing.T) {
	var a, b bytes.Buffer
	if err := Replay(strings.NewReader(drillInput), &a); err != nil {
		t.Fatal(err)
	}
	if err := Replay(strings.NewReader(drillInput), &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatal("replaying the same input must produce identical output")
	}
}

func TestReplayUpgradeDeadlock(t *testing.T) {
	results := runReplay(t, drillInput)
	if len(results) != 9 {
		t.Fatalf("expected 9 results, got %d", len(results))
	}
	// Event 6 triggers the deadlock; victim is tx 2 (started later).
	r6 := results[5]
	if len(r6.Deadlock) != 1 {
		t.Fatalf("event 6 should report a deadlock, got %+v", r6)
	}
	if r6.Deadlock[0].Victim != 2 {
		t.Fatalf("victim should be tx 2, got %d", r6.Deadlock[0].Victim)
	}
	if len(r6.Deadlock[0].EdgesBefore) == 0 {
		t.Fatal("deadlock report must keep pre-resolution edges")
	}
	// Status after resolution: tx 2 gone everywhere.
	snap := results[8].Snapshot
	if snap == nil {
		t.Fatal("final status must carry a snapshot")
	}
	if _, ok := snap.Transactions["2"]; ok {
		t.Fatal("victim must not appear in transactions")
	}
	if _, ok := snap.Holders["a"]["2"]; ok {
		t.Fatal("victim must not hold locks")
	}
	for _, q := range snap.Queues {
		for _, req := range q {
			if req.Tx == 2 {
				t.Fatal("victim must not remain in queues")
			}
		}
	}
	// tx 1 commits cleanly at the end.
	if !results[7].OK {
		t.Fatalf("tx 1 commit should succeed: %+v", results[7])
	}
}

func TestReplayWaitingAndErrors(t *testing.T) {
	input := `{"op":"begin"}
{"op":"begin"}
{"op":"lock","tx":1,"resource":"a","mode":"X"}
{"op":"lock","tx":2,"resource":"a","mode":"X"}
{"op":"lock","tx":2,"resource":"b","mode":"S"}
{"op":"bogus"}
{"op":"rollback","tx":2}
{"op":"commit","tx":1}
`
	results := runReplay(t, input)
	if !results[3].Waiting {
		t.Fatalf("event 4 should be waiting, got %+v", results[3])
	}
	if results[4].OK {
		t.Fatal("waiting tx requesting another lock must fail")
	}
	if results[5].OK {
		t.Fatal("unknown op must fail")
	}
	if !results[6].OK {
		t.Fatal("waiting tx rollback must succeed")
	}
	if !results[7].OK {
		t.Fatal("commit must succeed")
	}
}
