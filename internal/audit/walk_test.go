package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// tree builds a fixture directory from a map of relative path -> content.
// Paths ending in "/" become directories.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if content == "" && len(rel) > 0 && rel[len(rel)-1] == '/' {
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func keptPaths(rep *Report) map[string]bool {
	m := map[string]bool{}
	for _, e := range rep.Kept {
		m[e.Path] = true
	}
	return m
}

func prunedPaths(rep *Report) map[string]bool {
	m := map[string]bool{}
	for _, e := range rep.Pruned {
		m[e.Path] = true
	}
	return m
}

func TestMultiLevelOverride(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":     "*.log\n",
		"top.log":        "x",
		"keep.txt":       "x",
		"sub/.gitignore": "!keep.log\n",
		"sub/keep.log":   "x",
		"sub/drop.log":   "x",
	})
	rep, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	kept := keptPaths(rep)
	if kept["top.log"] || kept["sub/drop.log"] {
		t.Errorf("log files should be excluded: %v", kept)
	}
	if !kept["keep.txt"] || !kept["sub/keep.log"] {
		t.Errorf("expected keep.txt and sub/keep.log kept: %v", kept)
	}
	for _, e := range rep.Kept {
		if e.Path == "sub/keep.log" {
			if e.File != "sub/.gitignore" || e.Line != 1 || !e.Negate {
				t.Errorf("sub/keep.log trace wrong: %+v", e)
			}
		}
	}
}

func TestPrunedDirCannotBeRevived(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":         "build/\n",
		"build/.gitignore":   "!rescue.txt\n",
		"build/rescue.txt":   "x",
		"build/deep/more.go": "x",
		"src/main.go":        "x",
	})
	rep, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	kept := keptPaths(rep)
	if kept["build/rescue.txt"] || kept["build/deep/more.go"] {
		t.Errorf("pruned subtree must stay excluded: %v", kept)
	}
	if !prunedPaths(rep)["build"] {
		t.Errorf("build should be reported as pruned: %v", rep.Pruned)
	}
	if !kept["src/main.go"] {
		t.Errorf("src/main.go should be kept: %v", kept)
	}
}

func TestDirRestoreAllowsChildren(t *testing.T) {
	// Root ignores build/, then restores it; a nested rule drops only .o files.
	root := tree(t, map[string]string{
		".gitignore":         "build/\n!build/\n",
		"build/.gitignore":   "*.o\n",
		"build/app.o":        "x",
		"build/app.go":       "x",
		"build/out/note.txt": "x",
	})
	rep, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	kept := keptPaths(rep)
	if prunedPaths(rep)["build"] {
		t.Error("restored directory must not be pruned")
	}
	if kept["build/app.o"] {
		t.Error("build/app.o should be excluded by nested rule")
	}
	if !kept["build/app.go"] || !kept["build/out/note.txt"] {
		t.Errorf("restored dir children should be kept: %v", kept)
	}
}

func TestEscapeAndCommentBoundaries(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore": "# comment\n\\#literal\n\\!literal\nspace\\ \n",
		"#literal":   "x",
		"!literal":   "x",
		"space ":     "x",
		"space":      "x",
		"plain.txt":  "x",
	})
	rep, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	kept := keptPaths(rep)
	if kept["#literal"] || kept["!literal"] || kept["space "] {
		t.Errorf("escaped literal names should be excluded: %v", kept)
	}
	if !kept["space"] || !kept["plain.txt"] {
		t.Errorf("space and plain.txt should be kept: %v", kept)
	}
}

func TestSymlinkNotFollowed(t *testing.T) {
	root := tree(t, map[string]string{
		"real/inside.txt": "x",
	})
	outside := tree(t, map[string]string{"secret.txt": "x"})
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	rep, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Path != "link" {
		t.Errorf("symlink should be skipped, got %+v", rep.Skipped)
	}
	for _, e := range rep.Kept {
		if e.Path == "link/secret.txt" {
			t.Error("traversal escaped the root through a symlink")
		}
	}
}
