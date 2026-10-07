package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRestartKeepsIdentityAndSeq(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadStore(dir, "A")
	if err != nil {
		t.Fatal(err)
	}
	s.Add("x")
	s.Add("y")
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}

	// "Restart": reload from disk in a fresh Store.
	s2, err := LoadStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Replica != "A" || s2.NextSeq != 3 {
		t.Fatalf("reloaded replica=%q next=%d, want A/3", s2.Replica, s2.NextSeq)
	}
	ev := s2.Add("z")
	if ev.Mark.Seq != 3 {
		t.Fatalf("mark seq reused after restart: %+v", ev.Mark)
	}
	if got := s2.VisibleTags(); !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
		t.Fatalf("visible = %v", got)
	}
}

func TestOldSnapshotDoesNotRegressProgress(t *testing.T) {
	dir := t.TempDir()
	s, _ := LoadStore(dir, "A")
	s.Add("x")
	old := s.ExportSnapshot()
	s.Add("y")
	s.Remove("x")
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}
	s2, _ := LoadStore(dir, "")
	if err := s2.MergeSnapshot(old); err != nil {
		t.Fatal(err)
	}
	if s2.NextSeq != 3 {
		t.Fatalf("NextSeq regressed to %d", s2.NextSeq)
	}
	if got := s2.VisibleTags(); !reflect.DeepEqual(got, []string{"y"}) {
		t.Fatalf("visible = %v, want [y]", got)
	}
}

func TestLoadStoreRejectsCorruptState(t *testing.T) {
	dir := t.TempDir()
	s, _ := LoadStore(dir, "A")
	s.Add("x")
	if err := SaveStore(dir, s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	bad := []byte(`{"replica":"A","next_seq":2,"adds":[{"mark":{"replica":"A","seq":1},"tag":"x"},{"mark":{"replica":"A","seq":1},"tag":"y"}],"removes":[]}`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStore(dir, ""); err == nil {
		t.Fatal("expected corrupt state to be rejected")
	}
}
