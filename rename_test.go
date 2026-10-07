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

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func TestSwapCycle(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB", "c.txt": "CCC"})

	plan, err := BuildPlan(dir, [][2]string{
		{"a.txt", "b.txt"}, {"b.txt", "c.txt"}, {"c.txt", "a.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Preview must not modify anything.
	if got := readFile(t, dir, "a.txt"); got != "AAA" {
		t.Fatalf("preview modified a.txt: %q", got)
	}
	res, err := Execute(plan, Options{FailBeforeStep: -1, FailRollbackStep: -1})
	if err != nil {
		t.Fatal(err)
	}
	if res.RolledBack || len(res.RollbackErrors) != 0 {
		t.Fatalf("unexpected rollback: %+v", res)
	}
	if got := readFile(t, dir, "a.txt"); got != "CCC" {
		t.Errorf("a.txt = %q, want CCC", got)
	}
	if got := readFile(t, dir, "b.txt"); got != "AAA" {
		t.Errorf("b.txt = %q, want AAA", got)
	}
	if got := readFile(t, dir, "c.txt"); got != "BBB" {
		t.Errorf("c.txt = %q, want BBB", got)
	}
	for _, n := range dirEntries(t, dir) {
		if strings.HasPrefix(n, ".tbr-") {
			t.Errorf("leftover temp file %q", n)
		}
	}
	if len(res.Final) != 3 {
		t.Errorf("final locations = %d, want 3", len(res.Final))
	}
}

func TestCaseOnlyRename(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"readme.txt": "data"})
	plan, err := BuildPlan(dir, [][2]string{{"readme.txt", "README.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(plan, Options{FailBeforeStep: -1, FailRollbackStep: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RollbackErrors) != 0 {
		t.Fatalf("rollback errors: %v", res.RollbackErrors)
	}
	if got := readFile(t, dir, "README.txt"); got != "data" {
		t.Errorf("README.txt = %q, want data", got)
	}
}

func TestSourceChangedAfterPreview(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB"})
	plan, err := BuildPlan(dir, [][2]string{{"a.txt", "b.txt"}, {"b.txt", "a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	// Mutate a source after preview.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("CHANGED"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Execute(plan, Options{FailBeforeStep: -1, FailRollbackStep: -1})
	if err == nil {
		t.Fatal("expected verification failure, got nil")
	}
	if res != nil {
		t.Fatalf("expected no result on verification failure, got %+v", res)
	}
	// Nothing must have moved.
	if got := readFile(t, dir, "a.txt"); got != "CHANGED" {
		t.Errorf("a.txt = %q, want CHANGED", got)
	}
	if got := readFile(t, dir, "b.txt"); got != "BBB" {
		t.Errorf("b.txt = %q, want BBB", got)
	}
}

func TestFailureRollback(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB", "x.txt": "XXX"})
	plan, err := BuildPlan(dir, [][2]string{
		{"a.txt", "b.txt"}, {"b.txt", "a.txt"}, {"x.txt", "y.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Fail before the last phase-2 step (step index 5 of 6).
	res, err := Execute(plan, Options{FailBeforeStep: 5, FailRollbackStep: -1})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if !res.RolledBack {
		t.Fatal("expected RolledBack=true")
	}
	if len(res.RollbackErrors) != 0 {
		t.Fatalf("unexpected rollback errors: %v", res.RollbackErrors)
	}
	// Original state restored.
	for name, want := range map[string]string{"a.txt": "AAA", "b.txt": "BBB", "x.txt": "XXX"} {
		if got := readFile(t, dir, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, n := range dirEntries(t, dir) {
		if strings.HasPrefix(n, ".tbr-") || n == "y.txt" {
			t.Errorf("unexpected leftover %q", n)
		}
	}
}

func TestRollbackFailureReportsLocations(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "b.txt": "BBB"})
	plan, err := BuildPlan(dir, [][2]string{{"a.txt", "b.txt"}, {"b.txt", "a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	// Fail at step 3 (first phase-2 move done), then fail one rollback step.
	res, err := Execute(plan, Options{FailBeforeStep: 3, FailRollbackStep: 0})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if len(res.RollbackErrors) == 0 {
		t.Fatal("expected rollback errors to be reported")
	}
	// No content lost: every file reported in Final must exist with content.
	total := 0
	for _, f := range res.Final {
		b, rerr := os.ReadFile(filepath.Join(dir, f.Where))
		if rerr != nil {
			t.Errorf("reported location %q unreadable: %v", f.Where, rerr)
			continue
		}
		total += len(b)
	}
	if total != len("AAA")+len("BBB") {
		t.Errorf("content bytes preserved = %d, want %d", total, 6)
	}
}

func TestValidation(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "AAA", "keep.txt": "K"})
	cases := []struct {
		name  string
		pairs [][2]string
	}{
		{"separator in target", [][2]string{{"a.txt", "sub/b.txt"}}},
		{"parent reference", [][2]string{{"a.txt", ".."}}},
		{"duplicate source case-insensitive", [][2]string{{"a.txt", "x"}, {"A.TXT", "y"}}},
		{"duplicate target case-insensitive", [][2]string{{"a.txt", "n.txt"}, {"keep.txt", "N.TXT"}}},
		{"target exists outside batch", [][2]string{{"a.txt", "keep.txt"}}},
		{"missing source", [][2]string{{"nope.txt", "x.txt"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildPlan(dir, tc.pairs); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
