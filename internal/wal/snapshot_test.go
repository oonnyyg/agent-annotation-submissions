package wal

import (
	"bytes"
	"testing"
)

func fillPage(b byte) []byte {
	p := make([]byte, testPageSize)
	for i := range p {
		p[i] = b
	}
	return p
}

// Repeated writes to the same page: the last frame within the committed
// range wins; frames after the last commit are ignored.
func TestSnapshotRepeatedPageWrites(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 0, 0x01) // uncommitted write of page 1
	b.frame(1, 2, 0x02) // commit: page 1 = 0x02
	b.frame(1, 0, 0x03) // post-commit write, must not enter snapshot
	b.frame(2, 0, 0x04) // post-commit write of page 2
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	base := makeBaseline(2)
	snap, err := BuildSnapshot(res, base)
	if err != nil {
		t.Fatal(err)
	}
	if snap.DBSize != 2 {
		t.Fatalf("dbSize = %d", snap.DBSize)
	}
	if !bytes.Equal(snap.Image[0:testPageSize], fillPage(0x02)) {
		t.Fatal("page 1 should come from the committed frame (0x02)")
	}
	if !bytes.Equal(snap.Image[testPageSize:], base[testPageSize:]) {
		t.Fatal("page 2 should come from the baseline")
	}
	if snap.Pages[0].Source != "wal" || snap.Pages[0].Frame != 1 {
		t.Fatalf("page 1 source = %+v", snap.Pages[0])
	}
	if snap.Pages[1].Source != "main" {
		t.Fatalf("page 2 source = %+v", snap.Pages[1])
	}
}

// A valid but uncommitted tail after the last commit must be excluded.
func TestSnapshotIgnoresUncommittedTail(t *testing.T) {
	b := newWalBuilder(BigEndianChecksum)
	b.frame(2, 2, 0x0A) // commit page 2
	b.frame(2, 0, 0x0B) // uncommitted overwrite of page 2
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := BuildSnapshot(res, makeBaseline(2))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap.Image[testPageSize:], fillPage(0x0A)) {
		t.Fatal("uncommitted tail frame leaked into snapshot")
	}
}

// Commit may grow the database; new pages come from WAL frames.
func TestSnapshotExtendsDatabase(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(3, 0, 0x33)
	b.frame(3, 3, 0x34) // commit, db grows to 3 pages
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := BuildSnapshot(res, makeBaseline(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Image) != 3*testPageSize {
		t.Fatalf("image size = %d", len(snap.Image))
	}
	if !bytes.Equal(snap.Image[2*testPageSize:], fillPage(0x34)) {
		t.Fatal("page 3 should be the last committed frame")
	}
}

// Missing baseline pages must be an error, never zero-filled.
func TestSnapshotMissingBaselinePage(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 3, 0x01) // commit says 3 pages, WAL only carries page 1
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildSnapshot(res, makeBaseline(2)) // baseline lacks page 3
	if err == nil {
		t.Fatal("expected missing-page error")
	}
}

// No commit in the valid prefix: keep the baseline verbatim.
func TestSnapshotNoCommit(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 0, 0x09)
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	base := makeBaseline(2)
	snap, err := BuildSnapshot(res, base)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NoCommit {
		t.Fatal("expected NoCommit")
	}
	if !bytes.Equal(snap.Image, base) {
		t.Fatal("no-commit snapshot must equal the baseline")
	}
	for _, p := range snap.Pages {
		if p.Source != "main" {
			t.Fatalf("page %d source = %s", p.Page, p.Source)
		}
	}
}

// Commit may shrink the database; the snapshot is truncated.
func TestSnapshotTruncatesDatabase(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 1, 0x05) // commit, db shrinks to 1 page
	res, err := Scan(b.bytes())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := BuildSnapshot(res, makeBaseline(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Image) != testPageSize {
		t.Fatalf("image size = %d", len(snap.Image))
	}
}
