package ogg

import (
	"bytes"
	"strings"
	"testing"
)

func expectErr(t *testing.T, data []byte, maxPkt int64, wantSub string) error {
	t.Helper()
	_, err := Audit(bytes.NewReader(data), maxPkt, nil)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("error %q does not contain %q", err, wantSub)
	}
	return err
}

func TestBadCapturePattern(t *testing.T) {
	data := buildFile(simplePage(1, 0, FlagBOS|FlagEOS, []byte("x")))
	data[0] = 'X'
	err := expectErr(t, data, 0, "bad capture pattern")
	pe, ok := err.(*PageError)
	if !ok || pe.Offset != 0 {
		t.Fatalf("expected PageError at offset 0, got %v", err)
	}
}

func TestCRCCorruption(t *testing.T) {
	good := buildFile(
		simplePage(1, 0, FlagBOS, []byte("ok")),
		simplePage(1, 1, FlagEOS, []byte("fine")),
	)
	bad := append([]byte(nil), good...)
	bad[28] ^= 0xFF // flip a body byte of the first page
	err := expectErr(t, bad, 0, "CRC mismatch")
	if pe, ok := err.(*PageError); !ok || pe.Offset != 0 {
		t.Fatalf("expected PageError at offset 0, got %v", err)
	}
	// Sanity: the unmodified file passes.
	if _, err := Audit(bytes.NewReader(good), 0, nil); err != nil {
		t.Fatalf("good file rejected: %v", err)
	}
}

func TestTruncatedPage(t *testing.T) {
	data := buildFile(simplePage(1, 0, FlagBOS|FlagEOS, []byte("x")))
	expectErr(t, data[:len(data)-1], 0, "truncated segment data")
	expectErr(t, data[:10], 0, "truncated page header")
}

func TestUnsupportedVersion(t *testing.T) {
	data := buildFile(simplePage(1, 0, FlagBOS|FlagEOS, []byte("x")))
	data[4] = 1
	// Recompute CRC so only the version check can fire.
	fixCRC(data)
	expectErr(t, data, 0, "unsupported stream structure version")
}

func fixCRC(page []byte) {
	for i := 22; i < 26; i++ {
		page[i] = 0
	}
	crc := crc32Ogg(page)
	page[22] = byte(crc)
	page[23] = byte(crc >> 8)
	page[24] = byte(crc >> 16)
	page[25] = byte(crc >> 24)
}

func TestSequenceGap(t *testing.T) {
	data := buildFile(
		simplePage(1, 0, FlagBOS, []byte("a")),
		simplePage(1, 2, FlagEOS, []byte("b")),
	)
	expectErr(t, data, 0, "page sequence gap")
}

func TestSequenceWrapAround(t *testing.T) {
	data := buildFile(
		simplePage(1, 0xFFFFFFFE, FlagBOS, []byte("a")),
		simplePage(1, 0xFFFFFFFF, 0, []byte("b")),
		simplePage(1, 0, FlagEOS, []byte("c")),
	)
	if _, err := Audit(bytes.NewReader(data), 0, nil); err != nil {
		t.Fatalf("wrap-around sequence rejected: %v", err)
	}
}

func TestDuplicateBOS(t *testing.T) {
	data := buildFile(
		simplePage(1, 0, FlagBOS, []byte("a")),
		simplePage(1, 1, FlagBOS, []byte("b")),
	)
	expectErr(t, data, 0, "duplicate BOS")
}

func TestPageAfterEOS(t *testing.T) {
	data := buildFile(
		simplePage(1, 0, FlagBOS|FlagEOS, []byte("a")),
		simplePage(1, 1, 0, []byte("b")),
	)
	expectErr(t, data, 0, "new group must start with a BOS page")
}

func TestContinuationWithoutPending(t *testing.T) {
	data := buildFile(
		simplePage(1, 0, FlagBOS, []byte("a")),
		simplePage(1, 1, FlagContinuation|FlagEOS, []byte("b")),
	)
	expectErr(t, data, 0, "continuation flag set but no packet is pending")
}

