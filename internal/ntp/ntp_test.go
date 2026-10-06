package ntp

import (
	"testing"
	"time"
)

func TestParseRejectsWrongLength(t *testing.T) {
	if _, err := Parse(make([]byte, 47)); err == nil {
		t.Fatal("47-byte packet accepted")
	}
	if _, err := Parse(make([]byte, 52)); err == nil {
		t.Fatal("52-byte packet (with extension) accepted")
	}
	if _, err := Parse(make([]byte, 48)); err != nil {
		t.Fatalf("48-byte packet rejected: %v", err)
	}
}

func TestExpandNearEraWrap(t *testing.T) {
	// Era boundary: 2036-02-07T06:28:16Z == NTP seconds 2^32.
	boundary := time.Date(2036, 2, 7, 6, 28, 16, 0, time.UTC)

	// Just after the wrap, a tiny 32-bit value belongs to era 1.
	ts, err := Expand(Timestamp{Sec: 5, Frac: 0x80000000}, boundary.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if ts.Sec != (int64(1)<<32)+5 {
		t.Fatalf("got sec %d, want 2^32+5", ts.Sec)
	}

	// Just before the wrap, a huge 32-bit value belongs to era 0.
	ts, err = Expand(Timestamp{Sec: 0xFFFFFFF0}, boundary.Add(-10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if ts.Sec != 0xFFFFFFF0 {
		t.Fatalf("got sec %d, want 0xFFFFFFF0", ts.Sec)
	}

	// Order is preserved across the wrap: era-1 small > era-0 huge.
	if Sub(Timestamp{Sec: (int64(1) << 32)}, ts) <= 0 {
		t.Fatal("wrap crossing broke ordering")
	}
}

func TestExpandAmbiguous(t *testing.T) {
	base := time.Unix(0, 0).UTC()               // NTP sec 2208988800
	amb := int64(2208988800) - (int64(1) << 31) // exactly half an era below base
	if _, err := Expand(Timestamp{Sec: amb}, base); err == nil {
		t.Fatal("half-era distance not reported as ambiguous")
	}
}

func TestOffsetDelayFractionPrecision(t *testing.T) {
	// Fractional differences must survive: offset 0.5s + 2^-32 s.
	t1 := Timestamp{Sec: 100, Frac: 0}
	t2 := Timestamp{Sec: 100, Frac: 0x80000001} // +0.5s + 1 unit
	t3 := Timestamp{Sec: 101, Frac: 0x80000001}
	t4 := Timestamp{Sec: 101, Frac: 0}
	off, del := OffsetDelay(t1, t2, t3, t4)
	wantOff := time.Duration(500_000_000) + time.Duration(1_000_000_000>>32)
	if off < wantOff-time.Nanosecond || off > wantOff+time.Nanosecond {
		t.Fatalf("offset %v, want ~%v", off, wantOff)
	}
	if del != 0 {
		t.Fatalf("delay %v, want 0", del)
	}
}

func TestValidateResponse(t *testing.T) {
	req := &Packet{Version: 4, Mode: 3, Transmit: Timestamp{Sec: 7, Frac: 9}}
	good := &Packet{Version: 4, Mode: 4, Stratum: 2, Origin: req.Transmit}
	if r := ValidateResponse(good, req); r != nil {
		t.Fatalf("good response rejected: %v", r)
	}

	cases := []struct {
		name string
		mut  func(*Packet)
		kind string
	}{
		{"bad version", func(p *Packet) { p.Version = 3 }, "bad-version"},
		{"bad mode", func(p *Packet) { p.Mode = 5 }, "bad-mode"},
		{"unsynced leap", func(p *Packet) { p.Leap = 3 }, "unsynchronized"},
		{"stratum 16", func(p *Packet) { p.Stratum = 16 }, "bad-stratum"},
		{"origin mismatch", func(p *Packet) { p.Origin = Timestamp{Sec: 8} }, "origin-mismatch"},
	}
	for _, c := range cases {
		p := *good
		c.mut(&p)
		if r := ValidateResponse(&p, req); r == nil || r.Kind != c.kind {
			t.Fatalf("%s: got %v, want kind %q", c.name, r, c.kind)
		}
	}

	kod := *good
	kod.Stratum = 0
	copy(kod.RefID[:], "RATE")
	r := ValidateResponse(&kod, req)
	if r == nil || r.Kind != "kiss-of-death" || r.KissCode != "RATE" {
		t.Fatalf("kiss-of-death: got %v", r)
	}
}
