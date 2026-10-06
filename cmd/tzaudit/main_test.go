package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"example.com/tzif-transition-audit/tzif"
)

// writeSampleTZif writes a small v2 file with a forward and a backward jump.
func writeSampleTZif(t *testing.T, dir string) string {
	t.Helper()
	var data []byte
	be := binary.BigEndian.AppendUint32
	hdr := func() []byte {
		h := append([]byte("TZif2"), make([]byte, 15)...)
		for _, v := range []uint32{0, 0, 0, 4, 2, 8} {
			h = be(h, v)
		}
		return h
	}
	block := func(wide bool) []byte {
		var b []byte
		for _, ts := range []int64{1000, 5000, 9000, 13000} {
			if wide {
				b = binary.BigEndian.AppendUint64(b, uint64(ts))
			} else {
				b = be(b, uint32(int32(ts)))
			}
		}
		b = append(b, 0, 1, 0, 1) // type indices
		// type 0: +3600 "AAA"; type 1: +7200 "BBB"
		b = be(b, 3600)
		b = append(b, 0, 0)
		b = be(b, 7200)
		b = append(b, 1, 4)
		b = append(b, []byte("AAA\x00BBB\x00")...)
		return b
	}
	data = append(data, hdr()...)
	data = append(data, block(false)...)
	data = append(data, hdr()...)
	data = append(data, block(true)...)
	p := filepath.Join(dir, "sample.tzif")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, path string) *tzif.File {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := tzif.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRunQueryAt(t *testing.T) {
	dir := t.TempDir()
	f := load(t, writeSampleTZif(t, dir))
	u := int64(5000)
	r := runQuery(f, 0, query{Type: "at", Unix: &u})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if *r.Offset != 7200 || r.Abbr != "BBB" || r.Local != "1970-01-01T03:23:20" {
		t.Errorf("r = %+v", r)
	}
	u = 13000 // window end: rejected
	r = runQuery(f, 1, query{Type: "at", Unix: &u})
	if r.Error == "" {
		t.Error("want window error at last transition")
	}
}

func TestRunQueryLocalGapAndOverlap(t *testing.T) {
	dir := t.TempDir()
	f := load(t, writeSampleTZif(t, dir))
	// Forward jump at 5000 (+1h -> +2h): local 02:46:40 never occurs.
	r := runQuery(f, 0, query{Type: "local", Local: "1970-01-01T02:46:40"})
	if r.Error != "" || r.Gap == nil {
		t.Fatalf("r = %+v", r)
	}
	if r.Gap.StartLocal != "1970-01-01T02:23:20" || r.Gap.EndLocal != "1970-01-01T03:23:20" {
		t.Errorf("gap = %+v", r.Gap)
	}
	// Backward jump at 9000 (+2h -> +1h): local 03:53:20 occurs twice.
	r = runQuery(f, 1, query{Type: "local", Local: "1970-01-01T03:53:20"})
	if len(r.Matches) != 2 {
		t.Fatalf("matches = %+v", r.Matches)
	}
	if r.Matches[0].Unix >= r.Matches[1].Unix {
		t.Errorf("matches not sorted: %+v", r.Matches)
	}
}

func TestRunQueryIntervals(t *testing.T) {
	dir := t.TempDir()
	f := load(t, writeSampleTZif(t, dir))
	r := runQuery(f, 0, query{Type: "intervals"})
	if len(r.Intervals) != 3 {
		t.Fatalf("intervals = %+v", r.Intervals)
	}
	if r.Intervals[0].Offset != 3600 || r.Intervals[1].Offset != 7200 || r.Intervals[2].Offset != 3600 {
		t.Errorf("intervals = %+v", r.Intervals)
	}
}

func TestQueriesAreIndependent(t *testing.T) {
	dir := t.TempDir()
	f := load(t, writeSampleTZif(t, dir))
	bad := runQuery(f, 0, query{Type: "bogus"})
	good := runQuery(f, 1, query{Type: "intervals"})
	if bad.Error == "" || good.Error != "" {
		t.Errorf("bad=%+v good=%+v", bad, good)
	}
}