func TestMissingContinuation(t *testing.T) {
	part := bytes.Repeat([]byte{1}, 255)
	data := buildFile(
		pageSpec{serial: 1, seq: 0, htype: FlagBOS, segments: []byte{255}, body: part},
		simplePage(1, 1, FlagEOS, []byte("b")),
	)
	expectErr(t, data, 0, "lacks continuation flag")
}

func TestUnfinishedPacketAtEOS(t *testing.T) {
	part := bytes.Repeat([]byte{1}, 255)
	data := buildFile(
		pageSpec{serial: 1, seq: 0, htype: FlagBOS, segments: []byte{255}, body: part},
		pageSpec{serial: 1, seq: 1, htype: FlagContinuation | FlagEOS, segments: []byte{255}, body: part},
	)
	expectErr(t, data, 0, "unfinished packet")
}

func TestMissingEOS(t *testing.T) {
	data := buildFile(simplePage(1, 0, FlagBOS, []byte("a")))
	expectErr(t, data, 0, "no end-of-stream page")
}

func TestMaxPacketSize(t *testing.T) {
	big := bytes.Repeat([]byte{7}, 1000)
	data := buildFile(
		pageSpec{serial: 1, seq: 0, htype: FlagBOS, segments: []byte{255, 255, 255, 235}, body: big},
		simplePage(1, 1, FlagEOS, []byte("x")),
	)
	expectErr(t, data, 500, "exceeds maximum size")
	if _, err := Audit(bytes.NewReader(data), 1000, nil); err != nil {
		t.Fatalf("packet at the limit rejected: %v", err)
	}
}

func TestMultiplexedStreams(t *testing.T) {
	data := buildFile(
		simplePage(100, 0, FlagBOS, []byte("a0")),
		simplePage(200, 0, FlagBOS, []byte("b0")),
		simplePage(100, 1, 0, []byte("a1")),
		simplePage(200, 1, FlagEOS, []byte("b1")),
		simplePage(100, 2, FlagEOS, []byte("a2")),
	)
	bySerial := map[uint32][]string{}
	_, err := Audit(bytes.NewReader(data), 0, func(s *StreamResult, p *Packet) error {
		bySerial[s.Serial] = append(bySerial[s.Serial], string(p.Data))
		return nil
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if strings.Join(bySerial[100], ",") != "a0,a1,a2" {
		t.Errorf("stream 100 packets: %v", bySerial[100])
	}
	if strings.Join(bySerial[200], ",") != "b0,b1" {
		t.Errorf("stream 200 packets: %v", bySerial[200])
	}
}

func TestChainedGroups(t *testing.T) {
	data := buildFile(
		simplePage(1, 0, FlagBOS|FlagEOS, []byte("first")),
		simplePage(1, 0, FlagBOS|FlagEOS, []byte("second")),
	)
	res, err := Audit(bytes.NewReader(data), 0, nil)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(res) != 2 || res[0].Group != 0 || res[1].Group != 1 {
		t.Fatalf("expected 2 groups, got %+v", res)
	}
}

func TestEmptyFile(t *testing.T) {
	expectErr(t, nil, 0, "empty file")
}

func TestGranulePreserved(t *testing.T) {
	data := buildFile(simplePage(1, 0, FlagBOS|FlagEOS, []byte("x")))
	// Set a granule with the high bit set; it must survive verbatim.
	page := data
	for i := 6; i < 14; i++ {
		page[i] = 0xFF
	}
	fixCRC(page)
	var granule uint64
	_, err := Audit(bytes.NewReader(data), 0, func(s *StreamResult, p *Packet) error {
		granule = p.Granule
		return nil
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if granule != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("granule = %#x, want 0xFFFFFFFFFFFFFFFF", granule)
	}
}
