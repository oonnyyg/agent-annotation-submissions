package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

// buildPCAP assembles a classic pcap byte stream.
func buildPCAP(order binary.ByteOrder, nano bool, records [][]byte) []byte {
	var buf bytes.Buffer
	var magic uint32 = 0xa1b2c3d4
	if nano {
		magic = 0xa1b23c4d
	}
	binary.Write(&buf, order, magic)
	binary.Write(&buf, order, uint16(2))
	binary.Write(&buf, order, uint16(4))
	binary.Write(&buf, order, int32(0))
	binary.Write(&buf, order, uint32(0))
	binary.Write(&buf, order, uint32(65535))
	binary.Write(&buf, order, uint32(1)) // Ethernet
	for i, rec := range records {
		binary.Write(&buf, order, uint32(1700000000+i))
		binary.Write(&buf, order, uint32(12345))
		binary.Write(&buf, order, uint32(len(rec)))
		binary.Write(&buf, order, uint32(len(rec)))
		buf.Write(rec)
	}
	return buf.Bytes()
}

var (
	srcIP = []byte{10, 0, 0, 1}
	dstIP = []byte{10, 0, 0, 2}
)

// buildFragment builds an Ethernet/IPv4 frame carrying a UDP fragment.
func buildFragment(id uint16, fragField uint16, payload []byte) []byte {
	total := 20 + len(payload)
	ip := make([]byte, 20)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(total))
	binary.BigEndian.PutUint16(ip[4:6], id)
	binary.BigEndian.PutUint16(ip[6:8], fragField)
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:16], srcIP)
	copy(ip[16:20], dstIP)
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))
	frame := make([]byte, 14, 14+total)
	frame[3] = 0x11
	frame[7] = 0x22
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	frame = append(frame, ip...)
	frame = append(frame, payload...)
	return frame
}

// buildUDPPayload builds a UDP datagram (header+payload) with valid checksum.
func buildUDPPayload(srcPort, dstPort uint16, payload []byte) []byte {
	udp := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(len(udp)))
	copy(udp[8:], payload)
	src, _ := netipAddr(srcIP)
	dst, _ := netipAddr(dstIP)
	c := udpChecksum(src, dst, udp)
	if c == 0 {
		c = 0xffff
	}
	binary.BigEndian.PutUint16(udp[6:8], c)
	return udp
}

func netipAddr(b []byte) (netip.Addr, bool) {
	return netip.AddrFromSlice(b)
}

func runSummary(t *testing.T, pcap []byte) (summaryOut, []datagramOut) {
	t.Helper()
	var dg, sm bytes.Buffer
	if err := run(bytes.NewReader(pcap), &dg, &sm); err != nil {
		t.Fatalf("run: %v", err)
	}
	var sum summaryOut
	if err := json.Unmarshal(sm.Bytes(), &sum); err != nil {
		t.Fatalf("summary json: %v\n%s", err, sm.String())
	}
	var dgs []datagramOut
	for _, line := range strings.Split(strings.TrimSpace(dg.String()), "\n") {
		if line == "" {
			continue
		}
		var d datagramOut
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("datagram json: %v", err)
		}
		dgs = append(dgs, d)
	}
	return sum, dgs
}

const mf = 0x2000 // more-fragments flag

func TestReassembleOutOfOrder(t *testing.T) {
	udp := buildUDPPayload(1234, 53, []byte("hello-fragmented-world")) // 30 bytes
	// split: [0,16) MF, [16,30) last
	f1 := buildFragment(7, mf|0, udp[:16])
	f2 := buildFragment(7, 2, udp[16:]) // offset 2 (units of 8) = byte 16
	// send out of order
	sum, dgs := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f2, f1}))
	if sum.Complete != 1 || len(dgs) != 1 {
		t.Fatalf("want 1 complete datagram, got %+v / %d", sum, len(dgs))
	}
	if string(dgs[0].Payload) != "hello-fragmented-world" {
		t.Fatalf("payload = %q", dgs[0].Payload)
	}
	if len(sum.Flows) != 1 || sum.Flows[0].PayloadBytes != 22 || sum.Flows[0].Datagrams != 1 {
		t.Fatalf("flows = %+v", sum.Flows)
	}
	if len(sum.Anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", sum.Anomalies)
	}
}

func TestEndiannessAndNano(t *testing.T) {
	udp := buildUDPPayload(9, 9, []byte("12345678"))
	frame := buildFragment(1, 0, udp)
	for _, tc := range []struct {
		order binary.ByteOrder
		nano  bool
	}{
		{binary.LittleEndian, false},
		{binary.BigEndian, false},
		{binary.LittleEndian, true},
		{binary.BigEndian, true},
	} {
		sum, dgs := runSummary(t, buildPCAP(tc.order, tc.nano, [][]byte{frame}))
		if sum.Complete != 1 || len(dgs) != 1 {
			t.Fatalf("order=%v nano=%v: got %+v", tc.order, tc.nano, sum)
		}
	}
}

