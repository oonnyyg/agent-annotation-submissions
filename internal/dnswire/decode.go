package dnswire

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"unicode/utf8"
)

const (
	maxNameLen  = 255
	maxLabelLen = 63
	maxPtr      = 0x3FFF
)

// DecodeMessage parses a raw DNS message into the JSON-friendly model.
func DecodeMessage(msg []byte) (*Message, error) {
	if len(msg) < 12 {
		return nil, errAt(len(msg), "message of %d bytes is shorter than the 12-byte header", len(msg))
	}
	h := Header{
		ID:     binary.BigEndian.Uint16(msg[0:2]),
		QR:     msg[2]&0x80 != 0,
		Opcode: (msg[2] >> 3) & 0x0F,
		AA:     msg[2]&0x04 != 0,
		TC:     msg[2]&0x02 != 0,
		RD:     msg[2]&0x01 != 0,
		RA:     msg[3]&0x80 != 0,
		Z:      (msg[3] >> 4) & 0x07,
		Rcode:  msg[3] & 0x0F,
	}
	m := &Message{Header: h}
	counts := []struct {
		n    int
		kind string
	}{
		{int(binary.BigEndian.Uint16(msg[4:6])), "question"},
		{int(binary.BigEndian.Uint16(msg[6:8])), "answer"},
		{int(binary.BigEndian.Uint16(msg[8:10])), "authority"},
		{int(binary.BigEndian.Uint16(msg[10:12])), "additional"},
	}
	off := 12
	for _, c := range counts {
		for i := 0; i < c.n; i++ {
			var err error
			if c.kind == "question" {
				var q Question
				q, off, err = decodeQuestion(msg, off)
				if err != nil {
					return nil, err
				}
				m.Questions = append(m.Questions, q)
			} else {
				var r Record
				r, off, err = decodeRecord(msg, off)
				if err != nil {
					return nil, err
				}
				switch c.kind {
				case "answer":
					m.Answers = append(m.Answers, r)
				case "authority":
					m.Authorities = append(m.Authorities, r)
				default:
					m.Additionals = append(m.Additionals, r)
				}
			}
		}
	}
	if off != len(msg) {
		return nil, errAt(off, "%d trailing bytes after declared sections", len(msg)-off)
	}
	if m.Questions == nil {
		m.Questions = []Question{}
	}
	if m.Answers == nil {
		m.Answers = []Record{}
	}
	if m.Authorities == nil {
		m.Authorities = []Record{}
	}
	if m.Additionals == nil {
		m.Additionals = []Record{}
	}
	return m, nil
}

func decodeQuestion(msg []byte, off int) (Question, int, error) {
	name, n, err := decodeName(msg, off)
	if err != nil {
		return Question{}, 0, err
	}
	off += n
	if off+4 > len(msg) {
		return Question{}, 0, errAt(off, "question is truncated")
	}
	qtype := binary.BigEndian.Uint16(msg[off : off+2])
	qclass := binary.BigEndian.Uint16(msg[off+2 : off+4])
	return Question{Name: name, Type: typeName(qtype), Class: className(qclass)}, off + 4, nil
}

func decodeRecord(msg []byte, off int) (Record, int, error) {
	name, n, err := decodeName(msg, off)
	if err != nil {
		return Record{}, 0, err
	}
	off += n
	if off+10 > len(msg) {
		return Record{}, 0, errAt(off, "resource record header is truncated")
	}
	rtype := binary.BigEndian.Uint16(msg[off : off+2])
	rclass := binary.BigEndian.Uint16(msg[off+2 : off+4])
	ttl := binary.BigEndian.Uint32(msg[off+4 : off+8])
	rdlen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
	off += 10
	if off+rdlen > len(msg) {
		return Record{}, 0, errAt(off, "rdata of %d bytes extends past end of message", rdlen)
	}
	rec := Record{Name: name, Type: typeName(rtype), Class: className(rclass), TTL: ttl}
	end := off + rdlen
	if rclass != classIN || !knownType(rtype) {
		rec.RData = RData{Hex: hex.EncodeToString(msg[off:end])}
		return rec, end, nil
	}
	rd, err := decodeRData(msg, off, end, rtype)
	if err != nil {
		return Record{}, 0, err
	}
	rec.RData = rd
	return rec, end, nil
}

