package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFailedDecodeKeepsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.bin")
	out := filepath.Join(dir, "out.json")
	if err := os.WriteFile(in, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("previous output")
	if err := os.WriteFile(out, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runDecode(in, out); err == nil {
		t.Fatal("expected decode failure")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Fatalf("output overwritten on failure: %q", got)
	}
}

func TestDecodeEncodeCLI(t *testing.T) {
	dir := t.TempDir()
	jsonIn := filepath.Join(dir, "in.json")
	binOut := filepath.Join(dir, "out.bin")
	back := filepath.Join(dir, "back.json")
	doc := `{
	  "header": {"id": 1, "qr": true},
	  "questions": [{"name": ["a", "b"], "qtype": 1, "qclass": 1}],
	  "answers": [{"name": ["a", "b"], "type": 1, "class": 1, "ttl": 60, "data": "192.0.2.1"}]
	}`
	if err := os.WriteFile(jsonIn, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runEncode(jsonIn, binOut); err != nil {
		t.Fatal(err)
	}
	if err := runDecode(binOut, back); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("empty decode output")
	}
}
