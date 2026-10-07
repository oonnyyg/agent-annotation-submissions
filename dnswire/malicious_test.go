package dnswire

import (
	"strings"
	"testing"
)

func headerOnly(qd, an uint16) []byte {
	return []byte{0, 1, 0x81, 0x80, byte(qd >> 8), byte(qd), byte(an >> 8), byte(an), 0, 0, 0, 0}
}

func expectWireErr(t *testing.T, msg []byte, want string) {
	t.Helper()
	_, err := DecodeMessage(msg)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	we, ok := err.(*WireError)
	if !ok {
		t.Fatalf("error is %T, want *WireError", err)
	}
	if we.Offset < 0 || we.Offset > len(msg) {
		t.Fatalf("offset %d outside message of %d bytes", we.Offset, len(msg))
	}
	if !strings.Contains(we.Reason, want) {
		t.Fatalf("reason %q does not contain %q", we.Reason, want)
	}
}

func TestMalformedInputs(t *testing.T) {
	cases := []struct {
		name string
		msg  []byte
		want string
	}{
		{"short header", []byte{0, 1, 2}, "shorter than 12-byte header"},
		{"missing question", headerOnly(1, 0), "truncated"},
		{"truncated label", append(headerOnly(1, 0), 5, 'a', 'b'), "truncated"},
		{"reserved label type", append(headerOnly(1, 0), 0x40, 0), "reserved label type"},
		{"pointer truncated", append(headerOnly(1, 0), 0xC0), "pointer truncated"},
		{"pointer out of bounds", append(headerOnly(1, 0), 0xC0, 0x7F), "out of bounds"},
		{"pointer self loop", append(headerOnly(1, 0), 0xC0, 12), "loop"},
		{"pointer cycle", append(headerOnly(1, 0), 0xC0, 14, 0xC0, 12), "loop"},
		{"non-ascii label", append(headerOnly(1, 0), 1, 0xC3, 0, 0, 1, 0, 1), "non-ASCII"},
		{"trailing bytes", append(headerOnly(0, 0), 0), "trailing bytes"},
		{
			"rdlen overflow",
			append(headerOnly(0, 1), 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 10, 1, 2),
			"exceeds message",
		},
		{
			"A rdlen wrong",
			append(headerOnly(0, 1), 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 3, 1, 2, 3),
			"must be 4 bytes",
		},
		{
			"TXT string overruns rdata",
			append(headerOnly(0, 1), 0, 0, 16, 0, 1, 0, 0, 0, 0, 0, 2, 5, 'a'),
			"exceeds RDATA boundary",
		},
		{
			"CNAME rdata boundary mismatch",
			append(headerOnly(0, 1), 0, 0, 5, 0, 1, 0, 0, 0, 0, 0, 4, 1, 'a', 0, 0),
			"RDATA declares",
		},
		{
			"MX too short",
			append(headerOnly(0, 1), 0, 0, 15, 0, 1, 0, 0, 0, 0, 0, 2, 0, 10),
			"too short",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectWireErr(t, tc.msg, tc.want)
		})
	}
}

func TestExpandedNameTooLong(t *testing.T) {
	// Two 130-byte expanded halves pointing at each other's tails would
	// loop; instead chain: name = 63-label, ptr to another 63-label, ptr
	// to another 63-label, ptr to another 63-label, ptr to root => 257.
	msg := headerOnly(4, 0)
	mk := func(target int) []byte {
		l := append([]byte{63}, bytes63('x')...)
		return append(l, 0xC0, byte(target))
	}
	p1 := len(msg)
	msg = append(msg, mk(0)...) // placeholder, fix below
	p2 := len(msg)
	msg = append(msg, mk(0)...)
	p3 := len(msg)
	msg = append(msg, mk(0)...)
	p4 := len(msg)
	msg = append(msg, mk(0)...)
	// Chain p1->p2->p3->p4->root(0 terminator at header? use a zero byte).
	msg = append(msg, 0)
	root := len(msg) - 1
	for _, p := range []struct{ at, to int }{{p1, p2}, {p2, p3}, {p3, p4}, {p4, root}} {
		msg[p.at+64] = 0xC0
		msg[p.at+65] = byte(p.to)
	}
	msg[4] = 0
	msg[5] = 1 // qdcount = 1
	expectWireErr(t, msg, "exceeds 255")
}

func bytes63(c byte) []byte {
	b := make([]byte, 63)
	for i := range b {
		b[i] = c
	}
	return b
}

func TestEncodeRejectsUnknown(t *testing.T) {
	m := &Message{
		Answers: []Record{
			{Name: NameResult{Labels: []string{"x"}}, Type: 99, Class: 1, Raw: "deadbeef"},
		},
	}
	if _, err := EncodeMessage(m); err == nil || !strings.Contains(err.Error(), "raw") {
		t.Fatalf("expected raw rejection, got %v", err)
	}
	m2 := &Message{
		Answers: []Record{
			{Name: NameResult{Labels: []string{"x"}}, Type: 1, Class: 3, Data: "1.2.3.4"},
		},
	}
	if _, err := EncodeMessage(m2); err == nil || !strings.Contains(err.Error(), "IN class") {
		t.Fatalf("expected class rejection, got %v", err)
	}
}

func TestEncodeFallbackBeyondPointerRange(t *testing.T) {
	enc := newNameEncoder()
	// Pad the buffer beyond the 14-bit pointer range, then encode a name:
	// its suffix must not be registered for compression.
	buf := make([]byte, maxPtrOff+1)
	var err error
	buf, err = enc.appendName(buf, Name{"pad", "example"})
	if err != nil {
		t.Fatal(err)
	}
	// Now encode a name at a representable offset... it is not, so reset:
	enc2 := newNameEncoder()
	buf2, err := enc2.appendName(nil, Name{"a", "example"})
	if err != nil {
		t.Fatal(err)
	}
	// Force the suffix table to contain an out-of-range offset and verify
	// the encoder emits plain labels instead of an illegal pointer.
	enc2.suffixes[suffixKey(Name{"example"})] = maxPtrOff + 5
	out, err := enc2.appendName(nil, Name{"b", "example"})
	if err != nil {
		t.Fatal(err)
	}
	if out[1]&ptrMask == ptrMask {
		t.Fatalf("emitted pointer to out-of-range offset: %x", out)
	}
	want := []byte{1, 'b', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0}
	if string(out) != string(want) {
		t.Fatalf("got %x want %x", out, want)
	}
	_ = buf2
}
