package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cpy = `
01 REC.
   05 NAME       PIC X(5).
   05 BALANCE    PIC S9(5)V99 COMP-3.
   05 TAGS       OCCURS 2 TIMES.
      10 TAG-ID  PIC 9(3).
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLIRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cpyPath := writeFile(t, dir, "rec.cpy", cpy)
	jsonl := "{\"BALANCE\":\"-12.34\",\"NAME\":\"AL   \",\"TAGS\":[{\"TAG-ID\":\"7\"},{\"TAG-ID\":\"42\"}]}\n" +
		"{\"BALANCE\":\"0.01\",\"NAME\":\"BOB\",\"TAGS\":[{\"TAG-ID\":\"0\"},{\"TAG-ID\":\"999\"}]}\n"
	inJSONL := writeFile(t, dir, "in.jsonl", jsonl)
	binPath := filepath.Join(dir, "out.bin")
	if err := run("to-bin", cpyPath, inJSONL, binPath); err != nil {
		t.Fatalf("to-bin: %v", err)
	}
	bin, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	// Record length: 5 + 4 + 2*3 = 15.
	if len(bin) != 30 {
		t.Fatalf("bin length = %d, want 30", len(bin))
	}
	outJSONL := filepath.Join(dir, "out.jsonl")
	if err := run("to-json", cpyPath, binPath, outJSONL); err != nil {
		t.Fatalf("to-json: %v", err)
	}
	got, err := os.ReadFile(outJSONL)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"BALANCE\":\"-12.34\",\"NAME\":\"AL   \",\"TAGS\":[{\"TAG-ID\":\"7\"},{\"TAG-ID\":\"42\"}]}\n" +
		"{\"BALANCE\":\"0.01\",\"NAME\":\"BOB  \",\"TAGS\":[{\"TAG-ID\":\"0\"},{\"TAG-ID\":\"999\"}]}\n"
	if string(got) != want {
		t.Fatalf("round trip mismatch:\ngot  %q\nwant %q", got, want)
	}
}

func TestCLITruncatedFile(t *testing.T) {
	dir := t.TempDir()
	cpyPath := writeFile(t, dir, "rec.cpy", cpy)
	binPath := writeFile(t, dir, "bad.bin", strings.Repeat("A", 16)) // 15+1
	outPath := filepath.Join(dir, "out.jsonl")
	err := run("to-json", cpyPath, binPath, outPath)
	if err == nil || !strings.Contains(err.Error(), "trailing bytes") {
		t.Fatalf("want trailing-bytes error, got %v", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatal("partial output must not be published")
	}
}

func TestCLIBadRecordReportsNumber(t *testing.T) {
	dir := t.TempDir()
	cpyPath := writeFile(t, dir, "rec.cpy", cpy)
	good := []byte("AL   \x00\x00\x12\x3C" + "001002")
	bad := []byte("BOB  \x00\x00\x12\x3F" + "001002") // sign nibble F
	binPath := writeFile(t, dir, "in.bin", string(append(good, bad...)))
	outPath := filepath.Join(dir, "out.jsonl")
	err := run("to-json", cpyPath, binPath, outPath)
	if err == nil || !strings.Contains(err.Error(), "record 2") || !strings.Contains(err.Error(), "BALANCE") {
		t.Fatalf("want record-2 BALANCE error, got %v", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatal("partial output must not be published")
	}
}

func TestCLILayout(t *testing.T) {
	dir := t.TempDir()
	cpyPath := writeFile(t, dir, "rec.cpy", cpy)
	layout, err := loadLayout(cpyPath)
	if err != nil {
		t.Fatal(err)
	}
	if layout.RecordLength != 15 {
		t.Fatalf("record length = %d, want 15", layout.RecordLength)
	}
	b, err := layout.MarshalLayout()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"recordLength": 15`, `"kind": "comp-3"`, `"scale": 2`, `"occurs": 2`} {
		if !strings.Contains(s, want) {
			t.Errorf("layout JSON missing %s", want)
		}
	}
}
