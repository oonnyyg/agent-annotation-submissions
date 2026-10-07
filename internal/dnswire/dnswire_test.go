package dnswire

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func sampleMessage() *Message {
	return &Message{
		Header: Header{ID: 0x1234, QR: true, Opcode: 0, AA: true, RD: true, RA: true, Rcode: 0},
		Questions: []Question{
			{Name: Name{"WWW", "Example", "COM"}, Type: "A", Class: "IN"},
		},
		Answers: []Record{
			{Name: Name{"WWW", "Example", "COM"}, Type: "A", Class: "IN", TTL: 300,
				RData: RData{A: "192.0.2.1"}},
			{Name: Name{"alias", "Example", "COM"}, Type: "CNAME", Class: "IN", TTL: 60,
				RData: RData{CNAME: Name{"WWW", "Example", "COM"}}},
			{Name: Name{"Example", "COM"}, Type: "MX", Class: "IN", TTL: 60,
				RData: RData{MX: &MXData{Preference: 10, Exchange: Name{"mail", "Example", "COM"}}}},
			{Name: Name{"Example", "COM"}, Type: "TXT", Class: "IN", TTL: 60,
				RData: RData{TXT: []string{"v=spf1 -all", "", "hex:00ff"}}},
		},
		Authorities: []Record{
			{Name: Name{}, Type: "TXT", Class: "IN", TTL: 0, RData: RData{TXT: []string{"root"}}},
		},
	}
}

func roundTrip(t *testing.T, m *Message) *Message {
	t.Helper()
	wire, err := EncodeMessage(m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return back
}

func TestRoundTripCompressed(t *testing.T) {
	m := sampleMessage()
	back := roundTrip(t, m)
	if len(back.Questions) != 1 || back.Questions[0].Name[0] != "WWW" {
		t.Fatalf("question lost case or labels: %+v", back.Questions)
	}
	if back.Answers[1].RData.CNAME[0] != "WWW" {
		t.Fatalf("cname case not preserved: %+v", back.Answers[1].RData.CNAME)
	}
	txts := back.Answers[3].RData.TXT
	if len(txts) != 3 || txts[0] != "v=spf1 -all" || txts[1] != "" || txts[2] != "hex:00ff" {
		t.Fatalf("txt strings mangled: %#v", txts)
	}
	if back.Authorities[0].Name == nil || len(back.Authorities[0].Name) != 0 {
		t.Fatalf("root name not preserved: %#v", back.Authorities[0].Name)
	}
}

func TestCompressionActuallyUsed(t *testing.T) {
	wire, err := EncodeMessage(sampleMessage())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range wire {
		if b&0xC0 == 0xC0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected at least one compression pointer")
	}
}

// encodeUncompressed renders a message with no compression pointers.
func encodeUncompressed(t *testing.T, m *Message) []byte {
	t.Helper()
	var buf bytes.Buffer
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint16(hdr[0:2], m.Header.ID)
	binary.BigEndian.PutUint16(hdr[4:6], uint16(len(m.Questions)))
	binary.BigEndian.PutUint16(hdr[6:8], uint16(len(m.Answers)))
	buf.Write(hdr)
	writeName := func(n Name) {
		for _, l := range n {
			buf.WriteByte(byte(len(l)))
			buf.WriteString(l)
		}
		buf.WriteByte(0)
	}
	for _, q := range m.Questions {
		writeName(q.Name)
		binary.Write(&buf, binary.BigEndian, typeCodes[q.Type])
		binary.Write(&buf, binary.BigEndian, classIN)
	}
	for _, r := range m.Answers {
		writeName(r.Name)
		binary.Write(&buf, binary.BigEndian, typeCodes[r.Type])
		binary.Write(&buf, binary.BigEndian, classIN)
		binary.Write(&buf, binary.BigEndian, r.TTL)
		switch r.Type {
		case "A":
			binary.Write(&buf, binary.BigEndian, uint16(4))
			buf.Write([]byte{192, 0, 2, 1})
		case "CNAME":
			var tmp bytes.Buffer
			for _, l := range r.RData.CNAME {
				tmp.WriteByte(byte(len(l)))
				tmp.WriteString(l)
			}
			tmp.WriteByte(0)
			body := tmp.Bytes()
			binary.Write(&buf, binary.BigEndian, uint16(len(body)))
			buf.Write(body)
		}
	}
	return buf.Bytes()
}

func TestRoundTripUncompressed(t *testing.T) {
	m := &Message{
		Header:    Header{ID: 7, RD: true},
		Questions: []Question{{Name: Name{"a", "b"}, Type: "A", Class: "IN"}},
		Answers: []Record{
			{Name: Name{"a", "b"}, Type: "A", Class: "IN", TTL: 1, RData: RData{A: "192.0.2.1"}},
			{Name: Name{"x", "a", "b"}, Type: "CNAME", Class: "IN", TTL: 1,
				RData: RData{CNAME: Name{"a", "b"}}},
		},
	}
	wire := encodeUncompressed(t, m)
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatalf("decode uncompressed: %v", err)
	}
	if back.Answers[1].RData.CNAME[0] != "a" {
		t.Fatalf("bad cname: %+v", back.Answers[1].RData.CNAME)
	}
}

