package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// crcTable mirrors the Ogg CRC-32 (polynomial 0x04c11db7).
var crcTable = func() [256]uint32 {
	var tab [256]uint32
	for i := 0; i < 256; i++ {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		tab[i] = r
	}
	return tab
}()

func crc32Ogg(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>24)^b]
	}
	return crc
}

func testPage(serial, seq uint32, htype byte, packets ...[]byte) []byte {
	hdr := make([]byte, 27)
	copy(hdr, "OggS")
	hdr[5] = htype
	binary.LittleEndian.PutUint32(hdr[14:18], serial)
	binary.LittleEndian.PutUint32(hdr[18:22], seq)
	hdr[26] = byte(len(packets))
	out := hdr
	for _, p := range packets {
		out = append(out, byte(len(p)))
	}
	for _, p := range packets {
		out = append(out, p...)
	}
	binary.LittleEndian.PutUint32(out[22:26], crc32Ogg(out))
	return out
}

func validFile() []byte {
	var out []byte
	out = append(out, testPage(42, 0, 0x02, []byte("header"))...)
	out = append(out, testPage(42, 1, 0x04, []byte("payload"))...)
	return out
}

func TestRunExportsPacketsAndIndex(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.ogg")
	if err := os.WriteFile(src, validFile(), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := run(src, out, 1<<20, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	pkt, err := os.ReadFile(filepath.Join(out, "stream-0000002a-g0", "packet-000001.bin"))
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(pkt) != "payload" {
		t.Fatalf("packet = %q, want %q", pkt, "payload")
	}
	raw, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx indexJSON
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("parse index: %v", err)
	}
	if len(idx.Streams) != 1 || len(idx.Streams[0].Packets) != 2 {
		t.Fatalf("unexpected index: %+v", idx)
	}
	p0 := idx.Streams[0].Packets[0]
	if p0.Length != 6 || len(p0.SHA256) != 64 || len(p0.Pages) != 1 || p0.Pages[0].Offset != 0 {
		t.Fatalf("bad index entry: %+v", p0)
	}
}

func TestRunFailureKeepsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.ogg")
	corrupt := validFile()
	corrupt[28] ^= 0xFF // break the first page CRC
	if err := os.WriteFile(src, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(out, "previous.txt")
	if err := os.WriteFile(keep, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(src, out, 1<<20, false); err == nil {
		t.Fatal("expected failure for corrupt file")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("existing output was disturbed: %v", err)
	}
	// No staged temp directories may linger.
	matches, _ := filepath.Glob(filepath.Join(dir, ".oggaudit-*"))
	if len(matches) != 0 {
		t.Fatalf("stale temp dirs: %v", matches)
	}
}

func TestCheckOnlyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.ogg")
	if err := os.WriteFile(src, validFile(), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := run(src, out, 1<<20, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("check-only created output dir")
	}
}
