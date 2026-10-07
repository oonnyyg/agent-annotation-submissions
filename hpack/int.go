package hpack

import "fmt"

// appendInt encodes i using the HPACK integer representation with the
// given prefix width. The representation tag bits already computed by
// the caller are ORed into the first byte.
func appendInt(dst []byte, i uint64, prefixBits uint, tag byte) []byte {
	max := uint64(1<<prefixBits) - 1
	if i < max {
		return append(dst, tag|byte(i))
	}
	dst = append(dst, tag|byte(max))
	i -= max
	for i >= 128 {
		dst = append(dst, byte(i&0x7f)|0x80)
		i >>= 7
	}
	return append(dst, byte(i))
}

// readInt decodes an HPACK integer starting at data[off]. The prefix
// value is taken from data[off] masked to prefixBits. It returns the
// value and the offset of the first byte after the integer. A truncated
// integer (continuation bit set but no more bytes) is an error.
func readInt(data []byte, off int, prefixBits uint) (uint64, int, error) {
	if off >= len(data) {
		return 0, off, fmt.Errorf("byte %d: missing integer prefix byte", off)
	}
	max := uint64(1<<prefixBits) - 1
	v := uint64(data[off]) & max
	off++
	if v < max {
		return v, off, nil
	}
	shift := uint(0)
	for {
		if off >= len(data) {
			return 0, off, fmt.Errorf("byte %d: truncated integer continuation", off)
		}
		b := data[off]
		off++
		if shift >= 63 && b > 1 {
			return 0, off, fmt.Errorf("byte %d: integer overflow", off-1)
		}
		v += uint64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			return v, off, nil
		}
	}
}
