package ntpaudit

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

var testBase = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// near reports whether d is within a few nanoseconds of want, tolerating
// sub-nanosecond truncation in the NTP fraction <-> nanosecond conversion.
func near(d, want time.Duration) bool {
	diff := d - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 4*time.Nanosecond
}

func tsFromTime(t time.Time) Timestamp {
	return Timestamp{
		Seconds:  uint32(t.Unix() & 0xffffffff),
		Fraction: uint32(uint64(t.Nanosecond()) * (1 << 32) / uint64(time.Second)),
	}
}

func putTs(b []byte, ts Timestamp) {
	binary.BigEndian.PutUint32(b[0:4], ts.Seconds)
	binary.BigEndian.PutUint32(b[4:8], ts.Fraction)
}

func requestBytes(t1 time.Time) []byte {
	b := make([]byte, PacketLen)
	b[0] = 4<<3 | ModeClient
	putTs(b[40:48], tsFromTime(t1))
	return b
}

func responseBytes(req []byte, t2, t3 time.Time) []byte {
	b := make([]byte, PacketLen)
	b[0] = 4<<3 | ModeServer
	b[1] = 2
	copy(b[24:32], req[40:48])
	putTs(b[32:40], tsFromTime(t2))
	putTs(b[40:48], tsFromTime(t3))
	return b
}

func makeRecord(peer string, t1, t2, t3, t4 time.Time) Record {
	req := requestBytes(t1)
	resp := responseBytes(req, t2, t3)
	return Record{
		Peer:          peer,
		RequestHex:    hex.EncodeToString(req),
		ResponseHex:   hex.EncodeToString(resp),
		SentLocal:     t1,
		ReceivedLocal: t4,
		ApproxUTC:     testBase,
	}
}

func TestProcessAccepted(t *testing.T) {
	a := NewAuditor(8)
	t1 := testBase
	t2 := t1.Add(60 * time.Millisecond)
	t3 := t2.Add(5 * time.Millisecond)
	t4 := t1.Add(25 * time.Millisecond)
	an := a.Process(1, makeRecord("a", t1, t2, t3, t4))
	if an.Status != StatusAccepted {
		t.Fatalf("status %s: %s", an.Status, an.Reason)
	}
	if an.Sample.Delay != 20*time.Millisecond {
		t.Fatalf("delay %s, want 20ms", an.Sample.Delay)
	}
	if !near(an.Sample.Offset, 50*time.Millisecond) {
		t.Fatalf("offset %s, want 50ms", an.Sample.Offset)
	}
}

func TestProcessInvalidSamples(t *testing.T) {
	a := NewAuditor(8)
	t1 := testBase
	// Server transmit precedes server receive.
	an := a.Process(1, makeRecord("a", t1, t1.Add(100*time.Millisecond), t1.Add(90*time.Millisecond), t1.Add(120*time.Millisecond)))
	if an.Status != StatusRejected {
		t.Fatalf("t3<t2: status %s", an.Status)
	}
	// Negative delay: server processing exceeds total round trip.
	an = a.Process(2, makeRecord("a", t1, t1.Add(100*time.Millisecond), t1.Add(200*time.Millisecond), t1.Add(90*time.Millisecond)))
	if an.Status != StatusRejected {
		t.Fatalf("negative delay: status %s", an.Status)
	}
	sums := a.Summaries()
	if len(sums[0].Window) != 0 {
		t.Fatal("invalid samples entered the window")
	}
}

func TestDuplicateResponse(t *testing.T) {
	a := NewAuditor(8)
	t1 := testBase
	rec := makeRecord("a", t1, t1.Add(110*time.Millisecond), t1.Add(115*time.Millisecond), t1.Add(120*time.Millisecond))
	if an := a.Process(1, rec); an.Status != StatusAccepted {
		t.Fatalf("first response: %s", an.Status)
	}
	if an := a.Process(2, rec); an.Status != StatusDuplicate {
		t.Fatalf("second response: %s", an.Status)
	}
	if n := len(a.Summaries()[0].Window); n != 1 {
		t.Fatalf("window size %d, want 1", n)
	}
}

// addValid feeds one accepted sample with the given offset (symmetric 10ms
// legs, 5ms server processing, 20ms round-trip delay).
func addValid(t *testing.T, a *Auditor, peer string, seq int, offset time.Duration) {
	t.Helper()
	t1 := testBase.Add(time.Duration(seq) * time.Second)
	t2 := t1.Add(10*time.Millisecond + offset)
	t3 := t2.Add(5 * time.Millisecond)
	t4 := t1.Add(25 * time.Millisecond)
	an := a.Process(seq, makeRecord(peer, t1, t2, t3, t4))
	if an.Status != StatusAccepted {
		t.Fatalf("seq %d: %s %s", seq, an.Status, an.Reason)
	}
}

func TestWindowEvictionAndSelection(t *testing.T) {
	a := NewAuditor(2)
	addValid(t, a, "a", 1, 10*time.Millisecond)
	addValid(t, a, "a", 2, 20*time.Millisecond)
	addValid(t, a, "a", 3, 30*time.Millisecond)
	sum := a.Summaries()[0]
	if len(sum.Window) != 2 {
		t.Fatalf("window size %d, want 2", len(sum.Window))
	}
	if !near(sum.Window[0].Offset, 20*time.Millisecond) {
		t.Fatal("oldest sample was not evicted")
	}
	// Equal delays: the most recent sample wins.
	if !near(sum.Selected.Offset, 30*time.Millisecond) {
		t.Fatalf("selected %s, want latest 30ms", sum.Selected.Offset)
	}
	if !near(sum.Jitter, 10*time.Millisecond) {
		t.Fatalf("jitter %s, want 10ms", sum.Jitter)
	}
}

func TestMinDelaySelection(t *testing.T) {
	a := NewAuditor(4)
	addValid(t, a, "a", 1, 10*time.Millisecond)
	// A rejected record must not disturb the window.
	t1 := testBase.Add(2 * time.Second)
	a.Process(2, makeRecord("a", t1, t1.Add(100*time.Millisecond), t1.Add(90*time.Millisecond), t1.Add(120*time.Millisecond)))
	sum := a.Summaries()[0]
	if len(sum.Window) != 1 {
		t.Fatal("rejected record disturbed the window")
	}
	if sum.Jitter != 0 {
		t.Fatalf("single-sample jitter %s, want 0", sum.Jitter)
	}
	if !near(sum.Selected.Offset, 10*time.Millisecond) {
		t.Fatalf("selected %s, want 10ms", sum.Selected.Offset)
	}
}
