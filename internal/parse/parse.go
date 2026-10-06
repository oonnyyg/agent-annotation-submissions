// Package parse decodes Ethernet / IPv4 / UDP headers from captured bytes.
package parse

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	etherTypeIPv4 = 0x0800
	protoUDP      = 17
)

// Packet is a decoded IPv4 UDP datagram (possibly a fragment).
type Packet struct {
	SrcIP     [4]byte
	DstIP     [4]byte
	Ident     uint16 // IPv4 identification
	FragOff   int    // fragment offset in bytes
	MoreFrag  bool
	UDPSrc    uint16 // valid only when FragOff == 0
	UDPDst    uint16
	UDPLen    int    // UDP length field, valid only when FragOff == 0
	UDPCsum   uint16 // UDP checksum, valid only when FragOff == 0
	Payload   []byte // UDP datagram bytes carried by this fragment (header included when FragOff == 0)
	Truncated bool   // capture cut off before the IPv4 total length
}

// ErrNotUDP marks packets that are not IPv4-over-Ethernet UDP.
var ErrNotUDP = errors.New("not an IPv4 UDP packet")

// Decode parses one captured frame.
func Decode(frame []byte) (*Packet, error) {
	if len(frame) < 14 {
		return nil, fmt.Errorf("ethernet: frame too short (%d bytes)", len(frame))
	}
	if binary.BigEndian.Uint16(frame[12:14]) != etherTypeIPv4 {
		return nil, ErrNotUDP
	}
	ip := frame[14:]
	if len(ip) < 20 {
		return nil, fmt.Errorf("ipv4: header too short (%d bytes)", len(ip))
	}
	if ip[0]>>4 != 4 {
		return nil, ErrNotUDP
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl {
		return nil, fmt.Errorf("ipv4: invalid header length %d", ihl)
	}
	if csum := headerChecksum(ip[:ihl]); csum != 0 {
		return nil, fmt.Errorf("ipv4: bad header checksum (0x%04x)", csum)
	}
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	if total < ihl {
		return nil, fmt.Errorf("ipv4: total length %d smaller than header %d", total, ihl)
	}
	p := &Packet{Truncated: len(ip) < total}
	if len(ip) > total {
		ip = ip[:total]
	}
	if ip[9] != protoUDP {
		return nil, ErrNotUDP
	}
	frag := binary.BigEndian.Uint16(ip[6:8])
	p.MoreFrag = frag&0x2000 != 0
	p.FragOff = int(frag&0x1fff) * 8
	p.Ident = binary.BigEndian.Uint16(ip[4:6])
	copy(p.SrcIP[:], ip[12:16])
	copy(p.DstIP[:], ip[16:20])
	p.Payload = ip[ihl:]
	if p.FragOff == 0 && len(p.Payload) >= 8 {
		p.UDPSrc = binary.BigEndian.Uint16(p.Payload[0:2])
		p.UDPDst = binary.BigEndian.Uint16(p.Payload[2:4])
		p.UDPLen = int(binary.BigEndian.Uint16(p.Payload[4:6]))
		p.UDPCsum = binary.BigEndian.Uint16(p.Payload[6:8])
	}
	return p, nil
}

// headerChecksum computes the one's-complement checksum over data;
// a valid header (checksum field included) sums to zero.
func headerChecksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// UDPChecksum verifies the UDP checksum over the reassembled datagram.
// A zero checksum field (checksum disabled) is accepted.
func UDPChecksum(src, dst [4]byte, udp []byte) bool {
	if len(udp) < 8 {
		return false
	}
	if binary.BigEndian.Uint16(udp[6:8]) == 0 {
		return true
	}
	var sum uint32
	add := func(b []byte) {
		for len(b) >= 2 {
			sum += uint32(binary.BigEndian.Uint16(b))
			b = b[2:]
		}
		if len(b) == 1 {
			sum += uint32(b[0]) << 8
		}
	}
	add(src[:])
	add(dst[:])
	sum += uint32(protoUDP) + uint32(len(udp))
	add(udp)
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum) == 0
}
