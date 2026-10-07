
package orset

import (
	"path/filepath"
	"reflect"
	"testing"
)

func mustNew(t *testing.T, id string) *State {
	t.Helper()
	s, err := New(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReAddProducesDistinctDots(t *testing.T) {
	s := mustNew(t, "A")
	d1, _ := s.Add("x")
	d2, _ := s.Add("x")
	if d1 == d2 {
		t.Fatalf("re-add reused dot %v", d1)
	}
	if got := s.Tags(); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("tags = %v", got)
	}
}

// Concurrent add on B must survive A's remove of the same tag.
func TestConcurrentAddSurvivesRemove(t *testing.T) {
	a, b := mustNew(t, "A"), mustNew(t, "B")
	a.Add("x")
	if err := b.Merge(a.Export()); err != nil {
		t.Fatal(err)
	}
	// Now both see "x". A removes it; concurrently B re-adds it.
	a.Remove("x")
	b.Add("x")
	// Exchange snapshots both ways.
	sa, sb := a.Export(), b.Export()
	if err := a.Merge(sb); err != nil {
		t.Fatal(err)
	}
	if err := b.Merge(sa); err != nil {
		t.Fatal(err)
	}
	if !a.Visible("x") || !b.Visible("x") {
		t.Fatalf("concurrent add lost: A=%v B=%v", a.Explain("x"), b.Explain("x"))
	}
}

// Remove evidence arriving before the corresponding add must not resurrect
// the tag, and duplicate/out-of-order merges must converge.
func TestOutOfOrderAndDuplicateMerges(t *testing.T) {
	a, b := mustNew(t, "A"), mustNew(t, "B")
	a.Add("x")
	addSnap := a.Export()
	a.Remove("x")
	rmSnap := a.Export()

	// B receives removal evidence first, then the add, then duplicates.
	for _, snap := range []Snapshot{rmSnap, addSnap, rmSnap, addSnap} {
		if err := b.Merge(snap); err != nil {
			t.Fatal(err)
		}
	}
	if b.Visible("x") {
		t.Fatalf("tag resurrected: %v", b.Explain("x"))
	}
	// A merges B back; both must agree.
	if err := a.Merge(b.Export()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Tags(), b.Tags()) {
		t.Fatalf("diverged: A=%v B=%v", a.Tags(), b.Tags())
	}
}

func TestMergeOrderIndependence(t *testing.T) {
	a, b, c := mustNew(t, "A"), mustNew(t, "B"), mustNew(t, "C")
	a.Add("one")
	b.Add("two")
	a.Add("three")
	b.Remove("two") // removes only B's own observed add
	snaps := []Snapshot{a.Export(), b.Export()}
	for _, s := range snaps {
		c.Merge(s)
	}
	d := mustNew(t, "D")
	for i := len(snaps) - 1; i >= 0; i-- {
		d.Merge(snaps[i])
	}
	if !reflect.DeepEqual(c.Tags(), d.Tags()) {
		t.Fatalf("order changed result: %v vs %v", c.Tags(), d.Tags())
	}
	if !reflect.DeepEqual(c.Tags(), []string{"one", "three"}) {
		t.Fatalf("tags = %v", c.Tags())
	}
}

func TestConflictingDotRejectedAtomically(t *testing.T) {
	a, b := mustNew(t, "A"), mustNew(t, "B")
	a.Add("x")
	b.Add("y")
	// Forge a snapshot binding A's dot to a different tag.
	dot := a.Export().Adds["x"][0]
	if err := b.Merge(a.Export()); err != nil {
		t.Fatal(err)
	}
	bad := Snapshot{Adds: map[string][]Dot{"z": {dot}}}
	before := b.Export()
	if err := b.Merge(bad); err == nil {
		t.Fatal("expected conflict rejection")
	}
	if !reflect.DeepEqual(before.Adds, b.Export().Adds) {
		t.Fatal("rejected merge mutated state")
	}
}

func TestMalformedSnapshotRejected(t *testing.T) {
	s := mustNew(t, "A")
	if err := s.MergeJSON([]byte(`{"adds": "nope"}`)); err == nil {
		t.Fatal("expected malformed snapshot error")
	}
	if err := s.MergeJSON([]byte(`{"adds": {"x": [{"replica": "B", "seq": 0}]}}`)); err == nil {
		t.Fatal("expected invalid dot error")
	}
}

func TestRestartPersistsIdentityAndSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := mustNew(t, "A")
	s.Add("x")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := loaded.Add("y")
	if d.Replica != "A" || d.Seq != 2 {
		t.Fatalf("restarted replica produced %v, want A#2", d)
	}
	// Old snapshot merged after restart must not roll back progress.
	old := s.Export()
	if err := loaded.Merge(old); err != nil {
		t.Fatal(err)
	}
	if got := loaded.Tags(); !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("tags = %v", got)
	}
	if _, err := Load(path, "B"); err == nil {
		t.Fatal("expected identity mismatch error")
	}
}

func TestExplainEvidence(t *testing.T) {
	a := mustNew(t, "A")
	a.Add("x")
	a.Add("x")
	a.Remove("x")
	ev := a.Explain("x")
	if ev.Visible || len(ev.Removed) != 2 || len(ev.Live) != 0 {
		t.Fatalf("evidence = %+v", ev)
	}
	a.Add("x")
	ev = a.Explain("x")
	if !ev.Visible || len(ev.Live) != 1 || len(ev.Removed) != 2 {
		t.Fatalf("evidence = %+v", ev)
	}
}
