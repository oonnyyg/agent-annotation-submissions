package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadErrIn(t *testing.T, root, manifest string) error {
	t.Helper()
	p := filepath.Join(root, "build.json")
	if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(p, root)
	if err != nil {
		return err
	}
	return ValidateGraph(m)
}

func loadErr(t *testing.T, manifest string) error {
	t.Helper()
	return loadErrIn(t, t.TempDir(), manifest)
}

func TestValidationRejections(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "evil.txt")
	absJSON, _ := json.Marshal(absPath)
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"unknown dep", `{"targets":{"a":{"deps":["nope"],"command":["x"],"outputs":["o/a"]}}}`, "unknown target"},
		{"cycle", `{"targets":{
			"a":{"deps":["b"],"command":["x"],"outputs":["o/a"]},
			"b":{"deps":["a"],"command":["x"],"outputs":["o/b"]}}}`, "cycle"},
		{"path escape", `{"targets":{"a":{"command":["x"],"inputs":["../evil"],"outputs":["o/a"]}}}`, "escapes"},
		{"absolute path", `{"targets":{"a":{"command":["x"],"inputs":[` + string(absJSON) + `],"outputs":["o/a"]}}}`, "relative"},
		{"shared output", `{"targets":{
			"a":{"command":["x"],"outputs":["o/same"]},
			"b":{"command":["x"],"outputs":["o/same"]}}}`, "both"},
		{"undeclared dep input", `{"targets":{
			"a":{"command":["x"],"outputs":["o/a"]},
			"b":{"command":["x"],"inputs":["o/a"],"outputs":["o/b"]}}}`, "without declaring"},
		{"empty command", `{"targets":{"a":{"command":[],"outputs":["o/a"]}}}`, "non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := loadErr(t, tc.manifest)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not permitted here: %v", err)
	}
	manifest := `{"targets":{"a":{"command":["x"],"inputs":["link.txt"],"outputs":["o/a"]}}}`
	err := loadErrIn(t, root, manifest)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink rejection, got %v", err)
	}
}

func TestTopoOrderLexicographic(t *testing.T) {
	manifest := `{"targets":{
		"z":{"command":["x"],"outputs":["o/z"]},
		"a":{"command":["x"],"outputs":["o/a"]},
		"m":{"deps":["z"],"command":["x"],"inputs":["o/z"],"outputs":["o/m"]}}}`
	root := t.TempDir()
	p := filepath.Join(root, "build.json")
	if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(p, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGraph(m); err != nil {
		t.Fatal(err)
	}
	set, _ := Closure(m, []string{"m", "a"})
	order := TopoOrder(m, set)
	got := strings.Join(order, ",")
	if got != "a,z,m" {
		t.Fatalf("want a,z,m got %s", got)
	}
	// Closure of a single goal excludes unrelated targets.
	set, _ = Closure(m, []string{"m"})
	if set["a"] {
		t.Fatal("closure of m must not include a")
	}
}
