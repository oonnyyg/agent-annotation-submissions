package split

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPublishesBatch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "batch-001")
	input := `<r><item>1</item><item>2</item><item>3</item></r>`
	p, _ := ParsePath("r/item")
	n, err := Run(strings.NewReader(input), p, out, 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
	for _, name := range []string{"record-000001.xml", "record-000002.xml", "record-000003.xml", "index.txt"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	idx, _ := os.ReadFile(filepath.Join(out, "index.txt"))
	s := string(idx)
	if !strings.Contains(s, "count: 3") || !strings.Contains(s, "record-000003.xml") || !strings.Contains(s, "sha256=") {
		t.Fatalf("bad index:\n%s", s)
	}
	// staging directory must be gone
	matches, _ := filepath.Glob(filepath.Join(dir, ".staging-*"))
	if len(matches) != 0 {
		t.Fatalf("staging dirs left behind: %v", matches)
	}
}

func TestRunRefusesExistingDir(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "batch-001")
	p, _ := ParsePath("r/item")
	if _, err := Run(strings.NewReader(`<r><item/></r>`), p, out, 0); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if _, err := Run(strings.NewReader(`<r><item/></r>`), p, out, 0); err == nil {
		t.Fatal("second Run into existing dir must fail")
	}
}

func TestRunTailErrorLeavesNoBatch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "batch-001")
	// Two valid records, then a broken tail.
	input := `<r><item>1</item><item>2</item><broken>`
	p, _ := ParsePath("r/item")
	n, err := Run(strings.NewReader(input), p, out, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if n != 0 {
		t.Fatalf("n = %d, want 0 published records", n)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("batch dir must not exist after failure")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".staging-*"))
	if len(matches) != 0 {
		t.Fatalf("staging dirs left behind: %v", matches)
	}
}
