package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func mustSubmit(t *testing.T, r *Register, at int64, items ...Revision) {
	t.Helper()
	if err := r.Submit(Batch{RecordTime: at, Items: items}); err != nil {
		t.Fatalf("submit at %d: %v", at, err)
	}
}

func TestCrossSegmentOverride(t *testing.T) {
	r := NewRegister()
	mustSubmit(t, r, 100,
		Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-02-01"},
		Revision{Device: "d1", Unit: "B", From: "2026-03-01", To: "2026-04-01"},
	)
	// One revision spanning an existing segment, a gap, and another segment.
	mustSubmit(t, r, 200,
		Revision{Device: "d1", Unit: "C", From: "2026-01-15", To: "2026-03-15"},
	)
	got := r.Timeline("d1", 200)
	want := []Span{
		{From: "2026-01-01", To: "2026-01-15", Unit: "A"},
		{From: "2026-01-15", To: "2026-03-15", Unit: "C"},
		{From: "2026-03-15", To: "2026-04-01", Unit: "B"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline mismatch:\n got %+v\nwant %+v", got, want)
	}
	// Earlier cutoff still sees the old knowledge.
	got = r.Timeline("d1", 150)
	want = []Span{
		{From: "2026-01-01", To: "2026-02-01", Unit: "A"},
		{From: "2026-03-01", To: "2026-04-01", Unit: "B"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline at 150 mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestRetroactiveRevocation(t *testing.T) {
	r := NewRegister()
	mustSubmit(t, r, 100, Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-03-01"})
	// Late correction: revoke the middle of January, recorded later.
	mustSubmit(t, r, 300, Revision{Device: "d1", Unit: "", From: "2026-01-10", To: "2026-01-20"})

	unit, found, err := r.OwnerAt("d1", "2026-01-15", 300)
	if err != nil || found || unit != "" {
		t.Fatalf("expected gap at 2026-01-15 as of 300, got %q found=%v err=%v", unit, found, err)
	}
	// Querying with an earlier knowledge cutoff is unaffected.
	unit, found, err = r.OwnerAt("d1", "2026-01-15", 299)
	if err != nil || !found || unit != "A" {
		t.Fatalf("expected A at 2026-01-15 as of 299, got %q found=%v err=%v", unit, found, err)
	}

	changes := r.Diff("d1", 299, 300)
	want := []Change{{From: "2026-01-10", To: "2026-01-20", Before: "A", After: ""}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("diff mismatch:\n got %+v\nwant %+v", changes, want)
	}
}

func TestBatchFailureIsAtomic(t *testing.T) {
	r := NewRegister()
	mustSubmit(t, r, 100, Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-02-01"})

	bad := Batch{RecordTime: 200, Items: []Revision{
		{Device: "d1", Unit: "B", From: "2026-01-05", To: "2026-01-10"},
		{Device: "d1", Unit: "C", From: "2026-01-08", To: "2026-01-12"}, // overlaps
	}}
	if err := r.Submit(bad); err == nil {
		t.Fatal("expected overlapping batch to fail")
	}
	if ts, ok := r.LastRecordTime(); !ok || ts != 100 {
		t.Fatalf("last record time advanced on failure: %d %v", ts, ok)
	}
	if got := r.Timeline("d1", 1000); len(got) != 1 || got[0].Unit != "A" {
		t.Fatalf("archive mutated by failed batch: %+v", got)
	}

	// Non-increasing record time also fails and changes nothing.
	err := r.Submit(Batch{RecordTime: 100, Items: []Revision{
		{Device: "d2", Unit: "X", From: "2026-01-01", To: "2026-01-02"},
	}})
	if err == nil {
		t.Fatal("expected non-increasing record time to fail")
	}
	if got := r.Timeline("d2", 1000); len(got) != 0 {
		t.Fatalf("archive mutated by failed batch: %+v", got)
	}

	// Invalid dates rejected.
	if err := r.Submit(Batch{RecordTime: 200, Items: []Revision{
		{Device: "d1", Unit: "B", From: "2026-02-30", To: "2026-03-01"},
	}}); err == nil {
		t.Fatal("expected invalid calendar date to fail")
	}
	if err := r.Submit(Batch{RecordTime: 200, Items: []Revision{
		{Device: "d1", Unit: "B", From: "2026-03-01", To: "2026-03-01"},
	}}); err == nil {
		t.Fatal("expected empty range to fail")
	}
}

func TestMergeAdjacentSameUnit(t *testing.T) {
	r := NewRegister()
	mustSubmit(t, r, 100, Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-01-10"})
	mustSubmit(t, r, 200, Revision{Device: "d1", Unit: "A", From: "2026-01-10", To: "2026-01-20"})
	got := r.Timeline("d1", 200)
	want := []Span{{From: "2026-01-01", To: "2026-01-20", Unit: "A"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected merged span, got %+v", got)
	}
	// Revision evidence is preserved.
	if len(r.batches) != 2 {
		t.Fatalf("revision evidence lost: %d batches", len(r.batches))
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reg.json")
	r := NewRegister()
	mustSubmit(t, r, 100, Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-02-01"})
	mustSubmit(t, r, 200, Revision{Device: "d1", Unit: "", From: "2026-01-15", To: "2026-01-20"})
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, cutoff := range []int64{50, 100, 150, 200, 1000} {
		if !reflect.DeepEqual(r.Timeline("d1", cutoff), loaded.Timeline("d1", cutoff)) {
			t.Fatalf("timeline differs after reload at cutoff %d", cutoff)
		}
	}
	if !reflect.DeepEqual(r.Diff("d1", 100, 200), loaded.Diff("d1", 100, 200)) {
		t.Fatal("diff differs after reload")
	}
}

func TestCorruptArchiveRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reg.json")
	r := NewRegister()
	mustSubmit(t, r, 100, Revision{Device: "d1", Unit: "A", From: "2026-01-01", To: "2026-02-01"})
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte inside the payload.
	for i, b := range data {
		if b == 'A' {
			data[i] = 'Z'
			break
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected corrupted archive to be rejected")
	}
	// Truncated file is also rejected.
	if err := os.WriteFile(path, data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected truncated archive to be rejected")
	}
}
