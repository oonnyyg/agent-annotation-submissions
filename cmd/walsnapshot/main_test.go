package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRefuseOverwriteSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.db")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(src, src, "other", []byte("y")); err == nil {
		t.Fatal("expected refusal to overwrite source")
	}
	// Existing non-source file must also be refused.
	existing := filepath.Join(dir, "exists.db")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(existing, src, "other", []byte("y")); err == nil {
		t.Fatal("expected refusal to overwrite existing file")
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "old" {
		t.Fatal("existing file was modified")
	}
}

func TestReportShape(t *testing.T) {
	rep := &report{LastCommit: -1}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"dbPath", "walPath", "frames", "validFrames", "lastCommit"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("report missing key %q", k)
		}
	}
}
