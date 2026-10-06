package delta

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func makeSig(t *testing.T, data []byte, blockSize int) *Signature {
	t.Helper()
	sig, err := BuildSignature(bytes.NewReader(data), blockSize)
	if err != nil {
		t.Fatalf("BuildSignature: %v", err)
	}
	if err := sig.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return sig
}

func roundTrip(t *testing.T, oldData, newData []byte, blockSize int) (reused, literal int64) {
	t.Helper()
	sig := makeSig(t, oldData, blockSize)
	d, reused, literal, err := Generate(sig, newData)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := d.Validate(sig); err != nil {
		t.Fatalf("delta Validate: %v", err)
	}
	out, err := Rebuild(d, oldData)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if !bytes.Equal(out, newData) {
		t.Fatalf("round trip mismatch: got %d bytes, want %d", len(out), len(newData))
	}
	return reused, literal
}

func TestHeaderInsert(t *testing.T) {
	var oldData, newData []byte
	oldData = append(oldData, bytes.Repeat([]byte("ABCDEFGH"), 200)...)
	newData = append(newData, []byte("INSERTED-HEADER!")...)
	newData = append(newData, oldData...)
	reused, literal := roundTrip(t, oldData, newData, 64)
	if reused < int64(len(oldData)-64) {
		t.Fatalf("header insert: reused %d bytes, want most of %d", reused, len(oldData))
	}
	if literal != int64(len("INSERTED-HEADER!")) {
		t.Fatalf("header insert: literal %d bytes, want %d", literal, len("INSERTED-HEADER!"))
	}
}

func TestRepeatedBlocks(t *testing.T) {
	blk := bytes.Repeat([]byte("repeat-me-"), 10) // 100 bytes
	var oldData []byte
	for i := 0; i < 5; i++ {
		oldData = append(oldData, blk...)
	}
	newData := append([]byte("xy"), oldData...)
	newData = append(newData, blk...)
	reused, _ := roundTrip(t, oldData, newData, 100)
	if reused != int64(len(oldData)+len(blk)) {
		t.Fatalf("repeated blocks: reused %d, want %d", reused, len(oldData)+len(blk))
	}
	// Same content at many offsets: smallest old offset must be chosen.
	sig := makeSig(t, oldData, 100)
	d, _, _, err := Generate(sig, blk)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Ops) != 1 || d.Ops[0].Op != "copy" || d.Ops[0].Offset != 0 {
		t.Fatalf("repeated blocks: ops %+v, want single copy at offset 0", d.Ops)
	}
}

func TestEmptyFiles(t *testing.T) {
	reused, literal := roundTrip(t, nil, nil, 16)
	if reused != 0 || literal != 0 {
		t.Fatalf("empty: reused %d literal %d", reused, literal)
	}
	roundTrip(t, nil, []byte("from nothing"), 4)
	roundTrip(t, []byte("to nothing"), nil, 4)
}

func TestShortLastBlockReuse(t *testing.T) {
	oldData := append(bytes.Repeat([]byte("0123456789"), 3), []byte("tail!")...)
	newData := append([]byte(">>"), oldData...)
	reused, _ := roundTrip(t, oldData, newData, 10)
	if reused != int64(len(oldData)) {
		t.Fatalf("short last block: reused %d, want %d", reused, len(oldData))
	}
}

func TestRandomData(t *testing.T) {
	oldData := make([]byte, 5000)
	rand.Read(oldData)
	newData := make([]byte, 5000)
	copy(newData, oldData)
	newData[123] ^= 0xff // single byte change
	reused, literal := roundTrip(t, oldData, newData, 256)
	if literal > 256 {
		t.Fatalf("random: literal %d bytes for one-byte change", literal)
	}
	if reused < 4000 {
		t.Fatalf("random: reused only %d bytes", reused)
	}
}
