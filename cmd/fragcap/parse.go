package main

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	etherTypeIPv4 = 0x0800
	protoUDP      = 17
)

// ipPacket is a parsed Ethernet-carried IPv4 packet.
type ipPacket struct {
	Src, Dst   netip.Addr
	Protocol   uint8
	ID         uint16
	MoreFrags  bool
	FragOffset int // payload offset in bytes (field value * 8)
	Payload    []byte
}

// parseEthernetIPv4 parses one captured frame. It returns (nil, nil) for
// frames that are not Ethernet/IPv4 so callers can count and skip them.
func parseEthernetIPv4(frame []byte) (*ipPacket, error) {
	if len(frame) < 14 {
		return nil, fmt.Errorf("ethernet frame too short: %d bytes", len(frame))
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	if etherType != etherTypeIPv4 {
		return nil, nil // non-IPv4: caller counts and skips
	}
	ip := frame[14:]
	if len(ip) < 20 {
		return nil, fmt.Errorf("ipv4 header truncated: %d bytes", len(ip))
	}
	version := ip[0] >> 4
	ihl := int(ip[0]&0x0f) * 4
	if version != 4 {
		return nil, fmt.Errorf("unexpected ip version %d", version)
	}
	if ihl < 20 {
		return nil, fmt.Errorf("invalid ipv4 header length %d", ihl)
	}
	if len(ip) < ihl {
		return nil, fmt.Errorf("ipv4 header (ihl=%d) truncated: %d bytes", ihl, len(ip))
	}
	totalLen := int(binary.BigEndian.Uint16(ip[2:4]))
	if totalLen < ihl {
		return nil, fmt.Errorf("ipv4 total length %d smaller than header length %d", totalLen, ihl)
	}
	if len(ip) < totalLen {
		return nil, fmt.Errorf("ipv4 packet truncated: total length %d, captured %d", totalLen, len(ip))
	}
	if csum := checksum(ip[:ihl]); csum != 0 {
		return nil, fmt.Errorf("invalid ipv4 header checksum (got %#x)", csum)
	}
	fragField := binary.BigEndian.Uint16(ip[6:8])
	pkt := &ipPacket{
		Protocol:   ip[9],
		ID:         binary.BigEndian.Uint16(ip[4:6]),
		MoreFrags:  fragField&0x2000 != 0,
		FragOffset: int(fragField&0x1fff) * 8,
		Payload:    ip[ihl:totalLen],
	}
	var ok bool
	pkt.Src, ok = netip.AddrFromSlice(ip[12:16])
	if !ok {
		return nil, fmt.Errorf("invalid source address")
	}
	pkt.Dst, ok = netip.AddrFromSlice(ip[16:20])
	if !ok {
		return nil, fmt.Errorf("invalid destination address")
	}
	return pkt, nil
}

// checksum computes the Internet checksum over data; a valid header sums to 0.
func checksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data[:2]))
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

// udpDatagram is a decoded UDP datagram from a reassembled payload.
type udpDatagram struct {
	SrcPort, DstPort uint16
	Payload          []byte
}

// parseUDP validates and decodes a UDP datagram from a reassembled payload.
// src/dst are needed for the checksum pseudo-header.
func parseUDP(payload []byte, src, dst netip.Addr) (*udpDatagram, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("udp header truncated: %d bytes", len(payload))
	}
	udpLen := int(binary.BigEndian.Uint16(payload[4:6]))
	if udpLen < 8 {
		return nil, fmt.Errorf("invalid udp length %d", udpLen)
	}
	if udpLen != len(payload) {
		return nil, fmt.Errorf("udp length %d does not match reassembled payload length %d", udpLen, len(payload))
	}
	csum := binary.BigEndian.Uint16(payload[6:8])
	if csum != 0 {
		if udpChecksum(src, dst, payload) != 0 {
			return nil, fmt.Errorf("invalid udp checksum")
		}
	}
	return &udpDatagram{
		SrcPort: binary.BigEndian.Uint16(payload[0:2]),
		DstPort: binary.BigEndian.Uint16(payload[2:4]),
		Payload: payload[8:],
	}, nil
}

func udpChecksum(src, dst netip.Addr, udp []byte) uint16 {
	var pseudo []byte
	s4 := src.As4()
	d4 := dst.As4()
	pseudo = append(pseudo, s4[:]...)
	pseudo = append(pseudo, d4[:]...)
	pseudo = append(pseudo, 0, protoUDP, byte(len(udp)>>8), byte(len(udp)))
	sumData := append(pseudo, udp...)
	if len(sumData)%2 == 1 {
		sumData = append(sumData, 0)
	}
	return checksum(sumData)
}
