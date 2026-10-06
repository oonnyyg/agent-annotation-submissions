package ntpaudit

import (
	"errors"
	"testing"
	"time"
)

func TestParsePacketLength(t *testing.T) {
	if _, err := ParsePacket(make([]byte, 47)); err == nil {
		t.Fatal("47-byte packet accepted")
	}
	if _, err := ParsePacket(make([]byte, 49)); err == nil {
		t.Fatal("49-byte packet (extension) accepted")
	}
	if _, err := ParsePacket(make([]byte, 48)); err != nil {
		t.Fatalf("48-byte packet rejected: %v", err)
	}
}

func TestExpandEraWrap2036(t *testing.T) {
	// 2040 is in NTP era 1: seconds field wrapped past 2036-02-07.
	base := time.Date(2040, 6, 1, 0, 0, 0, 0, time.UTC)
	ts := Timestamp{Seconds: uint32(base.Unix() & 0xffffffff)}
	got, err := ts.Expand(base)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(base) {
		t.Fatalf("got %s, want %s", got, base)
	}
	// A timestamp from just before the 2036 wrap must still resolve to
	// era 0 when the base is just after it, keeping order correct.
	preWrap := time.Date(2036, 2, 1, 0, 0, 0, 0, time.UTC)
	postWrap := time.Date(2036, 3, 1, 0, 0, 0, 0, time.UTC)
	ts = Timestamp{Seconds: uint32(preWrap.Unix() & 0xffffffff)}
	got, err = ts.Expand(postWrap)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(preWrap) {
		t.Fatalf("got %s, want %s", got, preWrap)
	}
	if !got.Before(postWrap) {
		t.Fatal("pre-wrap timestamp did not stay before post-wrap base")
	}
}

func TestExpandFractionExact(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Fraction 0x80000000 is exactly half a second.
	ts := Timestamp{Seconds: uint32(base.Unix()), Fraction: 0x80000000}
	got, err := ts.Expand(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nanosecond() != 500000000 {
		t.Fatalf("half-second fraction expanded to %d ns", got.Nanosecond())
	}
	// Two timestamps a few fraction ticks apart must remain distinct at
	// nanosecond resolution (5 ticks ~= 1.16 ns).
	a := Timestamp{Seconds: uint32(base.Unix()), Fraction: 100}
	b := Timestamp{Seconds: uint32(base.Unix()), Fraction: 105}
	ta, _ := a.Expand(base)
	tb, _ := b.Expand(base)
	if !ta.Before(tb) {
		t.Fatal("adjacent fraction ticks collapsed to the same time")
	}
}

func TestExpandAmbiguous(t *testing.T) {
	// Base exactly half an era (2^31 seconds) after the candidate.
	ts := Timestamp{Seconds: 0}
	base := time.Unix(1<<31, 0).UTC()
	if _, err := ts.Expand(base); !errors.Is(err, ErrEraAmbiguous) {
		t.Fatalf("got %v, want ErrEraAmbiguous", err)
	}
}

func TestValidateResponse(t *testing.T) {
	req := &Packet{Version: 4, Mode: ModeClient,
		Transmit: Timestamp{Seconds: 10, Fraction: 5}}
	if err := req.ValidateRequest(); err != nil {
		t.Fatal(err)
	}
	good := &Packet{Version: 4, Mode: ModeServer, Stratum: 2,
		Origin: req.Transmit}
	if err := good.ValidateResponse(req); err != nil {
		t.Fatal(err)
	}

	kod := *good
	kod.Stratum = 0
	copy(kod.ReferenceID[:], "RATE")
	err := kod.ValidateResponse(req)
	var kodErr *KissOfDeathError
	if !errors.As(err, &kodErr) || kodErr.Code != "RATE" {
		t.Fatalf("kiss-of-death not reported separately: %v", err)
	}

	bad := *good
	bad.Stratum = 16
	if err := bad.ValidateResponse(req); err == nil {
		t.Fatal("stratum 16 accepted")
	}

	bad = *good
	bad.Leap = LeapUnknown
	if err := bad.ValidateResponse(req); err == nil {
		t.Fatal("unsynchronized server accepted")
	}

	bad = *good
	bad.Origin = Timestamp{Seconds: 11}
	if err := bad.ValidateResponse(req); err == nil {
		t.Fatal("mismatched origin timestamp accepted")
	}

	bad = *good
	bad.Mode = 5
	if err := bad.ValidateResponse(req); err == nil {
		t.Fatal("non-server mode accepted")
	}
}
