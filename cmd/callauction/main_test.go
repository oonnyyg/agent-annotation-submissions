package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFlow(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "callauction.exe")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}

	input := strings.Join([]string{
		`{"type":"place","id":"b1","side":"buy","price":100,"qty":10}`,
		`{"type":"place","id":"s1","side":"sell","price":100,"qty":4}`,
		`{"type":"place","id":"dup","side":"buy","price":100}`,
		`{"type":"preview","ref":100}`,
		`{"type":"execute","batch":"B1","ref":100}`,
		`{"type":"execute","batch":"B1","ref":100}`,
		`{"type":"execute","batch":"B1","ref":99}`,
		``,
	}, "\n")

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 7 {
		t.Fatalf("want 7 output lines incl. errors/preview/3 executes, got %d: %s", len(lines), out)
	}
	wantFields := [][]string{
		{`"type":"placed"`, `"seq":1`},
		{`"type":"placed"`, `"seq":2`},
		{`"type":"error"`},
		{`"type":"preview"`, `"batch":""`, `"matched":true`, `"price":100`, `"total_qty":4`, `"filled":0`},
		{`"type":"executed"`, `"batch":"B1"`, `"price":100`, `"total_qty":4`, `"id":"b1"`, `"remaining":6`},
		{`"type":"executed"`, `"batch":"B1"`, `"total_qty":4`, `"id":"b1"`, `"remaining":6`},
		{`"type":"error"`},
	}
	for i, fields := range wantFields {
		for _, want := range fields {
			if !strings.Contains(lines[i], want) {
				t.Fatalf("line %d = %s, want contains %s", i, lines[i], want)
			}
		}
	}
}