func TestMultiLevelPointer(t *testing.T) {
	// qname at 12: www.example.com; answer name -> 12; cname rdata -> 16 (example.com)
	wire := []byte{
		0, 1, 0x81, 0x80, 0, 1, 0, 2, 0, 0, 0, 0,
		3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0,
		0, 1, 0, 1,
	}
	a := []byte{0xC0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 1}
	cname := []byte{0xC0, 12, 0, 5, 0, 1, 0, 0, 0, 60, 0, 2, 0xC0, 16}
	wire = append(wire, a...)
	wire = append(wire, cname...)
	m, err := DecodeMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Answers[1].RData.CNAME, "."); got != "example.com" {
		t.Fatalf("multi-level pointer decoded wrong: %s", got)
	}
}

func TestUnknownTypeRawOnly(t *testing.T) {
	m := &Message{
		Header:  Header{ID: 1},
		Answers: []Record{{Name: Name{"x"}, Type: "TYPE99", Class: "IN", TTL: 0, RData: RData{Hex: "deadbeef"}}},
	}
	wire, err := EncodeMessage(m)
	if err == nil {
		t.Fatalf("expected encode refusal, got %x", wire)
	}
	// decode side: build by hand
	raw := []byte{0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 'x', 0, 0, 99, 0, 1, 0, 0, 0, 0, 0, 4, 0xde, 0xad, 0xbe, 0xef}
	back, err := DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.Answers[0].RData.Hex != "deadbeef" {
		t.Fatalf("unknown rdata not raw: %+v", back.Answers[0].RData)
	}
}

func TestPointerOffsetOverflowFallsBackToLabels(t *testing.T) {
	// Push the first occurrence of a shared suffix past 0x3FFF, then repeat
	// it: the encoder must emit plain labels instead of an illegal pointer.
	m := &Message{Header: Header{ID: 1}}
	for i := 0; i < 700; i++ {
		m.Questions = append(m.Questions, Question{
			Name:  Name{strings.Repeat("a", 20) + string(rune('a'+i%26)), "pad"},
			Type:  "A",
			Class: "IN",
		})
	}
	m.Questions = append(m.Questions,
		Question{Name: Name{"shared-suffix", "example"}, Type: "A", Class: "IN"},
		Question{Name: Name{"again", "shared-suffix", "example"}, Type: "A", Class: "IN"},
	)
	wire, err := EncodeMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatalf("round trip with large offsets: %v", err)
	}
	last := back.Questions[len(back.Questions)-1]
	if strings.Join(last.Name, ".") != "again.shared-suffix.example" {
		t.Fatalf("suffix corrupted: %v", last.Name)
	}
}
