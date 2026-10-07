package dnswire

import (
	"strings"
	"testing"
)

func expectDecodeErr(t *testing.T, wire []byte, want string) {
	t.Helper()
	_, err := DecodeMessage(wire)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	we, ok := err.(*WireError)
	if !ok {
		t.Fatalf("error is not a WireError: %T", err)
	}
	if we.Offset < 0 || we.Offset > len(wire) {
		t.Fatalf("offset %d outside message of %d bytes", we.Offset, len(wire))
	}
	if !strings.Contains(we.Reason, want) {
		t.Fatalf("reason %q does not contain %q", we.Reason, want)
	}
}

// oneQuestion builds a minimal query for the given qname wire bytes.
func oneQuestion(qname []byte) []byte {
	wire := []byte{0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	wire = append(wire, qname...)
	return append(wire, 0, 1, 0, 1)
}

func TestTruncatedHeader(t *testing.T) {
	expectDecodeErr(t, []byte{0, 1, 0}, "shorter than the 12-byte header")
}

func TestTruncatedName(t *testing.T) {
	expectDecodeErr(t, oneQuestion([]byte{3, 'w', 'w'}), "truncated")
}

func TestTruncatedPointer(t *testing.T) {
	wire := []byte{0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xC0}
	expectDecodeErr(t, wire, "pointer is truncated")
}

func TestPointerIntoHeader(t *testing.T) {
	expectDecodeErr(t, oneQuestion([]byte{0xC0, 0}), "points into the header")
}

func TestPointerOutOfBounds(t *testing.T) {
	expectDecodeErr(t, oneQuestion([]byte{0xC0, 0x7F, 0, 1, 0, 1}), "outside message")
}

func TestPointerLoop(t *testing.T) {
	expectDecodeErr(t, oneQuestion([]byte{0xC0, 12}), "loop")
}

func TestIndirectPointerLoop(t *testing.T) {
	// name at 12 -> 14, name at 14 -> 12
	expectDecodeErr(t, oneQuestion([]byte{0xC0, 14, 0xC0, 12}), "loop")
}

func TestReservedLabelType(t *testing.T) {
	expectDecodeErr(t, oneQuestion([]byte{0x40, 0, 0, 1, 0, 1}), "reserved")
}

func TestTrailingBytes(t *testing.T) {
	wire := append(oneQuestion([]byte{0}), 0xFF)
	expectDecodeErr(t, wire, "trailing bytes")
}

func TestRDataCrossesBoundary(t *testing.T) {
	// A record claiming 5 bytes of rdata.
	wire := []byte{0, 1, 0x81, 0x80, 0, 0, 0, 1, 0, 0, 0, 0,
		0, 0, 1, 0, 1, 0, 0, 0, 60, 0, 5, 1, 2, 3, 4, 5}
	expectDecodeErr(t, wire, "exactly 4 bytes")
}

func TestTXTCrossesBoundary(t *testing.T) {
	// TXT rdata declares a 10-byte string but boundary only holds 2.
	wire := []byte{0, 1, 0x81, 0x80, 0, 0, 0, 1, 0, 0, 0, 0,
		0, 0, 16, 0, 1, 0, 0, 0, 60, 0, 3, 10, 'a', 'b'}
	expectDecodeErr(t, wire, "crosses the rdata boundary")
}

func TestRDataNameEscapesBoundary(t *testing.T) {
	// CNAME whose name is longer than the declared rdlen.
	wire := []byte{0, 1, 0x81, 0x80, 0, 0, 0, 1, 0, 0, 0, 0,
		0, 0, 5, 0, 1, 0, 0, 0, 60, 0, 2, 3, 'a', 'b', 'c', 0}
	expectDecodeErr(t, wire, "declared")
}

func TestEncodeRejectsUnknownType(t *testing.T) {
	m := &Message{Answers: []Record{
		{Name: Name{"x"}, Type: "TYPE99", Class: "IN", RData: RData{Hex: "ff"}},
	}}
	if _, err := EncodeMessage(m); err == nil {
		t.Fatal("expected refusal to encode unknown type")
	}
}

func TestEncodeRejectsBadNames(t *testing.T) {
	long := strings.Repeat("a", 64)
	for _, name := range []Name{{long}, {"a", ""}, {"a", "b\xc3"}} {
		m := &Message{Questions: []Question{{Name: name, Type: "A", Class: "IN"}}}
		if _, err := EncodeMessage(m); err == nil {
			t.Fatalf("expected rejection of name %q", name)
		}
	}
}

func TestEncodeRejectsOversizeName(t *testing.T) {
	var name Name
	for len(name) < 5 {
		name = append(name, strings.Repeat("a", 63))
	}
	m := &Message{Questions: []Question{{Name: name, Type: "A", Class: "IN"}}}
	if _, err := EncodeMessage(m); err == nil {
		t.Fatal("expected rejection of >255-byte name")
	}
}

func TestEncodeRejectsBadRData(t *testing.T) {
	m := &Message{Answers: []Record{
		{Name: Name{"x"}, Type: "A", Class: "IN", RData: RData{A: "999.1.1.1"}},
	}}
	if _, err := EncodeMessage(m); err == nil {
		t.Fatal("expected rejection of bad IPv4")
	}
	m.Answers[0] = Record{Name: Name{"x"}, Type: "TXT", Class: "IN",
		RData: RData{TXT: []string{strings.Repeat("a", 256)}}}
	if _, err := EncodeMessage(m); err == nil {
		t.Fatal("expected rejection of >255-byte TXT string")
	}
}
