package main

import (
	"reflect"
	"testing"
)

func mergeAll(t *testing.T, s *Store, snaps ...Snapshot) {
	t.Helper()
	for _, snap := range snaps {
		if err := s.MergeSnapshot(snap); err != nil {
			t.Fatalf("merge: %v", err)
		}
	}
}

func TestConcurrentAddRemoveAddWins(t *testing.T) {
	a, b := NewStore("A"), NewStore("B")
	a.Add("x")
	// A syncs to B, then both act concurrently.
	mergeAll(t, b, a.ExportSnapshot())
	b.Remove("x") // B removes the only mark it has observed
	a.Add("x")    // A concurrently re-adds the same tag
	snapA, snapB := a.ExportSnapshot(), b.ExportSnapshot()
	mergeAll(t, a, snapB)
	mergeAll(t, b, snapA)
	want := []string{"x"}
	if got := a.VisibleTags(); !reflect.DeepEqual(got, want) {
		t.Fatalf("A visible = %v, want %v", got, want)
	}
	if got := b.VisibleTags(); !reflect.DeepEqual(got, want) {
		t.Fatalf("B visible = %v, want %v", got, want)
	}
}

func TestRemoveThenConcurrentAddSurvives(t *testing.T) {
	a, b := NewStore("A"), NewStore("B")
	a.Add("x")
	mergeAll(t, b, a.ExportSnapshot())
	b.Remove("x")
	a.Add("x") // unobserved by B's remove
	mergeAll(t, a, b.ExportSnapshot())
	mergeAll(t, b, a.ExportSnapshot())
	if got := a.VisibleTags(); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("visible = %v, want [x]", got)
	}
}

func TestDuplicateAddMarksIndependent(t *testing.T) {
	a := NewStore("A")
	e1 := a.Add("x")
	e2 := a.Add("x")
	if e1.Mark == e2.Mark {
		t.Fatal("duplicate add reused a mark")
	}
	a.Remove("x") // removes both observed marks
	if got := a.VisibleTags(); len(got) != 0 {
		t.Fatalf("visible = %v, want empty", got)
	}
}

func TestOutOfOrderAndDuplicateMerge(t *testing.T) {
	a, b := NewStore("A"), NewStore("B")
	a.Add("x")
	a.Add("y")
	full := a.ExportSnapshot()
	a.Remove("x")
	removalOnly := Snapshot{Replica: "A", Adds: nil, Removes: a.ExportSnapshot().Removes}

	// Removal evidence arrives before the corresponding adds.
	mergeAll(t, b, removalOnly)
	mergeAll(t, b, full)       // adds arrive later; x must stay removed
	mergeAll(t, b, removalOnly) // duplicate delivery is a no-op
	mergeAll(t, b, full)
	if got := b.VisibleTags(); !reflect.DeepEqual(got, []string{"y"}) {
		t.Fatalf("visible = %v, want [y]", got)
	}
}

func TestMergeOrderIndependence(t *testing.T) {
	a, b, c := NewStore("A"), NewStore("B"), NewStore("C")
	a.Add("x")
	b.Add("y")
	mergeAll(t, c, a.ExportSnapshot())
	c.Remove("x")
	snaps := []Snapshot{a.ExportSnapshot(), b.ExportSnapshot(), c.ExportSnapshot()}

	d := NewStore("D")
	mergeAll(t, d, snaps...)
	e := NewStore("E")
	mergeAll(t, e, snaps[2], snaps[0], snaps[1])
	if !reflect.DeepEqual(d.VisibleTags(), e.VisibleTags()) {
		t.Fatalf("order changed result: %v vs %v", d.VisibleTags(), e.VisibleTags())
	}
	if !reflect.DeepEqual(d.VisibleTags(), []string{"y"}) {
		t.Fatalf("visible = %v, want [y]", d.VisibleTags())
	}
}

func TestConflictingMarkRejected(t *testing.T) {
	a := NewStore("A")
	a.Add("x")
	bad := Snapshot{Replica: "B", Adds: []AddEvidence{
		{Mark: Mark{Replica: "A", Seq: 1}, Tag: "different"},
	}}
	if err := a.MergeSnapshot(bad); err == nil {
		t.Fatal("expected conflict rejection")
	}
	if got := a.VisibleTags(); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("state mutated after rejected merge: %v", got)
	}
}

func TestExplainEvidence(t *testing.T) {
	a := NewStore("A")
	a.Add("x")
	a.Add("y")
	a.Remove("x")
	ex := a.Explain("")
	if len(ex) != 2 {
		t.Fatalf("explain returned %d entries, want 2", len(ex))
	}
	if ex[0].Tag != "x" || ex[0].Visible || len(ex[0].RemovedAdds) != 1 {
		t.Fatalf("bad evidence for x: %+v", ex[0])
	}
	if ex[1].Tag != "y" || !ex[1].Visible || len(ex[1].LiveAdds) != 1 {
		t.Fatalf("bad evidence for y: %+v", ex[1])
	}
}
