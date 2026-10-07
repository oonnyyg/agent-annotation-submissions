package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestNestedOverrideAndNegation(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "*.log\nkeep.log\n")
	write(t, root, "a.log", "")
	write(t, root, "keep.log", "")
	write(t, root, "sub/.gitignore", "!keep.log\n")
	write(t, root, "sub/keep.log", "")
	write(t, root, "sub/other.log", "")

	rep, _, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	// Last same-level match wins: keep.log re-ignored by later rule.
	if !contains(rep.Ignored, "keep.log") {
		t.Error("keep.log should be ignored by later same-level rule")
	}
	// Deeper .gitignore negation overrides ancestor rule.
	if !contains(rep.Kept, "sub/keep.log") {
		t.Error("sub/keep.log should be restored by deeper negation")
	}
	if !contains(rep.Ignored, "sub/other.log") {
		t.Error("sub/other.log should be ignored by ancestor rule")
	}
	if !contains(rep.Kept, ".gitignore") || !contains(rep.Kept, "sub/.gitignore") {
		t.Error(".gitignore files themselves are regular files and kept")
	}
}

func TestPrunedDirBlocksDescendants(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "build/\n")
	write(t, root, "build/.gitignore", "!rescue.txt\n")
	write(t, root, "build/rescue.txt", "")
	write(t, root, "src/ok.txt", "")

	rep, entries, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(rep.Pruned, "build") {
		t.Fatal("build should be pruned")
	}
	if contains(rep.Kept, "build/rescue.txt") {
		t.Error("negation inside a pruned dir must not re-include descendants")
	}
	ex, err := Explain(root, "build/rescue.txt", entries)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Decision != "excluded" || ex.Reason != "ancestor" || ex.BlockedAt != "build" {
		t.Errorf("expected ancestor-blocked explanation, got %+v", ex)
	}
	if ex.BlockedBy == nil || ex.BlockedBy.File != ".gitignore" || ex.BlockedBy.Line != 1 {
		t.Errorf("expected blocking rule .gitignore:1, got %+v", ex.BlockedBy)
	}
}

func TestDirRestoreAllowsChildren(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "out/\n!out/\nout/*\n!out/keep.txt\n")
	write(t, root, "out/keep.txt", "")
	write(t, root, "out/drop.txt", "")

	rep, entries, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if contains(rep.Pruned, "out") {
		t.Fatal("restored dir must not be pruned")
	}
	if !contains(rep.Kept, "out/keep.txt") || !contains(rep.Ignored, "out/drop.txt") {
		t.Errorf("kept=%v ignored=%v", rep.Kept, rep.Ignored)
	}
	ex, err := Explain(root, "out/keep.txt", entries)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Decision != "kept" || ex.MatchedBy == nil || ex.MatchedBy.Line != 4 || !ex.MatchedBy.Negate {
		t.Errorf("expected kept via negation at line 4, got %+v", ex)
	}
}

func TestSymlinkNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "secret.txt", "")
	write(t, root, "real.txt", "")
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	rep, _, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if contains(rep.Kept, "link/secret.txt") {
		t.Fatal("symlink must never be followed outside the root")
	}
	found := false
	for _, s := range rep.Skipped {
		if s.Path == "link" && s.Kind == "symlink" {
			found = true
		}
	}
	if !found {
		t.Errorf("symlink should be recorded as skipped, got %+v", rep.Skipped)
	}
}

func TestExplainMissingAndKept(t *testing.T) {
	root := t.TempDir()
	write(t, root, "plain.txt", "")
	_, entries, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := Explain(root, "plain.txt", entries)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Decision != "kept" || ex.MatchedBy != nil {
		t.Errorf("plain file should be kept with no rule, got %+v", ex)
	}
	ex, err = Explain(root, "nope.txt", entries)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Decision != "missing" {
		t.Errorf("missing path should report missing, got %+v", ex)
	}
	if _, err := Explain(root, "../escape", entries); err == nil {
		t.Error("paths escaping the root must be rejected")
	}
}
