package delta

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupFiles(t *testing.T, oldData, newData []byte, blockSize int) (sig *Signature, d *Delta, oldPath, outPath string) {
	t.Helper()
	dir := t.TempDir()
	oldPath = filepath.Join(dir, "old.bin")
	outPath = filepath.Join(dir, "new.bin")
	if err := os.WriteFile(oldPath, oldData, 0o644); err != nil {
		t.Fatal(err)
	}
	sig = makeSig(t, oldData, blockSize)
	var err error
	d, _, _, err = Generate(sig, newData)
	if err != nil {
		t.Fatal(err)
	}
	return sig, d, oldPath, outPath
}

func TestApplyToFile(t *testing.T) {
	oldData := bytes.Repeat([]byte("hello world, hello world, "), 50)
	newData := append([]byte("v2: "), oldData...)
	sig, d, oldPath, outPath := setupFiles(t, oldData, newData, 64)

	if err := ApplyToFile(d, sig, oldPath, outPath); err != nil {
		t.Fatalf("ApplyToFile: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatal("target content mismatch")
	}
	// Old file untouched.
	oldNow, _ := os.ReadFile(oldPath)
	if !bytes.Equal(oldNow, oldData) {
		t.Fatal("old file was modified")
	}
}

func TestApplyRejectsSamePath(t *testing.T) {
	oldData := []byte("some old content here")
	sig, d, oldPath, _ := setupFiles(t, oldData, oldData, 8)
	if err := ApplyToFile(d, sig, oldPath, oldPath); err == nil {
		t.Fatal("expected error when target equals old path")
	}
}

func TestApplyRejectsWrongBasis(t *testing.T) {
	oldData := []byte("the real basis file contents")
	sig, d, oldPath, outPath := setupFiles(t, oldData, []byte("new"), 8)
	// Corrupt the old file after the signature was made.
	if err := os.WriteFile(oldPath, []byte("tampered basis file contents!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ApplyToFile(d, sig, oldPath, outPath); err == nil {
		t.Fatal("expected basis identity check to fail")
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatal("target must not be created on failure")
	}
}

func TestCorruptDelta(t *testing.T) {
	oldData := bytes.Repeat([]byte("0123456789abcdef"), 20)
	newData := append([]byte("hdr"), oldData...)
	sig, d, oldPath, outPath := setupFiles(t, oldData, newData, 16)

	// Pre-existing target must survive every failure.
	prev := []byte("previous target content")
	if err := os.WriteFile(outPath, prev, 0o644); err != nil {
		t.Fatal(err)
	}
	checkPreserved := func(name string) {
		t.Helper()
		got, err := os.ReadFile(outPath)
		if err != nil || !bytes.Equal(got, prev) {
			t.Fatalf("%s: target not preserved", name)
		}
	}

	// 1. Out-of-bounds copy.
	bad := *d
	bad.Ops = append([]Op{}, d.Ops...)
	bad.Ops = append(bad.Ops, Op{Op: "copy", Offset: sig.FileSize - 1, Length: 100})
	bad.TargetSize += 100
	if err := ApplyToFile(&bad, sig, oldPath, outPath); err == nil {
		t.Fatal("out-of-bounds copy accepted")
	}
	checkPreserved("out-of-bounds copy")

	// 2. Unknown op.
	bad2 := *d
	bad2.Ops = append([]Op{}, d.Ops...)
	bad2.Ops = append(bad2.Ops, Op{Op: "explode", Length: 1})
	if err := ApplyToFile(&bad2, sig, oldPath, outPath); err == nil {
		t.Fatal("unknown op accepted")
	}
	checkPreserved("unknown op")

	// 3. Corrupted literal data (digest mismatch at rebuild).
	bad3 := *d
	bad3.Ops = append([]Op{}, d.Ops...)
	for i := range bad3.Ops {
		if bad3.Ops[i].Op == "literal" && len(bad3.Ops[i].Data) > 0 {
			data := append([]byte{}, bad3.Ops[i].Data...)
			data[0] ^= 0xff
			bad3.Ops[i].Data = data
			break
		}
	}
	if err := ApplyToFile(&bad3, sig, oldPath, outPath); err == nil {
		t.Fatal("corrupted literal accepted")
	}
	checkPreserved("corrupted literal")

	// 4. Delta bound to a different basis.
	other := makeSig(t, []byte("completely different basis"), 16)
	if err := ApplyToFile(d, other, oldPath, outPath); err == nil ||
		!strings.Contains(err.Error(), "different basis") {
		t.Fatalf("expected different-basis error, got %v", err)
	}
	checkPreserved("different basis")
}
