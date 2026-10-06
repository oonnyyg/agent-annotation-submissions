package parse

import (
	"testing"

	"example.com/pcap-fragment-audit/internal/testutil"
)

var (
	src = [4]byte{10, 0, 0, 1}
	dst = [4]byte{10, 0, 0, 2}
)

func TestDecodeUnfragmented(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 7, UDPSrc: 1234, UDPDst: 53,
		Payload: []byte("hello"),
	})
	p, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ident != 7 || p.FragOff != 0 || p.MoreFrag {
		t.Fatalf("bad packet: %+v", p)
	}
	if p.UDPSrc != 1234 || p.UDPDst != 53 || p.UDPLen != 13 {
		t.Fatalf("bad udp fields: %+v", p)
	}
	if !UDPChecksum(p.SrcIP, p.DstIP, p.Payload) {
		t.Fatal("checksum should verify")
	}
}

func TestDecodeFragment(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 9, FragOff: 16, MoreFrag: true,
		Payload: []byte("12345678"),
	})
	p, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if p.FragOff != 16 || !p.MoreFrag {
		t.Fatalf("bad fragment fields: %+v", p)
	}
}

func TestBadIPChecksum(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 1, UDPSrc: 1, UDPDst: 2, Payload: []byte("x"),
	})
	frame[14+10] ^= 0xff // corrupt IP header checksum
	if _, err := Decode(frame); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestNotUDP(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 1, UDPSrc: 1, UDPDst: 2, Payload: []byte("x"),
	})
	frame[14+9] = 6 // TCP
	// fix IP checksum after the change
	frame[14+10], frame[14+11] = 0, 0
	var sum uint32
	for i := 14; i < 34; i += 2 {
		sum += uint32(frame[i])<<8 | uint32(frame[i+1])
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	c := ^uint16(sum)
	frame[14+10], frame[14+11] = byte(c>>8), byte(c)
	if _, err := Decode(frame); err != ErrNotUDP {
		t.Fatalf("want ErrNotUDP, got %v", err)
	}
	frame[12], frame[13] = 0x86, 0xdd // IPv6 ethertype
	if _, err := Decode(frame); err != ErrNotUDP {
		t.Fatalf("want ErrNotUDP for non-IPv4, got %v", err)
	}
}

func TestTruncatedCapture(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 1, UDPSrc: 1, UDPDst: 2,
		Payload: []byte("0123456789"),
	})
	p, err := Decode(frame[:len(frame)-4])
	if err != nil {
		t.Fatal(err)
	}
	if !p.Truncated {
		t.Fatal("expected Truncated=true")
	}
}

func TestBadUDPChecksum(t *testing.T) {
	frame := testutil.BuildFragment(testutil.FragmentParams{
		Src: src, Dst: dst, Ident: 1, UDPSrc: 1, UDPDst: 2,
		Payload: []byte("data"), BadCsum: true,
	})
	p, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if UDPChecksum(p.SrcIP, p.DstIP, p.Payload) {
		t.Fatal("corrupt checksum must not verify")
	}
}
