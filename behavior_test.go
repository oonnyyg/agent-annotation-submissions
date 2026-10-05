package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const simpleManifest = `{
  "targets": {
    "gen": {
      "command": [],
      "inputs": ["src/in.txt"],
      "outputs": ["out/mid.txt"]
    },
    "pack": {
      "deps": ["gen"],
      "command": [],
      "inputs": ["out/mid.txt"],
      "outputs": ["out/final.txt"]
    }
  }
}`

func manifestWith(cmdGen, cmdPack []string) string {
	q := func(v []string) string {
		s := `"`
		for i, a := range v {
			if i > 0 {
				s += `","`
			}
			s += a
		}
		return s + `"`
	}
	m := simpleManifest
	m = strings.Replace(m, `"command": []`, `"command": [`+q(cmdGen)+`]`, 1)
	m = strings.Replace(m, `"command": []`, `"command": [`+q(cmdPack)+`]`, 1)
	return m
}

func TestContentInvalidationAndRestore(t *testing.T) {
	m := manifestWith(
		helperCmd("copy", "src/in.txt", "out/mid.txt"),
		helperCmd("copy", "out/mid.txt", "out/final.txt"),
	)
	root, cacheDir, logDir := setup(t, m)
	writeFile(t, root, "src/in.txt", "v1")

	man, cache, order := loadAll(t, root, cacheDir, nil)
	res := Run(man, cache, root, logDir, order, false)
	if statusOf(res, "gen").Status != "rebuilt" || statusOf(res, "pack").Status != "rebuilt" {
		t.Fatalf("first run should rebuild all: %+v", res)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	// Second run: everything cached.
	man, cache, order = loadAll(t, root, cacheDir, nil)
	res = Run(man, cache, root, logDir, order, false)
	if statusOf(res, "gen").Status != "cached" || statusOf(res, "pack").Status != "cached" {
		t.Fatalf("second run should be fully cached: %+v", res)
	}

	// Change input content: gen rebuilds, pack follows (dep output changed).
	writeFile(t, root, "src/in.txt", "v2")
	man, cache, order = loadAll(t, root, cacheDir, nil)
	res = Run(man, cache, root, logDir, order, false)
	if statusOf(res, "gen").Status != "rebuilt" {
		t.Fatalf("gen should rebuild after input change: %+v", res)
	}
	if got := statusOf(res, "pack"); got.Status != "rebuilt" {
		t.Fatalf("pack should rebuild after dep output change: %+v", got)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	// Restore original content: only mtime differs, content matches again.
	writeFile(t, root, "src/in.txt", "v1")
	man, cache, order = loadAll(t, root, cacheDir, nil)
	// Note: cache now holds v2 fingerprints, so gen rebuilds once...
	res = Run(man, cache, root, logDir, order, false)
	if statusOf(res, "gen").Status != "rebuilt" {
		t.Fatalf("gen should rebuild back to v1: %+v", res)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	// ...and touching the file again (same content, new mtime) must NOT rebuild.
	writeFile(t, root, "src/in.txt", "v1")
	man, cache, order = loadAll(t, root, cacheDir, nil)
	res = Run(man, cache, root, logDir, order, false)
	if statusOf(res, "gen").Status != "cached" {
		t.Fatalf("content-only match must be cached despite mtime change: %+v", res)
	}
}

func TestOutputTampering(t *testing.T) {
	m := manifestWith(helperCmd("copy", "src/in.txt", "out/mid.txt"), helperCmd("copy", "out/mid.txt", "out/final.txt"))
	root, cacheDir, logDir := setup(t, m)
	writeFile(t, root, "src/in.txt", "v1")

	man, cache, order := loadAll(t, root, cacheDir, nil)
	Run(man, cache, root, logDir, order, false)
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	// Externally modify an output without changing inputs.
	writeFile(t, root, "out/mid.txt", "HACKED")
	man, cache, order = loadAll(t, root, cacheDir, nil)
	res := Run(man, cache, root, logDir, order, false)
	got := statusOf(res, "gen")
	if got.Status != "rebuilt" || !strings.Contains(got.Reason, "modified externally") {
		t.Fatalf("tampered output must rebuild with clear reason: %+v", got)
	}
}

func TestBranchFailure(t *testing.T) {
	manifest := `{
	  "targets": {
	    "bad":  {"command": ["CMD_FAIL"], "inputs": [], "outputs": ["out/bad.txt"]},
	    "down": {"deps": ["bad"], "command": ["CMD_OK"], "inputs": ["out/bad.txt"], "outputs": ["out/down.txt"]},
	    "free": {"command": ["CMD_OK"], "inputs": [], "outputs": ["out/free.txt"]}
	  }
	}`
	ok := strings.Join(helperCmd("write", "out/free.txt", "F"), `","`)
	fail := strings.Join(helperCmd("fail"), `","`)
	manifest = strings.ReplaceAll(manifest, "CMD_OK", ok)
	manifest = strings.ReplaceAll(manifest, "CMD_FAIL", fail)
	root, cacheDir, logDir := setup(t, manifest)

	man, cache, order := loadAll(t, root, cacheDir, nil)
	res := Run(man, cache, root, logDir, order, false)
	if got := statusOf(res, "bad"); got.Status != "failed" {
		t.Fatalf("bad should fail: %+v", got)
	}
	if got := statusOf(res, "down"); got.Status != "skipped" || !strings.Contains(got.Reason, "bad") {
		t.Fatalf("down should be skipped naming the failed dep: %+v", got)
	}
	if got := statusOf(res, "free"); got.Status != "rebuilt" {
		t.Fatalf("unrelated branch must still run: %+v", got)
	}
	if cache.Entries["bad"] != nil {
		t.Fatal("failed target must not record a success cache entry")
	}
	if cache.Entries["free"] == nil {
		t.Fatal("successful unrelated target must be cached")
	}
}

func TestPlanMode(t *testing.T) {
	m := manifestWith(helperCmd("copy", "src/in.txt", "out/mid.txt"), helperCmd("copy", "out/mid.txt", "out/final.txt"))
	root, cacheDir, logDir := setup(t, m)
	writeFile(t, root, "src/in.txt", "v1")

	man, cache, order := loadAll(t, root, cacheDir, nil)
	Run(man, cache, root, logDir, order, false)
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	// Change input, then plan: gen rebuilds, pack is pending-upstream.
	writeFile(t, root, "src/in.txt", "v2")
	man, cache, order = loadAll(t, root, cacheDir, nil)
	res := Run(man, cache, root, logDir, order, true)
	if got := statusOf(res, "gen"); got.Status != "rebuild" {
		t.Fatalf("plan: gen should be rebuild: %+v", got)
	}
	if got := statusOf(res, "pack"); got.Status != "pending-upstream" {
		t.Fatalf("plan: pack should be pending-upstream: %+v", got)
	}
	// Plan mode must not run commands or touch the cache.
	if cache.Entries["gen"].Fingerprint == "" {
		t.Fatal("cache entry should still exist")
	}
	man2, cache2, _ := loadAll(t, root, cacheDir, nil)
	_ = man2
	if cache2.Entries["gen"].Fingerprint != cache.Entries["gen"].Fingerprint {
		t.Fatal("plan mode must not modify the cache")
	}
	// Outputs unchanged by plan mode.
	if readFile(root, "out/mid.txt") != "v1" {
		t.Fatal("plan mode must not run commands")
	}
}

func readFile(root, rel string) string {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return ""
	}
	return string(b)
}
