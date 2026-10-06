
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func rev(device, unit, from, to string) Revision {
	return Revision{Device: device, Unit: unit, From: from, To: to}
}

func mustApply(t *testing.T, r *Register, at int64, revs ...Revision) {
	t.Helper()
	if err := r.Apply(&Batch{RecordedAt: at, Revisions: revs}); err != nil {
		t.Fatalf("apply at %d: %v", at, err)
	}
}

func segStrings(tl []Segment) []string {
	out := []string{}
	for _, s := range tl {
		out = append(out, s.From.String()+"|"+s.To.String()+"|"+s.Unit)
	}
	return out
}

// Cross-span override: a later revision covering the middle of an earlier
// span keeps both uncovered ends, and a gap-filling revision merges with
// equal-unit neighbours.
func TestCrossSpanOverrideAndGapFill(t *testing.T) {
	r := NewRegister()
	mustApply(t, r, 100, rev("dev1", "alpha", "2026-01-01", "2026-12-31"))
	mustApply(t, r, 200, rev("dev1", "beta", "2026-03-01", "2026-06-01"))

	got := segStrings(r.Timeline("dev1", 200))
	want := []string{
		"2026-01-01|2026-03-01|alpha",
		"2026-03-01|2026-06-01|beta",
		"2026-06-01|2026-12-31|alpha",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	// Earlier knowledge cutoff is unaffected by the later revision.
	got = segStrings(r.Timeline("dev1", 150))
	want = []string{"2026-01-01|2026-12-31|alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("as-of 150: got %v want %v", got, want)
	}

	// Fill a gap with the same unit as a neighbour: results merge.
	mustApply(t, r, 300, rev("dev1", "beta", "2026-02-01", "2026-03-01"))
	got = segStrings(r.Timeline("dev1", 300))
	want = []string{
		"2026-01-01|2026-02-01|alpha",
		"2026-02-01|2026-06-01|beta",
		"2026-06-01|2026-12-31|alpha",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gap fill: got %v want %v", got, want)
	}
}

// Retroactive revoke: a late-recorded empty-unit revision erases a past
// span for queries at later knowledge cutoffs, but earlier-cutoff answers
// are unchanged.
func TestRetroactiveRevoke(t *testing.T) {
	r := NewRegister()
	mustApply(t, r, 100, rev("dev1", "alpha", "2026-01-01", "2026-04-01"))
	mustApply(t, r, 500, rev("dev1", "", "2026-02-01", "2026-03-01"))

	got := segStrings(r.Timeline("dev1", 500))
	want := []string{
		"2026-01-01|2026-02-01|alpha",
		"2026-03-01|2026-04-01|alpha",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after revoke: got %v want %v", got, want)
	}
	got = segStrings(r.Timeline("dev1", 100))
	want = []string{"2026-01-01|2026-04-01|alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("before revoke knowledge: got %v want %v", got, want)
	}

	changes := r.Diff("dev1", 100, 500)
	if len(changes) != 1 {
		t.Fatalf("diff len %d, want 1: %+v", len(changes), changes)
	}
	c := changes[0]
	if c.From.String() != "2026-02-01" || c.To.String() != "2026-03-01" || c.Before != "alpha" || c.After != "" {
		t.Fatalf("unexpected change %+v", c)
	}
}

// Batch failure: invalid batch leaves register and last recorded_at
// untouched; overlapping spans in one batch are rejected.
func TestBatchFailureAtomic(t *testing.T) {
	r := NewRegister()
	mustApply(t, r, 100, rev("dev1", "alpha", "2026-01-01", "2026-02-01"))

	bad := &Batch{RecordedAt: 200, Revisions: []Revision{
		rev("dev2", "beta", "2026-01-01", "2026-02-01"),
		rev("dev2", "gamma", "2026-01-15", "2026-03-01"), // overlaps previous
	}}
	if err := r.Apply(bad); err == nil {
		t.Fatal("expected overlap error")
	}
	if tl := r.Timeline("dev2", 1000); len(tl) != 0 {
		t.Fatalf("failed batch leaked revisions: %v", segStrings(tl))
	}
	if at, ok := r.LastRecordedAt(); !ok || at != 100 {
		t.Fatalf("last recorded_at = %d,%v want 100,true", at, ok)
	}

	// Non-increasing recorded_at rejected.
	if err := r.Apply(&Batch{RecordedAt: 100, Revisions: []Revision{rev("d", "u", "2026-01-01", "2026-01-02")}}); err == nil {
		t.Fatal("expected recorded_at monotonicity error")
	}
	// Inverted span rejected.
	if err := r.Apply(&Batch{RecordedAt: 300, Revisions: []Revision{rev("d", "u", "2026-01-02", "2026-01-01")}}); err == nil {
		t.Fatal("expected inverted span error")
	}
	// Bad date rejected.
	if err := r.Apply(&Batch{RecordedAt: 300, Revisions: []Revision{rev("d", "u", "2026-13-01", "2026-12-01")}}); err == nil {
		t.Fatal("expected date parse error")
	}
}

// Diff merges adjacent equal changes and reports minimal segments.
func TestDiffMergesAdjacent(t *testing.T) {
	r := NewRegister()
	mustApply(t, r, 100, rev("dev1", "alpha", "2026-01-01", "2026-06-01"))
	mustApply(t, r, 200,
		rev("dev1", "beta", "2026-02-01", "2026-03-01"),
		rev("dev1", "beta", "2026-03-01", "2026-04-01"),
	)
	changes := r.Diff("dev1", 100, 200)
	if len(changes) != 1 {
		t.Fatalf("want 1 merged change, got %+v", changes)
	}
	c := changes[0]
	if c.From.String() != "2026-02-01" || c.To.String() != "2026-04-01" || c.Before != "alpha" || c.After != "beta" {
		t.Fatalf("unexpected %+v", c)
	}
}

// Archive round-trip preserves history queries and diffs; corrupt archives
// are rejected.
func TestArchiveRoundTripAndCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.json")

	r := NewRegister()
	batches := []*Batch{
		{RecordedAt: 100, Revisions: []Revision{rev("dev1", "alpha", "2026-01-01", "2026-06-01")}},
		{RecordedAt: 200, Revisions: []Revision{rev("dev1", "", "2026-02-01", "2026-03-01")}},
	}
	for _, b := range batches {
		if err := r.Apply(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := SaveArchive(path, batches); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := LoadArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, cutoff := range []int64{50, 100, 150, 200, 1000} {
		got, want := segStrings(loaded.Timeline("dev1", cutoff)), segStrings(r.Timeline("dev1", cutoff))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cutoff %d: got %v want %v", cutoff, got, want)
		}
	}
	if !reflect.DeepEqual(loaded.Diff("dev1", 100, 200), r.Diff("dev1", 100, 200)) {
		t.Fatal("diff mismatch after reload")
	}

	// Tamper with the archive: must be rejected.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := string(data)
	tampered = tampered[:len(tampered)/2] + "X" + tampered[len(tampered)/2+1:]
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadArchive(path); err == nil {
		t.Fatal("expected corrupt archive to be rejected")
	}
}
