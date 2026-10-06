// Package testutil builds synthetic Ethernet/IPv4/UDP frames for tests.
package testutil

import "encoding/binary"

// FragmentParams describes one IPv4 fragment carrying UDP bytes.
type FragmentParams struct {
	Src, Dst [4]byte
	Ident    uint16
	FragOff  int // in bytes (must be a multiple of 8)
	MoreFrag bool
	UDPSrc   uint16 // used to build a UDP header when FragOff == 0
	UDPDst   uint16
	Payload  []byte // UDP payload (header is prepended when FragOff == 0)
	UDPLen   int    // override UDP length field; 0 = computed
	BadCsum  bool   // corrupt the UDP checksum
	// Raw, when non-nil, is used verbatim as the UDP bytes of this
	// fragment (including the UDP header when FragOff == 0); the
	// UDPSrc/UDPDst/UDPLen/BadCsum fields are then ignored.
	Raw []byte
}

// udpChecksum computes the UDP checksum with the IPv4 pseudo-header.
func udpChecksum(src, dst [4]byte, udp []byte) uint16 {
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
	sum += uint32(17) + uint32(len(udp))
	add(udp)
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	c := ^uint16(sum)
	if c == 0 {
		c = 0xffff
	}
	return c
}

// BuildFragment renders one Ethernet frame containing an IPv4 UDP fragment.
func BuildFragment(p FragmentParams) []byte {
	var udp []byte
	if p.Raw != nil {
		udp = p.Raw
	} else if p.FragOff == 0 {
		udpLen := p.UDPLen
		if udpLen == 0 {
			udpLen = 8 + len(p.Payload)
		}
		udp = make([]byte, 8)
		binary.BigEndian.PutUint16(udp[0:2], p.UDPSrc)
		binary.BigEndian.PutUint16(udp[2:4], p.UDPDst)
		binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen))
		udp = append(udp, p.Payload...)
		csum := udpChecksum(p.Src, p.Dst, udp)
		if p.BadCsum {
			csum ^= 0xffff
		}
		binary.BigEndian.PutUint16(udp[6:8], csum)
	} else {
		udp = p.Payload
	}

	ip := make([]byte, 20)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+len(udp)))
	binary.BigEndian.PutUint16(ip[4:6], p.Ident)
	frag := uint16(p.FragOff / 8)
	if p.MoreFrag {
		frag |= 0x2000
	}
	binary.BigEndian.PutUint16(ip[6:8], frag)
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:16], p.Src[:])
	copy(ip[16:20], p.Dst[:])
	var sum uint32
	for i := 0; i < 20; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ip[i : i+2]))
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	binary.BigEndian.PutUint16(ip[10:12], ^uint16(sum))

	frame := make([]byte, 14, 14+len(ip)+len(udp))
	frame[12] = 0x08
	frame[13] = 0x00
	frame = append(frame, ip...)
	return append(frame, udp...)
}