// decodeRData parses rdata within [off, end); every consumed byte must stay
// inside the declared boundary.
func decodeRData(msg []byte, off, end int, rtype uint16) (RData, error) {
	switch rtype {
	case 1: // A
		if end-off != 4 {
			return RData{}, errAt(off, "A rdata must be exactly 4 bytes, got %d", end-off)
		}
		return RData{A: net.IP(msg[off:end]).String()}, nil
	case 5: // CNAME
		name, n, err := decodeName(msg, off)
		if err != nil {
			return RData{}, err
		}
		if off+n != end {
			return RData{}, errAt(off, "CNAME rdata does not fill its declared %d-byte boundary", end-off)
		}
		return RData{CNAME: name}, nil
	case 15: // MX
		if off+2 > end {
			return RData{}, errAt(off, "MX preference is truncated")
		}
		pref := binary.BigEndian.Uint16(msg[off : off+2])
		name, n, err := decodeName(msg, off+2)
		if err != nil {
			return RData{}, err
		}
		if off+2+n != end {
			return RData{}, errAt(off, "MX rdata does not fill its declared %d-byte boundary", end-off)
		}
		return RData{MX: &MXData{Preference: pref, Exchange: name}}, nil
	case 16: // TXT
		var txts []string
		for p := off; p < end; {
			l := int(msg[p])
			if p+1+l > end {
				return RData{}, errAt(p, "TXT string of %d bytes crosses the rdata boundary", l)
			}
			txts = append(txts, txtString(msg[p+1:p+1+l]))
			p += 1 + l
		}
		return RData{TXT: txts}, nil
	}
	return RData{}, errAt(off, "unsupported type %d", rtype)
}

// txtString renders TXT bytes as a JSON-safe string; non-UTF-8 data is hex.
func txtString(b []byte) string {
	s := string(b)
	if utf8.ValidString(s) {
		return s
	}
	return "hex:" + hex.EncodeToString(b)
}

func typeName(code uint16) string {
	if s, ok := codeTypes[code]; ok {
		return s
	}
	return fmt.Sprintf("TYPE%d", code)
}

func className(code uint16) string {
	if code == classIN {
		return "IN"
	}
	return fmt.Sprintf("CLASS%d", code)
}

func knownType(code uint16) bool {
	_, ok := codeTypes[code]
	return ok
}

// decodeName reads a possibly-compressed name starting at offset.
// It returns the labels and the number of bytes the name occupies at its
// original position (i.e. up to and including the first pointer or the
// terminating zero), so the caller can find the next field correctly.
func decodeName(msg []byte, offset int) (Name, int, error) {
	labels, next, err := decodeNameAt(msg, offset, map[int]bool{})
	if err != nil {
		return nil, 0, err
	}
	return labels, next - offset, nil
}

// decodeNameAt returns the labels and the offset just past the name as seen
// from the position where decoding started (before any pointer jump).
func decodeNameAt(msg []byte, offset int, seen map[int]bool) (Name, int, error) {
	labels := Name{}
	next := -1
	for {
		if offset >= len(msg) {
			return nil, 0, errAt(offset, "name is truncated: label length byte outside message")
		}
		b := msg[offset]
		switch b & 0xC0 {
		case 0x00:
			if b == 0 {
				next = offset + 1
				if wireNameLen(labels) > maxNameLen {
					return nil, 0, errAt(offset, "expanded name exceeds %d bytes", maxNameLen)
				}
				return labels, next, nil
			}
			l := int(b)
			if l > maxLabelLen {
				return nil, 0, errAt(offset, "label length %d exceeds %d", l, maxLabelLen)
			}
			if offset+1+l > len(msg) {
				return nil, 0, errAt(offset, "label of %d bytes is truncated", l)
			}
			label := msg[offset+1 : offset+1+l]
			if !isASCII(label) {
				return nil, 0, errAt(offset+1, "label contains non-ASCII bytes")
			}
			labels = append(labels, string(label))
			next = offset + 1 + l
			offset += 1 + l
		case 0xC0:
			if offset+1 >= len(msg) {
				return nil, 0, errAt(offset, "compression pointer is truncated")
			}
			ptr := int(binary.BigEndian.Uint16(msg[offset:offset+2]) & 0x3FFF)
			if ptr < 12 {
				return nil, 0, errAt(offset, "compression pointer %d points into the header", ptr)
			}
			if ptr >= len(msg) {
				return nil, 0, errAt(offset, "compression pointer %d outside message of %d bytes", ptr, len(msg))
			}
			if seen[ptr] {
				return nil, 0, errAt(offset, "compression pointer loop at offset %d", ptr)
			}
			seen[ptr] = true
			next = offset + 2
			tail, _, err := decodeNameAt(msg, ptr, seen)
			if err != nil {
				return nil, 0, err
			}
			labels = append(labels, tail...)
			if wireNameLen(labels) > maxNameLen {
				return nil, 0, errAt(offset, "expanded name exceeds %d bytes", maxNameLen)
			}
			return labels, next, nil
		default:
			return nil, 0, errAt(offset, "reserved label type 0x%02x", b&0xC0)
		}
	}
}

// wireNameLen is the length of the name in uncompressed wire form.
func wireNameLen(n Name) int {
	total := 1
	for _, l := range n {
		total += 1 + len(l)
	}
	return total
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c > 0x7F {
			return false
		}
	}
	return true
}
