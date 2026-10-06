package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"example.com/sqlite-wal-snapshot/internal/wal"
)

const pageSize = 512

// buildWAL constructs a small WAL: commit(page1=0x11, size 2), then an
// uncommitted frame(page2=0x22).
func buildWAL(order wal.ByteOrder) []byte {
	magic := wal.MagicLE
	if order == wal.BigEndianChecksum {
		magic = wal.MagicBE
	}
	salt1, salt2 := uint32(7), uint32(9)
	hdr := make([]byte, wal.WalHeaderSize)
	binary.BigEndian.PutUint32(hdr[0:4], magic)
	binary.BigEndian.PutUint32(hdr[4:8], wal.WalVersion)
	binary.BigEndian.PutUint32(hdr[8:12], pageSize)
	binary.BigEndian.PutUint32(hdr[16:20], salt1)
	binary.BigEndian.PutUint32(hdr[20:24], salt2)
	s1, s2 := wal.Checksum(order, 0, 0, hdr[0:24])
	binary.BigEndian.PutUint32(hdr[24:28], s1)
	binary.BigEndian.PutUint32(hdr[28:32], s2)
	out := append([]byte{}, hdr...)

	addFrame := func(pgno, dbSize uint32, fill byte) {
		fh := make([]byte, wal.FrameHdrSize)
		binary.BigEndian.PutUint32(fh[0:4], pgno)
		binary.BigEndian.PutUint32(fh[4:8], dbSize)
		binary.BigEndian.PutUint32(fh[8:12], salt1)
		binary.BigEndian.PutUint32(fh[12:16], salt2)
		data := make([]byte, pageSize)
		for i := range data {
			data[i] = fill
		}
		s1, s2 = wal.Checksum(order, s1, s2, fh[0:8])
		s1, s2 = wal.Checksum(order, s1, s2, data)
		binary.BigEndian.PutUint32(fh[16:20], s1)
		binary.BigEndian.PutUint32(fh[20:24], s2)
		out = append(out, fh...)
		out = append(out, data...)
	}
	addFrame(1, 2, 0x11)
	addFrame(2, 0, 0x22)
	return out
}

func buildBaseline() []byte {
	db := make([]byte, 2*pageSize)
	for i := range db {
		db[i] = 0x55
	}
	copy(db[0:16], []byte("SQLite format 3\x00"))
	binary.BigEndian.PutUint16(db[16:18], pageSize)
	return db
}

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "main.db")
	walPath := filepath.Join(dir, "main.db-wal")
	outPath := filepath.Join(dir, "snapshot.db")
	if err := os.WriteFile(dbPath, buildBaseline(), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walPath, buildWAL(wal.BigEndianChecksum), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := run(dbPath, walPath, outPath); err != nil {
		t.Fatal(err)
	}
	img, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(img) != 2*pageSize {
		t.Fatalf("snapshot size = %d", len(img))
	}
	if img[0] != 0x11 {
		t.Fatalf("page 1 byte = %#x, want 0x11", img[0])
	}
	if img[pageSize] != 0x55 {
		t.Fatalf("page 2 should come from baseline, got %#x", img[pageSize])
	}
	// Refuses to overwrite an existing file.
	if err := run(dbPath, walPath, outPath); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	// Refuses to write onto a source path.
	if err := run(dbPath, walPath, dbPath); err == nil {
		t.Fatal("expected source-path refusal")
	}
}

func TestRunBadWalHeaderNoSnapshot(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "main.db")
	walPath := filepath.Join(dir, "main.db-wal")
	outPath := filepath.Join(dir, "snapshot.db")
	if err := os.WriteFile(dbPath, buildBaseline(), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walPath, make([]byte, 64), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := run(dbPath, walPath, outPath); err != nil {
		t.Fatal(err) // report is still emitted
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatal("snapshot must not be created for an invalid WAL header")
	}
}
