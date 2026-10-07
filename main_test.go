package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLI(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	instPath := filepath.Join(dir, "data.jsonl")
	schema := `{"type":"object","required":["id"],"properties":{"id":{"type":"integer"}},"additionalProperties":false}`
	if err := os.WriteFile(schemaPath, []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	lines := strings.Join([]string{
		`{"id":1}`,
		`{"id":"x","extra":true}`,
		`not json`,
		`{"id":2}`,
	}, "\n")
	if err := os.WriteFile(instPath, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run(schemaPath, instPath, &out)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	got := out.String()
	for _, want := range []string{
		"line 1: OK",
		"line 2: REJECT",
		"line 3: INVALID JSON",
		"line 4: OK",
		"expected type integer",
		"additional property",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunBadSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	instPath := filepath.Join(dir, "data.jsonl")
	if err := os.WriteFile(schemaPath, []byte(`{"pattern":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run(schemaPath, instPath, &out)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(out.String(), "unsupported keyword") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
