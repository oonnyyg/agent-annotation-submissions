package tzif

import (
	"testing"
)

func TestAtUsesNewTypeAtTransition(t *testing.T) {
	f := mustParse(t, sampleFile())
	// One second before the 2200000000 transition: still XST (+8:45).
	_, typ, err := f.At(2199999999)
	if err != nil || typ.Abbr != "XST" {
		t.Fatalf("At(before) = %v, %v", typ, err)
	}
	// At the transition instant: new type XDT (+9:15).
	local, typ, err := f.At(2200000000)
	if err != nil || typ.Abbr != "XDT" || typ.Offset != 9*3600+15*60 {
		t.Fatalf("At(at) = %v, %v", typ, err)
	}
	if local != 2200000000+9*3600+15*60 {
		t.Errorf("local = %d", local)
	}
}

func TestAtOutsideWindow(t *testing.T) {
	f := mustParse(t, sampleFile())
	for _, ts := range []int64{1499999999, 2500000000, 2600000000} {
		if _, _, err := f.At(ts); err == nil {
			t.Errorf("At(%d): want window error", ts)
		} else if _, ok := err.(*ErrOutsideWindow); !ok {
			t.Errorf("At(%d): err = %v, want ErrOutsideWindow", ts, err)
		}
	}
}

func TestAtBeyond2038(t *testing.T) {
	f := mustParse(t, sampleFile())
	// 2400000000 is 2046-01-17, well past 32-bit time_t.
	local, typ, err := f.At(2400000000)
	if err != nil {
		t.Fatal(err)
	}
	if typ.Abbr != "XST" || local != 2400000000+8*3600+45*60 {
		t.Errorf("local=%d typ=%+v", local, typ)
	}
	if got := FormatCivil(local); got != "2046-01-20T03:25:00" {
		t.Errorf("civil = %s", got)
	}
}

func TestFromLocalGap(t *testing.T) {
	f := mustParse(t, sampleFile())
	// Forward jump at 2200000000: +8:45 -> +9:15, 30-min gap in local time.
	gapStart := int64(2200000000) + 8*3600 + 45*60
	res, err := f.FromLocal(gapStart + 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Times) != 0 {
		t.Fatalf("times = %v", res.Times)
	}
	if res.Gap == nil || res.Gap[0] != gapStart || res.Gap[1] != gapStart+1800 {
		t.Fatalf("gap = %v, want [%d,%d)", res.Gap, gapStart, gapStart+1800)
	}
}

func TestFromLocalOverlap(t *testing.T) {
	f := mustParse(t, sampleFile())
	// Backward jump at 2300000000: +9:15 -> +8:45, 30-min overlap.
	// Local time 2300000000+8:45+600 exists in both intervals.
	local := int64(2300000000) + 8*3600 + 45*60 + 600
	res, err := f.FromLocal(local)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Times) != 2 {
		t.Fatalf("times = %v, want 2 candidates", res.Times)
	}
	if res.Times[0] >= res.Times[1] {
		t.Errorf("not sorted: %v", res.Times)
	}
	for _, ts := range res.Times {
		got, _, _ := f.At(ts)
		if got != local {
			t.Errorf("At(%d) = %d, want %d", ts, got, local)
		}
	}
}

func TestFromLocalUnique(t *testing.T) {
	f := mustParse(t, sampleFile())
	res, err := f.FromLocal(1500000000 + 8*3600 + 45*60 + 1)
	if err != nil || len(res.Times) != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestFromLocalOutsideWindow(t *testing.T) {
	f := mustParse(t, sampleFile())
	if _, err := f.FromLocal(0); err == nil {
		t.Fatal("want error for local time before window")
	}
}

func TestIntervals(t *testing.T) {
	f := mustParse(t, sampleFile())
	ivs := f.Intervals()
	if len(ivs) != 3 {
		t.Fatalf("intervals = %v", ivs)
	}
	if ivs[0].Offset != 8*3600+45*60 || ivs[1].Offset != 9*3600+15*60 {
		t.Errorf("offsets: %+v", ivs)
	}
	for i := 1; i < len(ivs); i++ {
		if ivs[i].Start != ivs[i-1].End {
			t.Errorf("gap between intervals %d and %d", i-1, i)
		}
	}
	if ivs[2].End != 2500000000 {
		t.Errorf("last end = %d", ivs[2].End)
	}
}
