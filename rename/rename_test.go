package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func dirState(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		state[e.Name()] = string(data)
	}
	return state
}

func mustPlan(t *testing.T, dir string, pairs [][2]string) *Plan {
	t.Helper()
	plan, err := BuildPlan(dir, pairs)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestSwapCycle(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB", "c.txt": "CCC"})
	plan := mustPlan(t, dir, [][2]string{
		{"a.txt", "b.txt"}, {"b.txt", "c.txt"}, {"c.txt", "a.txt"},
	})
	res, err := Execute(plan, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a.txt": "CCC", "b.txt": "AAA", "c.txt": "BBB"}
	if got := dirState(t, dir); !equalState(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(res.Final) != 3 {
		t.Fatalf("expected 3 final entries, got %v", res.Final)
	}
}

func TestCaseOnlyRename(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"readme.md": "data"})
	plan := mustPlan(t, dir, [][2]string{{"readme.md", "README.md"}})
	if _, err := Execute(plan, 0); err != nil {
		t.Fatal(err)
	}
	state := dirState(t, dir)
	if len(state) != 1 || state["README.md"] != "data" {
		t.Fatalf("unexpected state %v", state)
	}
}

func TestSourceChangedAfterPreview(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB"})
	plan := mustPlan(t, dir, [][2]string{{"a.txt", "b.txt"}, {"b.txt", "a.txt"}})
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(plan, 0); err == nil {
		t.Fatal("expected rejection after source change")
	}
	want := map[string]string{"a.txt": "tampered", "b.txt": "BBB"}
	if got := dirState(t, dir); !equalState(got, want) {
		t.Fatalf("files must be untouched, got %v", got)
	}
}

func TestInjectedFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	orig := map[string]string{"a.txt": "AAA", "b.txt": "BBB", "c.txt": "CCC"}
	writeFiles(t, dir, orig)
	plan := mustPlan(t, dir, [][2]string{
		{"a.txt", "b.txt"}, {"b.txt", "c.txt"}, {"c.txt", "a.txt"},
	})
	// Fail before step 4 (first phase-2 rename): all sources are in temp names.
	res, err := Execute(plan, 4)
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if len(res.Steps) != 3 || len(res.Rollback) != 3 {
		t.Fatalf("steps=%v rollback=%v", res.Steps, res.Rollback)
	}
	if len(res.Orphans) != 0 {
		t.Fatalf("unexpected orphans %v", res.Orphans)
	}
	if got := dirState(t, dir); !equalState(got, orig) {
		t.Fatalf("rollback must restore original state, got %v", got)
	}
}

func TestInjectedFailureBeforeFirstStep(t *testing.T) {
	dir := t.TempDir()
	orig := map[string]string{"a.txt": "AAA"}
	writeFiles(t, dir, orig)
	plan := mustPlan(t, dir, [][2]string{{"a.txt", "b.txt"}})
	if _, err := Execute(plan, 1); err == nil {
		t.Fatal("expected injected failure")
	}
	if got := dirState(t, dir); !equalState(got, orig) {
		t.Fatalf("state changed: %v", got)
	}
}

func TestValidation(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB", "keep.txt": "K"})
	cases := []struct {
		name  string
		pairs [][2]string
	}{
		{"separator in target", [][2]string{{"a.txt", "sub/c.txt"}}},
		{"parent reference", [][2]string{{"a.txt", ".."}}},
		{"duplicate source case-insensitive", [][2]string{{"a.txt", "x"}, {"A.TXT", "y"}}},
		{"duplicate target case-insensitive", [][2]string{{"a.txt", "x.txt"}, {"b.txt", "X.TXT"}}},
		{"target exists outside batch", [][2]string{{"a.txt", "keep.txt"}}},
		{"missing source", [][2]string{{"nope.txt", "x.txt"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildPlan(dir, tc.pairs); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	// State must be untouched by failed previews.
	want := map[string]string{"a.txt": "AAA", "b.txt": "BBB", "keep.txt": "K"}
	if got := dirState(t, dir); !equalState(got, want) {
		t.Fatalf("preview modified files: %v", got)
	}
}

func TestPlanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA"})
	plan := mustPlan(t, dir, [][2]string{{"a.txt", "b.txt"}})
	var sb strings.Builder
	if err := SavePlan(plan, &sb); err != nil {
		t.Fatal(err)
	}
	pf := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(pf, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPlan(pf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(loaded, 0); err != nil {
		t.Fatal(err)
	}
	if got := dirState(t, dir); got["b.txt"] != "AAA" || len(got) != 1 {
		t.Fatalf("unexpected state %v", got)
	}
}

func equalState(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
