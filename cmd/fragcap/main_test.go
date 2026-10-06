package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"example.com/pcap-fragment-audit/internal/pcap"
	"example.com/pcap-fragment-audit/internal/testutil"
)

var (
	src = [4]byte{10, 1, 0, 1}
	dst = [4]byte{10, 1, 0, 2}
)

// buildPcap renders a little-endian microsecond pcap with the given frames.
// inclLen[i] > 0 truncates record i to that many captured bytes.
func buildPcap(frames [][]byte, inclLen []int) []byte {
	var buf bytes.Buffer
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint32(hdr[0:4], 0xd4c3b2a1)
	binary.LittleEndian.PutUint16(hdr[4:6], 2)
	binary.LittleEndian.PutUint16(hdr[6:8], 4)
	binary.LittleEndian.PutUint32(hdr[20:24], 1)
	buf.Write(hdr)
	for i, fr := range frames {
		incl := len(fr)
		if inclLen != nil && inclLen[i] > 0 {
			incl = inclLen[i]
		}
		rh := make([]byte, 16)
		binary.LittleEndian.PutUint32(rh[0:4], uint32(1700000000+i))
		binary.LittleEndian.PutUint32(rh[8:12], uint32(incl))
		binary.LittleEndian.PutUint32(rh[12:16], uint32(len(fr)))
		buf.Write(rh)
		buf.Write(fr[:incl])
	}
	return buf.Bytes()
}

func udpFull(payload string) []byte {
	f := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, UDPSrc: 5000, UDPDst: 5001, Payload: []byte(payload),
	})
	return f[34:]
}

func TestEndToEnd(t *testing.T) {
	full := udpFull("0123456789abcdef") // 24-byte UDP datagram
	complete := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 1, UDPSrc: 5000, UDPDst: 5001, Payload: []byte("solo"),
	})
	frag0 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 2, MoreFrag: true, Raw: full[:16],
	})
	frag1 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 2, FragOff: 16, Raw: full[16:],
	})
	// group 3: conflicting overlap
	bad := append([]byte(nil), full[8:24]...)
	bad[0] ^= 0xff
	cf0 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 3, MoreFrag: true, Raw: full[:16],
	})
	cf1 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 3, FragOff: 8, MoreFrag: true, Raw: bad,
	})
	cf2 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 3, FragOff: 16, Raw: full[16:],
	})
	// group 4: missing middle fragment
	mf0 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 4, MoreFrag: true, Raw: full[:8],
	})
	mf1 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 4, FragOff: 16, Raw: full[16:],
	})
	// non-UDP frame (ARP ethertype)
	arp := make([]byte, 42)
	arp[12], arp[13] = 0x08, 0x06

	data := buildPcap([][]byte{complete, frag1, frag0, cf0, cf1, cf2, mf0, mf1, arp}, nil)
	rd, err := pcap.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	datagrams, sum := analyze(rd)

	if len(datagrams) != 2 {
		t.Fatalf("expected 2 complete datagrams, got %d: %+v", len(datagrams), datagrams)
	}
	if sum.Complete != 2 || sum.NonUDPRecords != 1 {
		t.Fatalf("bad summary counts: %+v", sum)
	}
	var payloadBytes int
	for _, f := range sum.Flows {
		payloadBytes += f.PayloadBytes
	}
	if payloadBytes != 4+16 {
		t.Fatalf("payload bytes = %d, want 20", payloadBytes)
	}
	kinds := map[string]int{}
	for _, a := range sum.Anomalies {
		kinds[a.Kind]++
	}
	if kinds["conflict"] != 1 || kinds["missing_fragments"] != 1 {
		t.Fatalf("unexpected anomalies: %+v", sum.Anomalies)
	}
}

func TestTruncatedRecordIsAnomaly(t *testing.T) {
	full := udpFull("0123456789abcdef")
	frag0 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 5, MoreFrag: true, Raw: full[:16],
	})
	frag1 := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 5, FragOff: 16, Raw: full[16:],
	})
	// capture cuts frag1 short
	data := buildPcap([][]byte{frag0, frag1}, []int{0, len(frag1) - 4})
	rd, err := pcap.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	datagrams, sum := analyze(rd)
	if len(datagrams) != 0 {
		t.Fatalf("truncated group must not complete: %+v", datagrams)
	}
	if sum.TruncCaptures != 1 {
		t.Fatalf("expected 1 truncated capture, got %d", sum.TruncCaptures)
	}
	found := false
	for _, a := range sum.Anomalies {
		if a.Kind == "truncated_group" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected truncated_group anomaly: %+v", sum.Anomalies)
	}
}

func TestUDPLengthMismatch(t *testing.T) {
	// UDP length field disagrees with the reassembled length
	bad := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 6, UDPSrc: 1, UDPDst: 2,
		Payload: []byte("abcd"), UDPLen: 99,
	})
	data := buildPcap([][]byte{bad}, nil)
	rd, err := pcap.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	datagrams, sum := analyze(rd)
	if len(datagrams) != 0 {
		t.Fatal("length-mismatched datagram must not be emitted")
	}
	found := false
	for _, a := range sum.Anomalies {
		if a.Kind == "invalid_udp" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid_udp anomaly: %+v", sum.Anomalies)
	}
}
