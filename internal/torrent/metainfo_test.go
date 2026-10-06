package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"testing"
)

// unsortedInfo has deliberately unsorted keys; the info hash must still be
// computed over the original bytes, not a re-encoded dictionary.
const unsortedInfo = "d6:pieces20:AAAAAAAAAAAAAAAAAAAA4:name4:test12:piece lengthi4e6:lengthi4ee"

func TestInfoHashUsesRawBytes(t *testing.T) {
	data := []byte("d4:info" + unsortedInfo + "e")
	m, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := sha1.Sum([]byte(unsortedInfo))
	if m.InfoHash != want {
		t.Fatalf("info hash %x, want %x", m.InfoHash, want)
	}
	if len(m.Warnings) == 0 {
		t.Fatal("expected unsorted-keys warning")
	}
	if m.PieceLength != 4 || len(m.Pieces) != 1 || m.TotalSize != 4 {
		t.Fatalf("meta: %+v", m)
	}
}

func TestPieceCountMismatch(t *testing.T) {
	// 8 bytes at piece length 4 needs 2 hashes; supply 1.
	data := []byte("d4:infod6:lengthi8e4:name4:test12:piece lengthi4e6:pieces20:AAAAAAAAAAAAAAAAAAAAee")
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "piece hashes") {
		t.Fatalf("err: %v", err)
	}
}

func TestDuplicateKeyRejected(t *testing.T) {
	data := []byte("d4:infod4:name1:a4:name1:bee")
	if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err: %v", err)
	}
}

func TestInfoHashHexStable(t *testing.T) {
	data := []byte("d4:infod6:lengthi0e4:name1:x12:piece lengthi4e6:pieces0:ee")
	m, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(m.InfoHash[:]); len(got) != 40 {
		t.Fatalf("hash: %s", got)
	}
}
