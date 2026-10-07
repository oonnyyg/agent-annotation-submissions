package dnswire

import (
	"encoding/binary"
	"fmt"
	"net"
)

// EncodeMessage serializes m, generating section counts and RDATA lengths
// from the actual data. Names are compressed against previously encoded
// suffixes. Unknown record types (Raw set) are rejected.
func EncodeMessage(m *Message) ([]byte, error) {
	buf := make([]byte, headerLen)
	binary.BigEndian.PutUint16(buf[0:2], m.Header.ID)
	var flags uint16
	if m.Header.QR {
		flags |= 0x8000
	}
	flags |= uint16(m.Header.Opcode&0xF) << 11
	if m.Header.AA {
		flags |= 0x0400
	}
	if m.Header.TC {
		flags |= 0x0200
	}
	if m.Header.RD {
		flags |= 0x0100
	}
	if m.Header.RA {
		flags |= 0x0080
	}
	flags |= uint16(m.Header.Z&0x7) << 4
	flags |= uint16(m.Header.RCode & 0xF)
	binary.BigEndian.PutUint16(buf[2:4], flags)

	sections := [][]Record{m.Answers, m.Authority, m.Additional}
	counts := []int{len(m.Questions), len(m.Answers), len(m.Authority), len(m.Additional)}
	for i, c := range counts {
		if c > 0xFFFF {
			return nil, errAt(0, "section %d count %d exceeds 65535", i, c)
		}
		binary.BigEndian.PutUint16(buf[4+2*i:6+2*i], uint16(c))
	}

	enc := newNameEncoder()
	var err error
	for i := range m.Questions {
		q := &m.Questions[i]
		if buf, err = enc.appendName(buf, q.Name.Labels); err != nil {
			return nil, err
		}
		buf = binary.BigEndian.AppendUint16(buf, q.QType)
		buf = binary.BigEndian.AppendUint16(buf, q.QClass)
	}
	for s, sec := range sections {
		for i := range sec {
			r := &sec[i]
			if buf, err = enc.appendName(buf, r.Name.Labels); err != nil {
				return nil, err
			}
			buf = binary.BigEndian.AppendUint16(buf, r.Type)
			buf = binary.BigEndian.AppendUint16(buf, r.Class)
			buf = binary.BigEndian.AppendUint32(buf, r.TTL)
			rdlenAt := len(buf)
			buf = append(buf, 0, 0)
			start := len(buf)
			if buf, err = encodeRData(enc, buf, r); err != nil {
				return nil, fmt.Errorf("section %d record %d: %w", s, i, err)
			}
			rdlen := len(buf) - start
			if rdlen > 0xFFFF {
				return nil, errAt(start, "RDATA of %d bytes exceeds 65535", rdlen)
			}
			binary.BigEndian.PutUint16(buf[rdlenAt:], uint16(rdlen))
		}
	}
	return buf, nil
}

func encodeRData(enc *nameEncoder, buf []byte, r *Record) ([]byte, error) {
	if r.Raw != "" {
		return nil, errAt(0, "unknown record type %d cannot be re-encoded from raw data", r.Type)
	}
	if r.Class != 1 {
		return nil, errAt(0, "only IN class is supported, got class %d", r.Class)
	}
	switch r.Type {
	case 1:
		s, ok := r.Data.(string)
		if !ok {
			return nil, errAt(0, "A data must be a dotted IPv4 string")
		}
		ip := net.ParseIP(s).To4()
		if ip == nil {
			return nil, errAt(0, "invalid IPv4 address %q", s)
		}
		return append(buf, ip...), nil
	case 5:
		nr, ok := r.Data.(NameResult)
		if !ok {
			return nil, errAt(0, "CNAME data must be a name")
		}
		return enc.appendName(buf, nr.Labels)
	case 15:
		mx, ok := r.Data.(MXData)
		if !ok {
			return nil, errAt(0, "MX data must be preference plus exchange")
		}
		buf = binary.BigEndian.AppendUint16(buf, mx.Preference)
		return enc.appendName(buf, mx.Exchange.Labels)
	case 16:
		txts, ok := r.Data.([]string)
		if !ok {
			return nil, errAt(0, "TXT data must be a list of strings")
		}
		for _, t := range txts {
			if len(t) > 255 {
				return nil, errAt(0, "TXT string of %d bytes exceeds 255", len(t))
			}
			buf = append(buf, byte(len(t)))
			buf = append(buf, t...)
		}
		return buf, nil
	default:
		return nil, errAt(0, "unsupported record type %d", r.Type)
	}
}
