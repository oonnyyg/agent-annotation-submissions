package ogg

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// pageSpec describes a synthetic page for test fixtures.
type pageSpec struct {
	serial   uint32
	seq      uint32
	htype    byte
	granule  uint64
	segments []byte
	body     []byte
}

func buildPage(sp pageSpec) []byte {
	hdr := make([]byte, 27)
	copy(hdr[0:4], "OggS")
	hdr[5] = sp.htype
	binary.LittleEndian.PutUint64(hdr[6:14], sp.granule)
	binary.LittleEndian.PutUint32(hdr[14:18], sp.serial)
	binary.LittleEndian.PutUint32(hdr[18:22], sp.seq)
	hdr[26] = byte(len(sp.segments))
	out := append(hdr, sp.segments...)
	out = append(out, sp.body...)
	binary.LittleEndian.PutUint32(out[22:26], crc32Ogg(out))
	return out
}

func buildFile(specs ...pageSpec) []byte {
	var buf bytes.Buffer
	for _, sp := range specs {
		buf.Write(buildPage(sp))
	}
	return buf.Bytes()
}

// simplePage builds a page whose packets are each one segment.
func simplePage(serial, seq uint32, htype byte, packets ...[]byte) pageSpec {
	sp := pageSpec{serial: serial, seq: seq, htype: htype}
	for _, p := range packets {
		sp.segments = append(sp.segments, byte(len(p)))
		sp.body = append(sp.body, p...)
	}
	return sp
}

func collect(t *testing.T, data []byte, maxPkt int64) []*StreamResult {
	t.Helper()
	var got []struct {
		serial uint32
		data   []byte
	}
	res, err := Audit(bytes.NewReader(data), maxPkt, func(s *StreamResult, p *Packet) error {
		got = append(got, struct {
			serial uint32
			data   []byte
		}{s.Serial, p.Data})
		return nil
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	_ = got
	return res
}

func packetsOf(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var out [][]byte
	_, err := Audit(bytes.NewReader(data), 0, func(s *StreamResult, p *Packet) error {
		cp := append([]byte(nil), p.Data...)
		out = append(out, cp)
		return nil
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	return out
}

func TestValidSingleStream(t *testing.T) {
	data := buildFile(
		simplePage(7, 0, FlagBOS, []byte("head")),
		simplePage(7, 1, 0, []byte("a"), []byte("bb")),
		simplePage(7, 2, FlagEOS, []byte("ccc")),
	)
	pkts := packetsOf(t, data)
	want := []string{"head", "a", "bb", "ccc"}
	if len(pkts) != len(want) {
		t.Fatalf("got %d packets, want %d", len(pkts), len(want))
	}
	for i, w := range want {
		if string(pkts[i]) != w {
			t.Errorf("packet %d = %q, want %q", i, pkts[i], w)
		}
	}
}

func TestEmptyAnd255MultiplePackets(t *testing.T) {
	// Packet 1: empty (single lacing value 0).
	// Packet 2: exactly 255 bytes (lacing 255 then 0).
	// Packet 3: 510 bytes (255, 255, 0).
	body255 := bytes.Repeat([]byte{0xAB}, 255)
	body510 := bytes.Repeat([]byte{0xCD}, 510)
	data := buildFile(
		pageSpec{serial: 1, seq: 0, htype: FlagBOS,
			segments: []byte{0, 255, 0, 255, 255, 0},
			body:     append(append([]byte{}, body255...), body510...)},
		pageSpec{serial: 1, seq: 1, htype: FlagEOS, segments: []byte{0}},
	)
	pkts := packetsOf(t, data)
	if len(pkts) != 4 {
		t.Fatalf("got %d packets, want 4", len(pkts))
	}
	if len(pkts[0]) != 0 {
		t.Errorf("packet 0: got %d bytes, want empty", len(pkts[0]))
	}
	if !bytes.Equal(pkts[1], body255) {
		t.Errorf("packet 1 mismatch")
	}
	if !bytes.Equal(pkts[2], body510) {
		t.Errorf("packet 2 mismatch")
	}
	if len(pkts[3]) != 0 {
		t.Errorf("packet 3: got %d bytes, want empty", len(pkts[3]))
	}
}

func TestPacketSpanningPages(t *testing.T) {
	part1 := bytes.Repeat([]byte{1}, 255)
	part2 := []byte("tail")
	data := buildFile(
		pageSpec{serial: 9, seq: 0, htype: FlagBOS, segments: []byte{255}, body: part1},
		pageSpec{serial: 9, seq: 1, htype: FlagContinuation | FlagEOS, segments: []byte{4}, body: part2},
	)
	pkts := packetsOf(t, data)
	if len(pkts) != 1 {
		t.Fatalf("got %d packets, want 1", len(pkts))
	}
	if !bytes.Equal(pkts[0], append(part1, part2...)) {
		t.Errorf("reassembled packet mismatch")
	}
}
