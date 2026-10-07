package dnswire

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func sampleMessage() *Message {
	return &Message{
		Header: Header{ID: 0x1234, QR: true, RD: true, RA: true},
		Questions: []Question{
			{Name: NameResult{Labels: []string{"www", "Example", "COM"}}, QType: 1, QClass: 1},
		},
		Answers: []Record{
			{Name: NameResult{Labels: []string{"www", "Example", "COM"}}, Type: 1, Class: 1, TTL: 300, Data: "192.0.2.1"},
			{Name: NameResult{Labels: []string{"Example", "COM"}}, Type: 5, Class: 1, TTL: 60, Data: NameResult{Labels: []string{"www", "Example", "COM"}}},
			{Name: NameResult{Labels: []string{"Example", "COM"}}, Type: 15, Class: 1, TTL: 60, Data: MXData{Preference: 10, Exchange: NameResult{Labels: []string{"mail", "Example", "COM"}}}},
			{Name: NameResult{Labels: []string{"Example", "COM"}}, Type: 16, Class: 1, TTL: 60, Data: []string{"v=spf1 -all", "", "second"}},
		},
		Authority: []Record{
			{Name: NameResult{Labels: []string{}}, Type: 1, Class: 1, TTL: 0, Data: "198.51.100.1"},
		},
	}
}

func TestRoundTripUncompressed(t *testing.T) {
	m := &Message{
		Header: Header{ID: 7, QR: true},
		Questions: []Question{
			{Name: NameResult{Labels: []string{"a", "b"}}, QType: 16, QClass: 1},
		},
	}
	wire, err := EncodeMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	// No compression possible with a single name: verify literal layout.
	want := []byte{0, 7, 0x80, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 1, 'b', 0, 0, 16, 0, 1}
	if !bytes.Equal(wire, want) {
		t.Fatalf("wire mismatch:\n got %x\nwant %x", wire, want)
	}
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if back.Questions[0].Name.Labels[0] != "a" || back.Questions[0].Name.OccupiedBytes != 5 {
		t.Fatalf("unexpected decode: %+v", back.Questions[0].Name)
	}
}

func TestRoundTripCompressed(t *testing.T) {
	m := sampleMessage()
	wire, err := EncodeMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint16(wire[6:8]); got != 4 {
		t.Fatalf("ancount = %d, want 4", got)
	}
	// Case must be preserved.
	if got := back.Questions[0].Name.Labels[1]; got != "Example" {
		t.Fatalf("case lost: %q", got)
	}
	// The repeated www.Example.COM answer name must be a 2-byte pointer.
	if got := back.Answers[0].Name.OccupiedBytes; got != 2 {
		t.Fatalf("occupied = %d, want compressed pointer (2)", got)
	}
	if got := back.Answers[0].Name.ExpandedLen; got != 17 {
		t.Fatalf("expanded = %d, want 17", got)
	}
	// A record data survives.
	if back.Answers[0].Data != "192.0.2.1" {
		t.Fatalf("A data = %v", back.Answers[0].Data)
	}
	// CNAME target decoded from RDATA.
	cn, ok := back.Answers[1].Data.(NameResult)
	if !ok || cn.Labels[0] != "www" {
		t.Fatalf("CNAME data = %+v", back.Answers[1].Data)
	}
	// MX preference and exchange.
	mx, ok := back.Answers[2].Data.(MXData)
	if !ok || mx.Preference != 10 || mx.Exchange.Labels[0] != "mail" {
		t.Fatalf("MX data = %+v", back.Answers[2].Data)
	}
	// TXT strings, including the empty one, are preserved individually.
	txts, ok := back.Answers[3].Data.([]string)
	if !ok || len(txts) != 3 || txts[0] != "v=spf1 -all" || txts[1] != "" || txts[2] != "second" {
		t.Fatalf("TXT data = %#v", back.Answers[3].Data)
	}
	// Root name round-trips.
	if len(back.Authority[0].Name.Labels) != 0 {
		t.Fatalf("root name = %+v", back.Authority[0].Name.Labels)
	}
	// Re-encode the decoded message and decode again: stable.
	wire2, err := EncodeMessage(back)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMessage(wire2); err != nil {
		t.Fatalf("second decode: %v", err)
	}
}

func TestMultiLevelPointer(t *testing.T) {
	// q1: a.example (offset 12), q2 name: pointer to b inside rdata name.
	// Layout: header, name "a" ptr->? Build manually:
	// offset 12: 1 'b' 1 'c' 0          (b.c)
	// offset 18: 1 'a' ptr->12          (a.b.c, multi-level via next)
	// offset 22: ptr->18                (a.b.c through one jump)
	wire := []byte{
		0, 1, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0,
		1, 'b', 1, 'c', 0, // 12: b.c
		0, 1, 0, 1, // qtype/qclass for q1
		1, 'a', 0xC0, 12, // 21: a.b.c
		0, 1, 0, 1,
		0xC0, 21, // 27: a.b.c via pointer to pointer target
		0, 1, 0, 1,
	}
	m, err := DecodeMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	q := m.Questions[2].Name
	if q.OccupiedBytes != 2 || q.ExpandedLen != 7 {
		t.Fatalf("occupied=%d expanded=%d", q.OccupiedBytes, q.ExpandedLen)
	}
	if len(q.Labels) != 3 || q.Labels[0] != "a" || q.Labels[2] != "c" {
		t.Fatalf("labels = %v", q.Labels)
	}
}
