package audit

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"example.com/ntp-exchange-auditor/internal/ntp"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ts(t time.Time) ntp.Timestamp {
	return ntp.Timestamp{
		Sec:  t.Unix() + 2208988800,
		Frac: uint32(uint64(t.Nanosecond()) << 32 / 1_000_000_000),
	}
}

func pkt(leap, ver, mode, stratum uint8, refID string, origin, recv, xmit ntp.Timestamp) string {
	b := make([]byte, 48)
	b[0] = leap<<6 | ver<<3 | mode
	b[1] = stratum
	copy(b[12:16], refID)
	put := func(off int, t ntp.Timestamp) {
		binary.BigEndian.PutUint32(b[off:], uint32(t.Sec))
		binary.BigEndian.PutUint32(b[off+4:], t.Frac)
	}
	put(24, origin)
	put(32, recv)
	put(40, xmit)
	return hex.EncodeToString(b)
}

// exchange builds a record where the server clock is offset by off and the
// round-trip delay is rtt, with zero server processing time.
func exchange(line int, sentAt time.Time, off, rtt time.Duration, xmit ntp.Timestamp) Record {
	t2 := sentAt.Add(rtt/2 + off)
	t3 := t2
	recvAt := sentAt.Add(rtt)
	req := pkt(0, 4, 3, 0, "", ntp.Timestamp{}, ntp.Timestamp{}, xmit)
	resp := pkt(0, 4, 4, 2, "", xmit, ts(t2), ts(t3))
	return Record{
		Peer:     "srv",
		Request:  req,
		Response: resp,
		SentAt:   sentAt.Format(time.RFC3339Nano),
		RecvAt:   recvAt.Format(time.RFC3339Nano),
	}
}

func TestAnalyzeValid(t *testing.T) {
	rec := exchange(1, base, 50*time.Millisecond, 100*time.Millisecond, ts(base))
	res := Analyze(1, rec)
	if !res.OK {
		t.Fatalf("rejected: %s %s", res.Kind, res.Detail)
	}
	if d := res.Sample.Offset - 50*time.Millisecond; d < -time.Microsecond || d > time.Microsecond {
		t.Fatalf("offset %v, want ~50ms", res.Sample.Offset)
	}
	if d := res.Sample.Delay - 100*time.Millisecond; d < -time.Microsecond || d > time.Microsecond {
		t.Fatalf("delay %v, want ~100ms", res.Sample.Delay)
	}
}

func TestAnalyzeRejects(t *testing.T) {
	xmit := ts(base)
	mk := func(mutResp func(*Record)) Record {
		r := exchange(1, base, 0, 100*time.Millisecond, xmit)
		mutResp(&r)
		return r
	}
	cases := []struct {
		name string
		rec  Record
		kind string
	}{
		{"short response", mk(func(r *Record) { r.Response = r.Response[:90] }), "bad-response"},
		{"unsynced", mk(func(r *Record) {
			r.Response = pkt(3, 4, 4, 2, "", xmit, ts(base), ts(base))
		}), "unsynchronized"},
		{"kiss of death", mk(func(r *Record) {
			r.Response = pkt(0, 4, 4, 0, "RATE", xmit, ts(base), ts(base))
		}), "kiss-of-death"},
		{"bad stratum", mk(func(r *Record) {
			r.Response = pkt(0, 4, 4, 16, "", xmit, ts(base), ts(base))
		}), "bad-stratum"},
		{"origin mismatch", mk(func(r *Record) {
			r.Response = pkt(0, 4, 4, 2, "", ts(base.Add(time.Second)), ts(base), ts(base))
		}), "origin-mismatch"},
		{"server xmit before recv", mk(func(r *Record) {
			r.Response = pkt(0, 4, 4, 2, "", xmit, ts(base.Add(50*time.Millisecond)), ts(base.Add(40*time.Millisecond)))
		}), "invalid-sample"},
		{"negative delay", mk(func(r *Record) {
			r.Response = pkt(0, 4, 4, 2, "", xmit, ts(base.Add(2*time.Second)), ts(base.Add(3*time.Second)))
		}), "invalid-sample"},
	}
	for _, c := range cases {
		if res := Analyze(1, c.rec); res.OK || res.Kind != c.kind {
			t.Errorf("%s: got %+v, want kind %q", c.name, res, c.kind)
		}
	}
}

