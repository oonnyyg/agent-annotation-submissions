package ogg

import (
	"bytes"
	"strings"
	"testing"
)

func expectErr(t *testing.T, file []byte, want string) {
	t.Helper()
	err := Audit(NewReader(bytes.NewReader(file)), AuditOptions{}, func(*Packet) error { return nil })
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %v", want, err)
	}
}

func TestSimpleStream(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, []byte("first")),
		singlePacketPage(0, 10, 1, 1, []byte("second")),
		singlePacketPage(FlagEOS, 20, 1, 2, nil), // empty packet
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 3 {
		t.Fatalf("got %d packets", len(pkts))
	}
	if string(pkts[0].Data) != "first" || string(pkts[1].Data) != "second" || len(pkts[2].Data) != 0 {
		t.Fatalf("bad packet data: %q %q %q", pkts[0].Data, pkts[1].Data, pkts[2].Data)
	}
	if pkts[1].GranulePos != 10 {
		t.Fatalf("granule = %d", pkts[1].GranulePos)
	}
	if pkts[0].Index != 0 || pkts[1].Index != 1 || pkts[2].Index != 2 {
		t.Fatalf("bad packet indexes")
	}
}

func TestPacketSpanningPages(t *testing.T) {
	part1 := bytes.Repeat([]byte("a"), 255)
	part2 := []byte("bb")
	file := buildFile(
		pageSpec{headerType: FlagBOS, serial: 1, seq: 0, segments: []uint8{255}, data: part1},
		pageSpec{headerType: FlagContinued | FlagEOS, granule: 5, serial: 1, seq: 1, segments: []uint8{2}, data: part2},
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 1 || len(pkts[0].Data) != 257 {
		t.Fatalf("got %d packets, len %d", len(pkts), len(pkts[0].Data))
	}
	if len(pkts[0].Pages) != 2 {
		t.Fatalf("expected 2 page refs, got %d", len(pkts[0].Pages))
	}
}

func TestPacketLengthMultipleOf255(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 510)
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, data),
		pageSpec{headerType: FlagEOS, granule: 1, serial: 1, seq: 1},
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 1 || len(pkts[0].Data) != 510 {
		t.Fatalf("got %d packets, len %d", len(pkts), len(pkts[0].Data))
	}
}

func TestSequenceGapDetected(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagEOS, 0, 1, 2, nil), // skipped seq 1
	)
	expectErr(t, file, "sequence gap")
}

func TestSequenceWrapAroundAllowed(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0xFFFFFFFF, nil),
		singlePacketPage(FlagEOS, 0, 1, 0, nil),
	)
	collectPackets(t, file, AuditOptions{})
}

func TestDuplicateBOS(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagBOS, 0, 1, 1, nil),
	)
	expectErr(t, file, "duplicate BOS")
}

func TestPageAfterEOS(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagEOS, 0, 1, 1, nil),
		singlePacketPage(0, 0, 1, 2, nil),
	)
	expectErr(t, file, "without BOS flag")
}

func TestPageAfterEOSWithinGroup(t *testing.T) {
	// A second stream is still open, so the group is active and the
	// extra page for the ended stream is caught precisely.
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagBOS, 0, 2, 0, nil),
		singlePacketPage(FlagEOS, 0, 1, 1, nil),
		singlePacketPage(0, 0, 1, 2, nil),
	)
	expectErr(t, file, "after its EOS")
}

func TestContinuedWithoutPending(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagContinued|FlagEOS, 0, 1, 1, []byte("x")),
	)
	expectErr(t, file, "continued flag with no packet in progress")
}

func TestMissingContinuedFlag(t *testing.T) {
	part := bytes.Repeat([]byte("a"), 255)
	file := buildFile(
		pageSpec{headerType: FlagBOS, serial: 1, seq: 0, segments: []uint8{255}, data: part},
		singlePacketPage(FlagEOS, 0, 1, 1, nil), // lacks continued flag
	)
	expectErr(t, file, "lacks continued flag")
}

func TestUnfinishedPacketAtEOF(t *testing.T) {
	part := bytes.Repeat([]byte("a"), 255)
	file := buildFile(
		pageSpec{headerType: FlagBOS, serial: 1, seq: 0, segments: []uint8{255}, data: part},
		pageSpec{headerType: FlagContinued | FlagEOS, serial: 1, seq: 1, segments: []uint8{255}, data: part},
	)
	expectErr(t, file, "unfinished packet")
}

func TestMissingEOS(t *testing.T) {
	file := buildFile(singlePacketPage(FlagBOS, 0, 1, 0, nil))
	expectErr(t, file, "missing EOS")
}

func TestInterleavedStreams(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, []byte("a0")),
		singlePacketPage(FlagBOS, 0, 2, 0, []byte("b0")),
		singlePacketPage(0, 1, 1, 1, []byte("a1")),
		singlePacketPage(0, 1, 2, 1, []byte("b1")),
		singlePacketPage(FlagEOS, 2, 1, 2, nil),
		singlePacketPage(FlagEOS, 2, 2, 2, nil),
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 6 {
		t.Fatalf("got %d packets", len(pkts))
	}
	if pkts[0].Serial != 1 || pkts[1].Serial != 2 || pkts[2].Serial != 1 {
		t.Fatalf("interleaving not preserved")
	}
}

func TestBOSAfterDataInGroup(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(0, 0, 1, 1, nil),
		singlePacketPage(FlagBOS, 0, 2, 0, nil), // late BOS
	)
	expectErr(t, file, "after data pages")
}

func TestChainedGroups(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, []byte("g1")),
		singlePacketPage(FlagEOS, 1, 1, 1, nil),
		singlePacketPage(FlagBOS, 0, 9, 0, []byte("g2")),
		singlePacketPage(FlagEOS, 1, 9, 1, nil),
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 4 || pkts[2].Serial != 9 {
		t.Fatalf("chained groups failed: %+v", pkts)
	}
}

func TestNewStreamWhileGroupActive(t *testing.T) {
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, nil),
		singlePacketPage(FlagEOS, 0, 1, 1, nil),
		singlePacketPage(FlagBOS, 0, 2, 0, nil), // ok: previous group ended
		singlePacketPage(FlagEOS, 0, 2, 1, nil),
	)
	collectPackets(t, file, AuditOptions{})
}

func TestPacketSizeLimit(t *testing.T) {
	big := bytes.Repeat([]byte("z"), 1024)
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, big),
		pageSpec{headerType: FlagEOS, serial: 1, seq: 1},
	)
	err := Audit(NewReader(bytes.NewReader(file)), AuditOptions{MaxPacketSize: 512}, func(*Packet) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("expected size limit error, got %v", err)
	}
	_ = collectPackets(t, file, AuditOptions{MaxPacketSize: 2048})
}

func TestPacketSizeLimitDefault(t *testing.T) {
	// 1024-byte packet must pass the default limit.
	big := bytes.Repeat([]byte("z"), 1024)
	file := buildFile(
		singlePacketPage(FlagBOS, 0, 1, 0, big),
		pageSpec{headerType: FlagEOS, serial: 1, seq: 1},
	)
	pkts := collectPackets(t, file, AuditOptions{})
	if len(pkts) != 1 || len(pkts[0].Data) != 1024 {
		t.Fatalf("default limit rejected valid packet")
	}
}
