package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestHelperProcess acts as the build command inside tests:
//
//	helper write <file> <content>   -- write a file (creating parent dirs)
//	helper fail                     -- exit non-zero
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	switch args[0] {
	case "write":
		if err := os.MkdirAll(filepath.Dir(args[1]), 0o755); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(args[1], []byte(args[2]), 0o644); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "copy":
		data, err := os.ReadFile(args[1])
		if err != nil {
			os.Exit(2)
		}
		if err := os.MkdirAll(filepath.Dir(args[2]), 0o755); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(args[2], data, 0o644); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "intentional failure")
		os.Exit(3)
	}
	os.Exit(2)
}

func helperCmd(action string, rest ...string) []string {
	exe, _ := os.Executable()
	// Forward slashes keep the path JSON-safe and still work on Windows.
	exe = filepath.ToSlash(exe)
	return append([]string{exe, "-test.run=TestHelperProcess", "--", action}, rest...)
}

// setup creates a temp work root with the given manifest and returns paths.
func setup(t *testing.T, manifest string) (root, cacheDir, logDir string) {
	t.Helper()
	t.Setenv("GO_HELPER", "1")
	root = t.TempDir()
	cacheDir = filepath.Join(root, ".cache")
	logDir = filepath.Join(root, "logs")
	if err := os.WriteFile(filepath.Join(root, "build.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, cacheDir, logDir
}

func loadAll(t *testing.T, root, cacheDir string, goals []string) (*Manifest, *Cache, []string) {
	t.Helper()
	m, err := LoadManifest(filepath.Join(root, "build.json"), root)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if err := ValidateGraph(m); err != nil {
		t.Fatalf("ValidateGraph: %v", err)
	}
	if len(goals) == 0 {
		goals = m.Order
	}
	set, err := Closure(m, goals)
	if err != nil {
		t.Fatalf("Closure: %v", err)
	}
	cache, err := LoadCache(cacheDir)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	return m, cache, TopoOrder(m, set)
}

func statusOf(results []Result, name string) Result {
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	return Result{Name: name, Status: "<absent>"}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readReport(t *testing.T, path string) Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
