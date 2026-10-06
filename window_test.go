package main

import (
	"strings"
	"testing"
)

func load(t *testing.T, lines ...string) map[string][]*Snapshot {
	t.Helper()
	m, err := LoadSnapshots(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("LoadSnapshots: %v", err)
	}
	return m
}

func TestUnorderedInputAndMerge(t *testing.T) {
	// Two instances, lines deliberately out of order.
	m := load(t,
		`{"instance":"b","ts":20,"boot":"b1","bounds":[1,10],"counts":[3,6,7],"sum":14}`,
		`{"instance":"a","ts":20,"boot":"a1","bounds":[1,10],"counts":[2,4,5],"sum":9}`,
		`{"instance":"b","ts":10,"boot":"b1","bounds":[1,10],"counts":[1,2,2],"sum":4}`,
		`{"instance":"a","ts":10,"boot":"a1","bounds":[1,10],"counts":[0,1,1],"sum":2}`,
	)
	rep, err := MergeWindow(m, 10, 20)
	if err != nil {
		t.Fatalf("MergeWindow: %v", err)
	}
	// cumulative deltas: a [2,3,4] sum 7; b [2,4,5] sum 10
	want := []uint64{4, 7, 9}
	for i := range want {
		if rep.BucketCounts[i] != want[i] {
			t.Errorf("bucket %d = %d, want %d", i, rep.BucketCounts[i], want[i])
		}
	}
	if rep.TotalCount != 9 || rep.TotalSum != 17 {
		t.Errorf("total count/sum = %d/%v, want 9/17", rep.TotalCount, rep.TotalSum)
	}
	if got := rep.RequestsPerSecond(); got != 0.9 {
		t.Errorf("rps = %v, want 0.9", got)
	}
	if avg, ok := rep.AvgLatency(); !ok || avg != 17.0/9.0 {
		t.Errorf("avg = %v,%v want %v,true", avg, ok, 17.0/9.0)
	}
	if len(rep.RestartGaps) != 0 {
		t.Errorf("unexpected restart gaps: %v", rep.RestartGaps)
	}
}

func TestRestartCountsNewSnapshotOnly(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":10,"boot":"old","bounds":[1,10],"counts":[5,8,10],"sum":20}`,
		`{"instance":"a","ts":20,"boot":"new","bounds":[1,10],"counts":[1,2,3],"sum":6}`,
		`{"instance":"a","ts":30,"boot":"new","bounds":[1,10],"counts":[2,5,7],"sum":15}`,
	)
	rep, err := MergeWindow(m, 10, 30)
	if err != nil {
		t.Fatalf("MergeWindow: %v", err)
	}
	// Restart interval counts the new snapshot as-is: [1,2,3] sum 6.
	// Next interval diffs within same boot: [1,3,4] sum 9.
	want := []uint64{2, 5, 7}
	for i := range want {
		if rep.BucketCounts[i] != want[i] {
			t.Errorf("bucket %d = %d, want %d", i, rep.BucketCounts[i], want[i])
		}
	}
	if rep.TotalSum != 15 {
		t.Errorf("sum = %v, want 15", rep.TotalSum)
	}
	if len(rep.RestartGaps) != 1 || rep.RestartGaps[0].Start != 10 || rep.RestartGaps[0].End != 20 {
		t.Errorf("restart gaps = %+v", rep.RestartGaps)
	}
}

func TestMissingEndpointAndBoundsMismatch(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[1,1],"sum":1}`,
		`{"instance":"a","ts":20,"boot":"x","bounds":[1],"counts":[2,2],"sum":2}`,
		`{"instance":"b","ts":10,"boot":"y","bounds":[1],"counts":[0,0],"sum":0}`,
	)
	if _, err := MergeWindow(m, 10, 20); err == nil {
		t.Fatal("expected error for missing endpoint on instance b")
	}
	m2 := load(t,
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[1,1],"sum":1}`,
		`{"instance":"a","ts":20,"boot":"x","bounds":[2],"counts":[2,2],"sum":2}`,
	)
	if _, err := MergeWindow(m2, 10, 20); err == nil {
		t.Fatal("expected error for inconsistent bounds")
	}
}

func TestNegativeDeltaWithoutRestartRejected(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[5,5],"sum":9}`,
		`{"instance":"a","ts":20,"boot":"x","bounds":[1],"counts":[3,3],"sum":9}`,
	)
	if _, err := MergeWindow(m, 10, 20); err == nil {
		t.Fatal("expected error for decreasing cumulative count")
	}
}

func TestDuplicateAndInvalidInput(t *testing.T) {
	dup := strings.Join([]string{
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[1,1],"sum":1}`,
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[1,1],"sum":1}`,
	}, "\n")
	if _, err := LoadSnapshots(strings.NewReader(dup)); err == nil {
		t.Fatal("expected duplicate rejection")
	}
	if _, err := LoadSnapshots(strings.NewReader(
		`{"instance":"a","ts":10,"boot":"x","bounds":[],"counts":[0],"sum":0}`)); err == nil {
		t.Fatal("expected error: at least one finite bucket required")
	}
	if _, err := LoadSnapshots(strings.NewReader(
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[0,0],"sum":3}`)); err == nil {
		t.Fatal("expected error: nonzero sum with zero total")
	}
}
