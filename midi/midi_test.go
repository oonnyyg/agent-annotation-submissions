package midi

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// smfBuilder assembles a minimal Standard MIDI File for tests.
type smfBuilder struct {
	format   uint16
	division uint16
	tracks   [][]byte
}

func newSMF(format, division uint16) *smfBuilder {
	return &smfBuilder{format: format, division: division}
}

func (b *smfBuilder) track(events ...[]byte) *smfBuilder {
	var body []byte
	for _, e := range events {
		body = append(body, e...)
	}
	b.tracks = append(b.tracks, body)
	return b
}

func (b *smfBuilder) build() []byte {
	var out bytes.Buffer
	out.WriteString("MThd")
	binary.Write(&out, binary.BigEndian, uint32(6))
	binary.Write(&out, binary.BigEndian, b.format)
	binary.Write(&out, binary.BigEndian, uint16(len(b.tracks)))
	binary.Write(&out, binary.BigEndian, b.division)
	for _, t := range b.tracks {
		out.WriteString("MTrk")
		binary.Write(&out, binary.BigEndian, uint32(len(t)))
		out.Write(t)
	}
	return out.Bytes()
}

// ev prefixes a delta-time varint to raw event bytes.
func ev(delta uint32, data ...byte) []byte {
	return append(varlen(delta), data...)
}

func varlen(v uint32) []byte {
	var buf [4]byte
	i := len(buf) - 1
	buf[i] = byte(v & 0x7f)
	for v >>= 7; v > 0; v >>= 7 {
		i--
		buf[i] = byte(v&0x7f) | 0x80
	}
	return buf[i:]
}

func noteOn(delta uint32, ch, pitch, vel uint8) []byte {
	return ev(delta, 0x90|ch, pitch, vel)
}

func noteOff(delta uint32, ch, pitch, vel uint8) []byte {
	return ev(delta, 0x80|ch, pitch, vel)
}

func cc(delta uint32, ch, controller, value uint8) []byte {
	return ev(delta, 0xb0|ch, controller, value)
}

func tempo(delta uint32, micros uint32) []byte {
	return ev(delta, 0xff, 0x51, 0x03,
		byte(micros>>16), byte(micros>>8), byte(micros))
}

func endOfTrack(delta uint32) []byte {
	return ev(delta, 0xff, 0x2f, 0x00)
}

func TestParseRejectsUnsupportedFiles(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"format2", newSMF(2, 480).track(endOfTrack(0)).build()},
		{"smpte", newSMF(1, 0xE728).track(endOfTrack(0)).build()},
		{"short", []byte{0x4d, 0x54}},
		{"badmagic", append([]byte("XXXX"), newSMF(1, 480).track(endOfTrack(0)).build()[4:]...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil {
				t.Fatal("expected error")
			}
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("expected *ParseError, got %T", err)
			}
		})
	}
}

func TestParseTruncatedTrackReportsOffset(t *testing.T) {
	// Track declares more bytes than the file holds.
	data := newSMF(1, 480).track(noteOn(0, 0, 60, 100)).build()
	data = data[:len(data)-2] // cut inside the track body
	_, err := Parse(data)
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T (%v)", err, err)
	}
	if pe.Offset <= 0 || pe.Offset >= len(newSMF(1, 480).track(noteOn(0, 0, 60, 100)).build()) {
		t.Fatalf("offset %d out of range", pe.Offset)
	}
}

func TestParseRunningStatus(t *testing.T) {
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100),
		ev(480, 60, 0), // running status: note-on velocity 0
		endOfTrack(0),
	).build()
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []EventKind
	for _, e := range f.Events {
		kinds = append(kinds, e.Kind)
	}
	if len(f.Events) != 3 || kinds[0] != EventNoteOn || kinds[1] != EventNoteOn {
		t.Fatalf("unexpected events: %+v", f.Events)
	}
	if f.Events[1].Velocity != 0 || f.Events[1].Tick != 480 {
		t.Fatalf("running-status event wrong: %+v", f.Events[1])
	}
}

func TestMergedEventsStableOrder(t *testing.T) {
	data := newSMF(1, 480).
		track(noteOn(0, 0, 60, 90), noteOff(480, 0, 60, 0), endOfTrack(0)).
		track(noteOn(0, 1, 64, 90), noteOff(240, 1, 64, 0), endOfTrack(0)).
		build()
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := f.MergedEvents()
	for i := 1; i < len(m); i++ {
		a, b := m[i-1], m[i]
		if a.Tick > b.Tick ||
			(a.Tick == b.Tick && a.Track > b.Track) ||
			(a.Tick == b.Tick && a.Track == b.Track && a.Seq > b.Seq) {
			t.Fatalf("merge order broken at %d: %+v then %+v", i, a, b)
		}
	}
}
