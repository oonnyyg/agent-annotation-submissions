package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDeltaCopyAndInsert(t *testing.T) {
	base := []byte("the quick brown fox")
	// copy "quick " (offset 4, len 6), insert "red", copy " fox".
	var d bytes.Buffer
	writeDeltaVarint(&d, int64(len(base)))
	writeDeltaVarint(&d, 6+3+4)
	d.WriteByte(0x80 | 0x01 | 0x10) // offset byte0, size byte0
	d.WriteByte(4)
	d.WriteByte(6)
	d.WriteByte(3)
	d.WriteString("red")
	d.WriteByte(0x80 | 0x01 | 0x10)
	d.WriteByte(15)
	d.WriteByte(4)
	got, err := ApplyDelta(base, d.Bytes())
	if err != nil {
		t.Fatalf("ApplyDelta: %v", err)
	}
	if string(got) != "quick red fox" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDeltaRejects(t *testing.T) {
	base := []byte("abc")
	cases := map[string][]byte{
		"base size mismatch": {3 + 1, 0, 0}, // declares base size 4
		"copy out of range":  nil,           // built below
		"opcode zero":        nil,
		"truncated insert":   nil,
		"result size short":  nil,
	}
	// copy out of range: copy 10 bytes from offset 0 of a 3-byte base.
	var d1 bytes.Buffer
	writeDeltaVarint(&d1, 3)
	writeDeltaVarint(&d1, 10)
	d1.WriteByte(0x80 | 0x10)
	d1.WriteByte(10)
	cases["copy out of range"] = d1.Bytes()
	// opcode zero.
	var d2 bytes.Buffer
	writeDeltaVarint(&d2, 3)
	writeDeltaVarint(&d2, 1)
	d2.WriteByte(0x00)
	cases["opcode zero"] = d2.Bytes()
	// truncated insert: claims 5 literal bytes, provides 2.
	var d3 bytes.Buffer
	writeDeltaVarint(&d3, 3)
	writeDeltaVarint(&d3, 5)
	d3.WriteByte(5)
	d3.WriteString("ab")
	cases["truncated insert"] = d3.Bytes()
	// result smaller than declared.
	var d4 bytes.Buffer
	writeDeltaVarint(&d4, 3)
	writeDeltaVarint(&d4, 9)
	d4.WriteByte(1)
	d4.WriteString("x")
	cases["result size short"] = d4.Bytes()

	for name, delta := range cases {
		if _, err := ApplyDelta(base, delta); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}

func TestLimitsEnforced(t *testing.T) {
	base := []byte("0123456789")
	big := bytes.Repeat([]byte("a"), 100)
	entries := []testEntry{
		{typ: TypeBlob, data: base},
		{typ: TypeOfsDelta, data: makeDelta(len(base), big, -1, 0), baseRel: 1},
		{typ: TypeOfsDelta, data: makeDelta(len(big), append(big, '!'), -1, 0), baseRel: 1},
	}
	probe, err := Parse(buildPack(t, entries))
	if err != nil {
		t.Fatalf("Parse probe: %v", err)
	}
	entries[1].baseRel = probe.Entries[1].Offset - probe.Entries[0].Offset
	entries[2].baseRel = probe.Entries[2].Offset - probe.Entries[1].Offset
	data := buildPack(t, entries)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Object size limit.
	if _, err := f.Resolve(Limits{MaxObjectSize: 50, MaxDeltaDepth: 10}); err == nil ||
		!strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("want size limit error, got %v", err)
	}
	// Delta depth limit: chain is 2 deep.
	if _, err := f.Resolve(Limits{MaxObjectSize: 1 << 20, MaxDeltaDepth: 1}); err == nil ||
		!strings.Contains(err.Error(), "delta depth") {
		t.Fatalf("want depth limit error, got %v", err)
	}
	// Sane limits pass.
	if _, err := f.Resolve(Limits{MaxObjectSize: 1 << 20, MaxDeltaDepth: 4}); err != nil {
		t.Fatalf("Resolve with sane limits: %v", err)
	}
}

func TestExportAtomicAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "export")

	objs := []*Object{
		{Entry: &Entry{Offset: 12}, Type: TypeBlob, Data: []byte("hello")},
	}
	objs[0].ID = ObjectID(TypeBlob, objs[0].Data)

	if err := Export(out, objs); err != nil {
		t.Fatalf("Export: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(out, objs[0].ID))
	if err != nil || string(got) != "hello" {
		t.Fatalf("object file: %v %q", err, got)
	}
	man, err := os.ReadFile(filepath.Join(out, "MANIFEST.txt"))
	if err != nil || !strings.Contains(string(man), objs[0].ID) {
		t.Fatalf("manifest: %v", err)
	}
	// No staging dirs left behind.
	matches, _ := filepath.Glob(filepath.Join(dir, ".pack-export-*"))
	if len(matches) != 0 {
		t.Fatalf("staging dirs left: %v", matches)
	}
	// Existing directory is never overwritten.
	if err := Export(out, objs); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want exists error, got %v", err)
	}
}