func TestConflictingOverlap(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	f1 := buildFragment(5, mf|0, udp[:16])
	f2 := buildFragment(5, 2, udp[16:])
	// overlapping fragment covering bytes [8,16) with different content
	bad := make([]byte, 8)
	copy(bad, udp[8:16])
	bad[0] ^= 0xff
	f3 := buildFragment(5, mf|1, bad)
	sum, dgs := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f1, f2, f3}))
	if sum.Complete != 0 || len(dgs) != 0 {
		t.Fatalf("conflict must not decode: %+v / %v", sum, dgs)
	}
	found := false
	for _, a := range sum.Anomalies {
		if a.Kind == "conflict" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected conflict anomaly: %+v", sum.Anomalies)
	}
}

func TestIdenticalDuplicateFragment(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	f1 := buildFragment(5, mf|0, udp[:16])
	f2 := buildFragment(5, 2, udp[16:])
	dup := buildFragment(5, mf|1, udp[8:16]) // identical retransmit
	sum, dgs := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f1, dup, f2}))
	if sum.Complete != 1 || len(dgs) != 1 {
		t.Fatalf("duplicate identical fragment should be fine: %+v", sum)
	}
	if sum.Flows[0].PayloadBytes != 8 {
		t.Fatalf("payload bytes must not double count: %+v", sum.Flows[0])
	}
}

func TestMissingFragmentGaps(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	f2 := buildFragment(5, 2, udp[16:]) // first fragment missing
	sum, _ := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f2}))
	if sum.Complete != 0 {
		t.Fatalf("must not be complete")
	}
	if len(sum.Anomalies) != 1 || sum.Anomalies[0].Kind != "incomplete" ||
		!strings.Contains(sum.Anomalies[0].Detail, "{0 16}") {
		t.Fatalf("expected gap [0,16): %+v", sum.Anomalies)
	}
}

func TestLastFragmentMissing(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	f1 := buildFragment(5, mf|0, udp[:16])
	sum, _ := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f1}))
	if len(sum.Anomalies) != 1 || sum.Anomalies[0].Kind != "last_fragment_missing" {
		t.Fatalf("expected last_fragment_missing: %+v", sum.Anomalies)
	}
}

func TestTruncatedRecord(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	frame := buildFragment(1, 0, udp)
	// record header claims more on-wire bytes than captured
	var buf bytes.Buffer
	o := binary.LittleEndian
	binary.Write(&buf, o, uint32(0xa1b2c3d4))
	binary.Write(&buf, o, uint16(2))
	binary.Write(&buf, o, uint16(4))
	binary.Write(&buf, o, int32(0))
	binary.Write(&buf, o, uint32(0))
	binary.Write(&buf, o, uint32(65535))
	binary.Write(&buf, o, uint32(1))
	binary.Write(&buf, o, uint32(1))
	binary.Write(&buf, o, uint32(0))
	binary.Write(&buf, o, uint32(len(frame)))
	binary.Write(&buf, o, uint32(len(frame)+100)) // origlen > caplen
	buf.Write(frame)
	sum, dgs := runSummary(t, buf.Bytes())
	if sum.TruncatedRecords != 1 {
		t.Fatalf("expected truncated record counted: %+v", sum)
	}
	if sum.Complete != 1 || len(dgs) != 1 {
		t.Fatalf("fully captured packet should still decode: %+v", sum)
	}
}

func TestInvalidChecksumLocated(t *testing.T) {
	udp := buildUDPPayload(1, 2, []byte("ABCDEFGH"))
	frame := buildFragment(1, 0, udp)
	frame[14+10] ^= 0xff // corrupt IP header checksum field
	sum, _ := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{frame}))
	if sum.InvalidPackets != 1 || len(sum.Anomalies) != 1 ||
		sum.Anomalies[0].Record != 1 || !strings.Contains(sum.Anomalies[0].Detail, "checksum") {
		t.Fatalf("expected invalid packet at record 1: %+v", sum)
	}
}

func TestNonUDPSkipped(t *testing.T) {
	frame := buildFragment(1, 0, []byte("tcp-ish"))
	frame[14+9] = 6 // protocol TCP
	binary.BigEndian.PutUint16(frame[14+10:14+12], 0)
	binary.BigEndian.PutUint16(frame[14+10:14+12], checksum(frame[14:34]))
	sum, _ := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{frame}))
	if sum.NonUDP != 1 || sum.Complete != 0 {
		t.Fatalf("expected non-udp counted and skipped: %+v", sum)
	}
}

func TestNonMultipleOf8NonFinal(t *testing.T) {
	f := buildFragment(3, mf|0, make([]byte, 10))
	sum, _ := runSummary(t, buildPCAP(binary.LittleEndian, false, [][]byte{f}))
	if len(sum.Anomalies) != 1 || !strings.Contains(sum.Anomalies[0].Detail, "multiple of 8") {
		t.Fatalf("expected multiple-of-8 violation: %+v", sum.Anomalies)
	}
}
