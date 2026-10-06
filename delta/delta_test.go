package delta

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func roundTrip(t *testing.T, oldData, newData []byte, blockSize int) (Stats, []byte) {
	t.Helper()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.bin")
	sigPath := filepath.Join(dir, "sig.json")
	deltaPath := filepath.Join(dir, "delta.json")
	targetPath := filepath.Join(dir, "new.bin")
	if err := os.WriteFile(oldPath, oldData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSignature(oldPath, blockSize, sigPath); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDelta(sigPath, writeTemp(t, dir, "new-input.bin", newData), deltaPath); err != nil {
		t.Fatal(err)
	}
	st, err := Apply(oldPath, deltaPath, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	return st, got
}

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHeaderInsert(t *testing.T) {
	body := make([]byte, 10000)
	rand.Read(body)
	newData := append([]byte("INSERTED-HEADER"), body...)
	st, got := roundTrip(t, body, newData, 1024)
	if !bytes.Equal(got, newData) {
		t.Fatal("reconstructed file mismatch")
	}
	if st.Reused != int64(len(body)) || st.Literal != int64(len("INSERTED-HEADER")) {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

func TestRepeatedBlocks(t *testing.T) {
	blk := bytes.Repeat([]byte("ABCD"), 256) // 1024 bytes
	oldData := append(append([]byte{}, blk...), blk...)
	newData := blk
	st, got := roundTrip(t, oldData, newData, 1024)
	if !bytes.Equal(got, newData) {
		t.Fatal("reconstructed file mismatch")
	}
	// Identical content occurs at old offsets 0 and 1024; the matcher must
	// pick the smallest offset.
	sig := BuildSignature(oldData, 1024)
	d := BuildDelta(sig, newData)
	var copies []Op
	for _, op := range d.Ops {
		if op.Op == "copy" {
			copies = append(copies, op)
		}
	}
	if len(copies) != 1 || copies[0].Offset != 0 || copies[0].Length != 1024 {
		t.Fatalf("expected one copy at offset 0, got %+v", copies)
	}
	if st.Reused != 1024 {
		t.Fatalf("expected 1024 reused bytes, got %+v", st)
	}
}

func TestCorruptedDeltaRejected(t *testing.T) {
	dir := t.TempDir()
	oldData := make([]byte, 4096)
	rand.Read(oldData)
	oldPath := writeTemp(t, dir, "old.bin", oldData)
	sigPath := filepath.Join(dir, "sig.json")
	if err := WriteSignature(oldPath, 1024, sigPath); err != nil {
		t.Fatal(err)
	}
	newPath := writeTemp(t, dir, "new.bin", oldData)
	deltaPath := filepath.Join(dir, "delta.json")
	if _, err := WriteDelta(sigPath, newPath, deltaPath); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(dir, "target.bin")
	original := []byte("pre-existing target")
	if err := os.WriteFile(targetPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	corrupt := func(mutate func(d *Delta)) {
		t.Helper()
		buf, err := os.ReadFile(deltaPath)
		if err != nil {
			t.Fatal(err)
		}
		var d Delta
		if err := json.Unmarshal(buf, &d); err != nil {
			t.Fatal(err)
		}
		mutate(&d)
		bad, _ := json.Marshal(&d)
		badPath := writeTemp(t, dir, "bad-delta.json", bad)
		if _, err := Apply(oldPath, badPath, targetPath); err == nil {
			t.Fatal("corrupted delta was accepted")
		}
		got, _ := os.ReadFile(targetPath)
		if !bytes.Equal(got, original) {
			t.Fatal("target was modified by failed apply")
		}
	}

	corrupt(func(d *Delta) { d.Ops[0].Offset = d.OldSize + 100 })  // out-of-bounds copy
	corrupt(func(d *Delta) { d.Ops[0].Length++ })                 // length mismatch
	corrupt(func(d *Delta) { d.Ops = append(d.Ops, Op{Op: "hack"}) }) // illegal op
	corrupt(func(d *Delta) { d.NewSHA256 = "00" })                // digest mismatch
}

func TestWrongBasisRejected(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeTemp(t, dir, "old.bin", []byte("the real basis file"))
	sigPath := filepath.Join(dir, "sig.json")
	if err := WriteSignature(oldPath, 4, sigPath); err != nil {
		t.Fatal(err)
	}
	deltaPath := filepath.Join(dir, "delta.json")
	if _, err := WriteDelta(sigPath, oldPath, deltaPath); err != nil {
		t.Fatal(err)
	}
	otherPath := writeTemp(t, dir, "other.bin", []byte("the real basis filf"))
	if _, err := Apply(otherPath, deltaPath, filepath.Join(dir, "out.bin")); err == nil {
		t.Fatal("delta applied to the wrong basis")
	}
}

func TestTargetMustDifferFromOld(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeTemp(t, dir, "old.bin", []byte("data"))
	sigPath := filepath.Join(dir, "sig.json")
	if err := WriteSignature(oldPath, 4, sigPath); err != nil {
		t.Fatal(err)
	}
	deltaPath := filepath.Join(dir, "delta.json")
	if _, err := WriteDelta(sigPath, oldPath, deltaPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(oldPath, deltaPath, oldPath); err == nil {
		t.Fatal("target equal to old path was accepted")
	}
}

func TestEmptyAndShortFiles(t *testing.T) {
	st, got := roundTrip(t, nil, nil, 1024)
	if len(got) != 0 || st.Reused != 0 || st.Literal != 0 {
		t.Fatalf("empty round trip failed: %+v", st)
	}
	// Short tail block reused: new file is the old tail alone.
	old := append(make([]byte, 1024), []byte("short-tail")...)
	st, got = roundTrip(t, old, []byte("short-tail"), 1024)
	if !bytes.Equal(got, []byte("short-tail")) {
		t.Fatal("tail reuse mismatch")
	}
	if st.Reused != 10 || st.Literal != 0 {
		t.Fatalf("expected tail reuse, got %+v", st)
	}
}

func TestNoWholeLiteralEscape(t *testing.T) {
	// A shifted copy must still be matched, not emitted as one big literal.
	body := make([]byte, 8192)
	rand.Read(body)
	newData := append([]byte{0x00}, body...)
	st, _ := roundTrip(t, body, newData, 512)
	if st.Literal != 1 || st.Reused != int64(len(body)) {
		t.Fatalf("matching escaped into literals: %+v", st)
	}
}
