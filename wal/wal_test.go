package wal

import (
	"bytes"
	"encoding/binary"
	"testing"
)

const testPageSize = 512

type testFrame struct {
	pgno   uint32
	dbsize uint32
	fill   byte
}

func buildWAL(t *testing.T, bigEndian bool, frames []testFrame) []byte {
	t.Helper()
	magic := magicLE
	if bigEndian {
		magic = magicBE
	}
	hdr := make([]byte, WALHeaderSize)
	be := binary.BigEndian
	be.PutUint32(hdr[0:4], magic)
	be.PutUint32(hdr[4:8], 3007000)
	be.PutUint32(hdr[8:12], testPageSize)
	be.PutUint32(hdr[12:16], 7)
	be.PutUint32(hdr[16:20], 0xAAAA0001)
	be.PutUint32(hdr[20:24], 0xBBBB0002)
	s1, s2 := checksum(bigEndian, 0, 0, hdr[0:24])
	be.PutUint32(hdr[24:28], s1)
	be.PutUint32(hdr[28:32], s2)

	out := append([]byte{}, hdr...)
	for _, f := range frames {
		fh := make([]byte, FrameHeaderSize)
		be.PutUint32(fh[0:4], f.pgno)
		be.PutUint32(fh[4:8], f.dbsize)
		be.PutUint32(fh[8:12], 0xAAAA0001)
		be.PutUint32(fh[12:16], 0xBBBB0002)
		data := bytes.Repeat([]byte{f.fill}, testPageSize)
		s1, s2 = checksum(bigEndian, s1, s2, fh[0:8])
		s1, s2 = checksum(bigEndian, s1, s2, data)
		be.PutUint32(fh[16:20], s1)
		be.PutUint32(fh[20:24], s2)
		out = append(out, fh...)
		out = append(out, data...)
	}
	return out
}

func buildDB(t *testing.T, pages int, fill byte) []byte {
	t.Helper()
	db := bytes.Repeat([]byte{fill}, pages*testPageSize)
	copy(db[0:16], []byte("SQLite format 3\x00"))
	binary.BigEndian.PutUint16(db[16:18], testPageSize)
	return db
}

func TestVerifyBothByteOrders(t *testing.T) {
	frames := []testFrame{
		{pgno: 1, dbsize: 0, fill: 0x11},
		{pgno: 2, dbsize: 3, fill: 0x22}, // commit
	}
	for _, be := range []bool{false, true} {
		w := buildWAL(t, be, frames)
		res, err := Verify(w)
		if err != nil {
			t.Fatalf("bigEndian=%v: %v", be, err)
		}
		if res.FirstError != "" {
			t.Fatalf("bigEndian=%v: unexpected error %q", be, res.FirstError)
		}
		if len(res.Frames) != 2 || res.LastCommit != 1 {
			t.Fatalf("bigEndian=%v: frames=%d lastCommit=%d", be, len(res.Frames), res.LastCommit)
		}
		if res.Header.BigEndian != be {
			t.Fatalf("bigEndian=%v: header flag mismatch", be)
		}
	}
}

func TestDuplicatePagesLastWriteWins(t *testing.T) {
	w := buildWAL(t, false, []testFrame{
		{pgno: 2, dbsize: 0, fill: 0x01},
		{pgno: 2, dbsize: 0, fill: 0x02},
		{pgno: 2, dbsize: 2, fill: 0x03}, // commit
	})
	res, err := Verify(w)
	if err != nil || res.FirstError != "" {
		t.Fatalf("verify: err=%v first=%q", err, res.FirstError)
	}
	snap, err := BuildSnapshot(buildDB(t, 2, 0x00), res)
	if err != nil {
		t.Fatal(err)
	}
	page2 := snap.Data[testPageSize : 2*testPageSize]
	if !bytes.Equal(page2, bytes.Repeat([]byte{0x03}, testPageSize)) {
		t.Fatal("page 2 should come from the last committed frame")
	}
	if snap.Pages[1].Source != "wal" || snap.Pages[1].FrameIndex != 2 {
		t.Fatalf("page 2 source: %+v", snap.Pages[1])
	}
}

func TestUncommittedTailExcluded(t *testing.T) {
	w := buildWAL(t, true, []testFrame{
		{pgno: 1, dbsize: 2, fill: 0x0A}, // commit
		{pgno: 2, dbsize: 0, fill: 0x0B}, // valid but uncommitted
	})
	res, err := Verify(w)
	if err != nil || res.FirstError != "" {
		t.Fatalf("verify: err=%v first=%q", err, res.FirstError)
	}
	if res.LastCommit != 0 {
		t.Fatalf("lastCommit=%d", res.LastCommit)
	}
	snap, err := BuildSnapshot(buildDB(t, 2, 0x00), res)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap.Data[testPageSize:], buildDB(t, 2, 0x00)[testPageSize:]) {
		t.Fatal("page 2 must come from baseline, not the uncommitted frame")
	}
	if snap.Pages[1].Source != "database" {
		t.Fatalf("page 2 source: %+v", snap.Pages[1])
	}
}