func TestDuplicateResponse(t *testing.T) {
	p := NewPeer("srv", 4)
	rec := exchange(1, base, 0, 100*time.Millisecond, ts(base))
	if p.Add(rec, Analyze(1, rec)) {
		t.Fatal("first response flagged duplicate")
	}
	// Same request again (retransmitted reply recorded later in the log).
	rec2 := exchange(2, base, 0, 100*time.Millisecond, ts(base))
	if !p.Add(rec2, Analyze(2, rec2)) {
		t.Fatal("second response to same request not flagged duplicate")
	}
	if n := len(p.Samples()); n != 1 {
		t.Fatalf("window has %d samples, want 1", n)
	}
}

func TestWindowKeepsRecentValid(t *testing.T) {
	p := NewPeer("srv", 2)
	for i := 0; i < 4; i++ {
		sent := base.Add(time.Duration(i) * time.Minute)
		rec := exchange(i+1, sent, time.Duration(i+1)*time.Millisecond, 100*time.Millisecond, ts(sent))
		rec.Peer = "srv"
		p.Add(rec, Analyze(i+1, rec))
		// Errors and duplicates must not evict valid samples.
		bad := Record{Peer: "srv", Request: "zz", Response: "zz", SentAt: "x", RecvAt: "y"}
		p.Add(bad, Analyze(99, bad))
	}
	ss := p.Samples()
	if len(ss) != 2 || ss[0].Line != 3 || ss[1].Line != 4 {
		t.Fatalf("window = %+v, want lines 3 and 4", ss)
	}
}

func TestBestMinDelayLatestTie(t *testing.T) {
	p := NewPeer("srv", 8)
	delays := []time.Duration{100, 80, 80, 120}
	for i, d := range delays {
		sent := base.Add(time.Duration(i) * time.Minute)
		rec := exchange(i+1, sent, 0, d*time.Millisecond, ts(sent))
		p.Add(rec, Analyze(i+1, rec))
	}
	best, ok := p.Best()
	if !ok {
		t.Fatal("no best sample")
	}
	// Two samples tie at 80ms delay; the latest (line 3) wins.
	if best.Line != 3 {
		t.Fatalf("best from line %d, want 3", best.Line)
	}
}

func TestJitter(t *testing.T) {
	p := NewPeer("srv", 8)
	if _, ok := p.Best(); ok {
		t.Fatal("empty peer has a best sample")
	}
	if p.Jitter() != 0 {
		t.Fatal("empty peer jitter nonzero")
	}
	sent := base
	rec := exchange(1, sent, 0, 100*time.Millisecond, ts(sent))
	p.Add(rec, Analyze(1, rec))
	if p.Jitter() != 0 {
		t.Fatal("single-sample jitter must be zero")
	}
	// Second sample with identical delay but +4ms offset: jitter 4ms.
	sent2 := base.Add(time.Minute)
	rec2 := exchange(2, sent2, 4*time.Millisecond, 100*time.Millisecond, ts(sent2))
	p.Add(rec2, Analyze(2, rec2))
	if d := p.Jitter() - 4*time.Millisecond; d < -time.Microsecond || d > time.Microsecond {
		t.Fatalf("jitter %v, want ~4ms", p.Jitter())
	}
}

func TestEraWrapExchange(t *testing.T) {
	// Exchange straddling the 2036 era boundary must stay consistent.
	wrap := time.Date(2036, 2, 7, 6, 28, 16, 0, time.UTC)
	sent := wrap.Add(-50 * time.Millisecond)
	rec := exchange(1, sent, 25*time.Millisecond, 100*time.Millisecond, ts(sent))
	res := Analyze(1, rec)
	if !res.OK {
		t.Fatalf("wrap exchange rejected: %s %s", res.Kind, res.Detail)
	}
	if d := res.Sample.Offset - 25*time.Millisecond; d < -time.Microsecond || d > time.Microsecond {
		t.Fatalf("offset %v, want ~25ms", res.Sample.Offset)
	}
}
