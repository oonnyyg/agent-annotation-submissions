package wal

import (
	"bytes"
	"testing"
)

func TestCorruptFrameStopsPrefix(t *testing.T) {
	good := buildWAL(t, false, []testFrame{{pgno: 1, dbsize: 1, fill: 0x01}})
	bad := buildWAL(t, false, []testFrame{
		{pgno: 1, dbsize: 1, fill: 0x01},
		{pgno: 2, dbsize: 2, fill: 0x02},
		{pgno: 3, dbsize: 3, fill: 0x03},
	})
	// Corrupt frame 1's page data, then append a perfectly valid frame 2.
	bad[WALHeaderSize+FrameHeaderSize+testPageSize+FrameHeaderSize+10] ^= 0xFF
	res, err := Verify(bad)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 1 {
		t.Fatalf("valid prefix should stop at frame 0, got %d frames", len(res.Frames))
	}
	if res.FirstError == "" || res.ErrorOffset != int64(WALHeaderSize+FrameHeaderSize+testPageSize) {
		t.Fatalf("error=%q offset=%d", res.FirstError, res.ErrorOffset)
	}
	_ = good
}

func TestIncompleteTrailingFrame(t *testing.T) {
	w := buildWAL(t, false, []testFrame{{pgno: 1, dbsize: 1, fill: 0x01}})
	w = append(w, make([]byte, 100)...) // partial frame
	res, err := Verify(w)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TruncatedTail || res.IgnoredFrom != int64(len(w)-100) {
		t.Fatalf("truncated=%v ignoredFrom=%d", res.TruncatedTail, res.IgnoredFrom)
	}
	if len(res.Frames) != 1 {
		t.Fatalf("frames=%d", len(res.Frames))
	}
}

func TestNoCommitKeepsBaseline(t *testing.T) {
	w := buildWAL(t, false, []testFrame{{pgno: 1, dbsize: 0, fill: 0x09}})
	res, err := Verify(w)
	if err != nil {
		t.Fatal(err)
	}
	db := buildDB(t, 2, 0x42)
	snap, err := BuildSnapshot(db, res)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NoCommit || !bytes.Equal(snap.Data, db) {
		t.Fatal("no-commit snapshot must equal the baseline")
	}
}

func TestInvalidWALHeader(t *testing.T) {
	w := buildWAL(t, false, nil)
	w[0] ^= 0xFF // break magic
	if _, err := Verify(w); err == nil {
		t.Fatal("expected magic error")
	}
	w2 := buildWAL(t, false, nil)
	w2[24] ^= 0x01 // break header checksum
	if _, err := Verify(w2); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestMissingPageNotZeroFilled(t *testing.T) {
	w := buildWAL(t, false, []testFrame{{pgno: 3, dbsize: 3, fill: 0x07}})
	res, err := Verify(w)
	if err != nil {
		t.Fatal(err)
	}
	// Baseline has only 1 page; pages 1-2 are absent from WAL and db.
	_, err = BuildSnapshot(buildDB(t, 1, 0x00), res)
	if err == nil {
		t.Fatal("expected missing-page error, must not zero-fill")
	}
}

func TestSaltMismatchStopsPrefix(t *testing.T) {
	w := buildWAL(t, false, []testFrame{
		{pgno: 1, dbsize: 1, fill: 0x01},
		{pgno: 1, dbsize: 1, fill: 0x02},
	})
	w[WALHeaderSize+FrameHeaderSize+testPageSize+8] ^= 0xFF // frame 1 salt1
	res, err := Verify(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 1 || res.FirstError == "" {
		t.Fatalf("frames=%d err=%q", len(res.Frames), res.FirstError)
	}
}

func TestPageSizeMismatch(t *testing.T) {
	w := buildWAL(t, false, []testFrame{{pgno: 1, dbsize: 1, fill: 0x01}})
	res, err := Verify(w)
	if err != nil {
		t.Fatal(err)
	}
	db := buildDB(t, 1, 0x00)
	db[16], db[17] = 0, 0 // page size 65536 marker? use 1024 instead
	db[16] = 4            // 1024
	if _, err := BuildSnapshot(db, res); err == nil {
		t.Fatal("expected page size mismatch error")
	}
}
