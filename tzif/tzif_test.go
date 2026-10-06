package tzif

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// buildTZif constructs a v2/v3 TZif file in memory for tests.
func buildTZif(version byte, trans []int64, types []int, locals []LocalTime, leapcnt int) []byte {
	var v1, v2 bytes.Buffer
	writeBlock := func(w *bytes.Buffer, timeSize int) {
		binary.Write(w, binary.BigEndian, uint32(0)) // isutcnt
		binary.Write(w, binary.BigEndian, uint32(0)) // isstdcnt
		binary.Write(w, binary.BigEndian, uint32(leapcnt))
		binary.Write(w, binary.BigEndian, uint32(len(trans)))
		binary.Write(w, binary.BigEndian, uint32(len(locals)))
		var chars []byte
		abbrIdx := make([]byte, len(locals))
		for i, l := range locals {
			abbrIdx[i] = byte(len(chars))
			chars = append(chars, l.Abbr...)
			chars = append(chars, 0)
		}
		binary.Write(w, binary.BigEndian, uint32(len(chars)))
		for _, t := range trans {
			if timeSize == 8 {
				binary.Write(w, binary.BigEndian, uint64(t))
			} else {
				binary.Write(w, binary.BigEndian, uint32(int32(t)))
			}
		}
		for _, ti := range types {
			w.WriteByte(byte(ti))
		}
		for i, l := range locals {
			binary.Write(w, binary.BigEndian, uint32(int32(l.Offset)))
			var dst byte
			if l.IsDST {
				dst = 1
			}
			w.WriteByte(dst)
			w.WriteByte(abbrIdx[i])
		}
		w.Write(chars)
		for i := 0; i < leapcnt; i++ {
			if timeSize == 8 {
				binary.Write(w, binary.BigEndian, uint64(0))
			} else {
				binary.Write(w, binary.BigEndian, uint32(0))
			}
			binary.Write(w, binary.BigEndian, uint32(0))
		}
	}
	head := func(w *bytes.Buffer) {
		w.WriteString("TZif")
		w.WriteByte(version)
		w.Write(make([]byte, 15))
	}
	head(&v1)
	writeBlock(&v1, 4)
	head(&v2)
	writeBlock(&v2, 8)
	return append(v1.Bytes(), v2.Bytes()...)
}

// testZone: offsets +8:00, then +8:30 (forward 30m jump), then back to +8:00
// (30m overlap), all after 2038.
//   t0 = 2200000000 (2039-09-18T23:06:40Z) type A (+8:00)
//   t1 = 2200086400 type B (+8:30)
//   t2 = 2200172800 type A (+8:00)
func testZone(t *testing.T) *Zone {
	t.Helper()
	locals := []LocalTime{
		{Offset: 8 * 3600, Abbr: "TST"},
		{Offset: 8*3600 + 1800, IsDST: true, Abbr: "TDT"},
	}
	trans := []int64{2200000000, 2200086400, 2200172800, 2200259200}
	types := []int{0, 1, 0, 0}
	z, err := Parse(buildTZif('2', trans, types, locals, 0))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return z
}

func TestAtUsesNewTypeAtTransition(t *testing.T) {
	z := testZone(t)
	wall, lt, err := z.At(2200086400)
	if err != nil {
		t.Fatal(err)
	}
	if lt.Offset != 8*3600+1800 || lt.Abbr != "TDT" {
		t.Fatalf("at transition: got offset %d abbr %q", lt.Offset, lt.Abbr)
	}
	want := time.Unix(2200086400, 0).UTC().Add(8*time.Hour + 30*time.Minute)
	if !wall.Equal(want) {
		t.Fatalf("wall = %v, want %v", wall, want)
	}
	// Just before: old type.
	_, lt, err = z.At(2200086399)
	if err != nil {
		t.Fatal(err)
	}
	if lt.Offset != 8*3600 {
		t.Fatalf("before transition: got offset %d", lt.Offset)
	}
}

func TestWindowEnforced(t *testing.T) {
	z := testZone(t)
	for _, u := range []int64{2199999999, 2200259200, 2200259201} {
		if _, _, err := z.At(u); err == nil {
			t.Fatalf("At(%d): expected out-of-window error", u)
		} else if !strings.Contains(err.Error(), "outside service window") {
			t.Fatalf("At(%d): unexpected error %v", u, err)
		}
	}
	if _, _, err := z.At(2200000000); err != nil {
		t.Fatalf("At(first): %v", err)
	}
}

func TestLocalGapAndOverlap(t *testing.T) {
	z := testZone(t)
	// Forward jump at t1: local 08:00-based clock skips 30 minutes.
	// gapStart = t1 + 8h, gapEnd = t1 + 8.5h (in unix-as-local terms).
	gapStart := time.Unix(2200086400+8*3600, 0).UTC()
	missing := gapStart.Add(15 * time.Minute)
	matches, gap := z.LookupLocal(missing)
	if len(matches) != 0 {
		t.Fatalf("gap local: got %d matches", len(matches))
	}
	if gap == nil {
		t.Fatal("gap local: expected gap report")
	}
	if !gap.StartLocal.Equal(gapStart) || !gap.EndLocal.Equal(gapStart.Add(30*time.Minute)) {
		t.Fatalf("gap = %v..%v", gap.StartLocal, gap.EndLocal)
	}
	// Backward jump at t2: 30 minutes of local time occur twice.
	dup := time.Unix(2200172800+8*3600+900, 0).UTC()
	matches, gap = z.LookupLocal(dup)
	if gap != nil {
		t.Fatalf("overlap local: unexpected gap %v", gap)
	}
	if len(matches) != 2 {
		t.Fatalf("overlap local: got %d matches, want 2", len(matches))
	}
	if !(matches[0].Unix < matches[1].Unix) {
		t.Fatal("matches not sorted")
	}
	if matches[0].Abbr != "TDT" || matches[1].Abbr != "TST" {
		t.Fatalf("match abbrs: %q %q", matches[0].Abbr, matches[1].Abbr)
	}
	// Ordinary local time: exactly one match.
	one := time.Unix(2200040000+8*3600, 0).UTC()
	matches, gap = z.LookupLocal(one)
	if len(matches) != 1 || gap != nil || matches[0].Unix != 2200040000 {
		t.Fatalf("single match: %v %v", matches, gap)
	}
}

func TestIntervals(t *testing.T) {
	z := testZone(t)
	iv := z.Intervals()
	if len(iv) != 3 {
		t.Fatalf("got %d intervals", len(iv))
	}
	if iv[0].Offset != 28800 || iv[1].Offset != 30600 || iv[2].Offset != 28800 {
		t.Fatalf("offsets: %d %d %d", iv[0].Offset, iv[1].Offset, iv[2].Offset)
	}
	if iv[0].EndUnix != iv[1].StartUnix || iv[1].EndUnix != iv[2].StartUnix {
		t.Fatal("interval boundaries not contiguous")
	}
	if iv[2].EndUnix != 2200259200 {
		t.Fatalf("last interval end = %d", iv[2].EndUnix)
	}
}