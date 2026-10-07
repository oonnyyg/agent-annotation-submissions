package hpack

import (
	"unicode/utf8"
)

// decodeInteger reads an HPACK integer (RFC 7541 §5.1) from buf starting at
// offset. prefixBits is the number of low bits of the first byte used for
// the integer. It returns the decoded value and the offset just past it.
func decodeInteger(buf []byte, offset, prefixBits int) (int, int, error) {
	if offset >= len(buf) {
		return 0, offset, &offsetError{offset: offset, err: ErrIntegerTruncated}
	}
	mask := (1 << prefixBits) - 1
	value := int(buf[offset]) & mask
	pos := offset + 1
	if value < mask {
		return value, pos, nil
	}
	var shift uint
	for {
		if pos >= len(buf) {
			return 0, pos, &offsetError{offset: pos, err: ErrIntegerTruncated}
		}
		b := buf[pos]
		pos++
		value += int(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return value, pos, nil
}

// encodeInteger appends an HPACK integer to dst. prefix is the already-set
// high bits (including the representation marker), and prefixBits is the
// width of the integer field in the first byte.
func encodeInteger(dst []byte, prefix byte, value, prefixBits int) []byte {
	mask := (1 << prefixBits) - 1
	if value < mask {
		return append(dst, prefix|byte(value))
	}
	dst = append(dst, prefix|byte(mask))
	value -= mask
	for value >= 128 {
		dst = append(dst, byte(value%128+128))
		value /= 128
	}
	dst = append(dst, byte(value))
	return dst
}

// decodeString reads an HPACK string literal (RFC 7541 §5.2). Huffman
// encoded strings are explicitly rejected; raw strings must be valid UTF-8.
func decodeString(buf []byte, offset int) (string, int, error) {
	if offset >= len(buf) {
		return "", offset, &offsetError{offset: offset, err: ErrStringTruncated}
	}
	huffman := buf[offset]&0x80 != 0
	length, pos, err := decodeInteger(buf, offset, 7)
	if err != nil {
		return "", pos, err
	}
	if huffman {
		// Skip the declared payload for accurate error location, then reject.
		end := pos + length
		if end > len(buf) {
			end = len(buf)
		}
		return "", pos, &offsetError{offset: offset, err: ErrHuffmanUnsupported}
	}
	end := pos + length
	if end > len(buf) {
		return "", len(buf), &offsetError{offset: pos, err: ErrStringTruncated}
	}
	raw := buf[pos:end]
	if !utf8.Valid(raw) {
		return "", pos, &offsetError{offset: pos, err: ErrInvalidUTF8}
	}
	return string(raw), end, nil
}

// encodeString appends a raw (never Huffman) HPACK string literal.
func encodeString(dst []byte, s string) []byte {
	dst = encodeInteger(dst, 0x00, len(s), 7)
	dst = append(dst, s...)
	return dst
}
