package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runCLI(t *testing.T, schema, jsonl string) (string, int) {
	t.Helper()
	sp := writeTemp(t, "schema.json", schema)
	ip := writeTemp(t, "data.jsonl", jsonl)
	var buf bytes.Buffer
	code := run(sp, ip, &buf)
	return buf.String(), code
}

func parseLines(t *testing.T, out string) []lineResult {
	t.Helper()
	var rs []lineResult
	for _, line := range bytes.Split([]byte(out), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r lineResult
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("bad output line %q: %v", line, err)
		}
		rs = append(rs, r)
	}
	return rs
}

func TestCLIBasic(t *testing.T) {
	schema := `{"type":"object","required":["id"],"properties":{"id":{"type":"integer"}},"additionalProperties":false}`
	jsonl := "{\"id\":1}\n{\"id\":\"x\"}\n{not json}\n{}\n"
	out, code := runCLI(t, schema, jsonl)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	rs := parseLines(t, out)
	if len(rs) != 4 {
		t.Fatalf("expected 4 results, got %d: %s", len(rs), out)
	}
	if !rs[0].OK || rs[0].Line != 1 {
		t.Fatalf("line 1 should pass: %+v", rs[0])
	}
	if rs[1].OK || rs[1].Errors[0].Instance != "#/id" {
		t.Fatalf("line 2 should fail at #/id: %+v", rs[1])
	}
	if rs[2].OK || rs[2].Kind != "parse" {
		t.Fatalf("line 3 should be a parse error: %+v", rs[2])
	}
	if rs[3].OK || rs[3].Errors[0].Message == "" {
		t.Fatalf("line 4 should fail required: %+v", rs[3])
	}
	// 顺序稳定：行号递增
	for i := 1; i < len(rs); i++ {
		if rs[i].Line <= rs[i-1].Line {
			t.Fatalf("line order not stable: %+v", rs)
		}
	}
}

func TestCLISchemaErrorPrecedesData(t *testing.T) {
	out, code := runCLI(t, `{"pattern":"x"}`, "{\"a\":1}\n")
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	rs := parseLines(t, out)
	if len(rs) != 1 || rs[0].Kind != "schema" || rs[0].Line != 0 {
		t.Fatalf("expected single schema error, got %s", out)
	}
}

func TestCLIAllValid(t *testing.T) {
	out, code := runCLI(t, `{"type":"integer"}`, "1\n2\n3\n")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, out)
	}
}

func TestCLIErrorLocations(t *testing.T) {
	schema := `{"$defs":{"n":{"type":"integer"}},"properties":{"v":{"$ref":"#/$defs/n"}}}`
	out, _ := runCLI(t, schema, "{\"v\":\"s\"}\n")
	rs := parseLines(t, out)
	if rs[0].Errors[0].Instance != "#/v" {
		t.Fatalf("bad instance location: %+v", rs[0].Errors[0])
	}
	if rs[0].Errors[0].Schema != "#/$defs/n/type" {
		t.Fatalf("bad schema location: %+v", rs[0].Errors[0])
	}
}
