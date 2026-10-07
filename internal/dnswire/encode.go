package dnswire

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"unicode/utf8"
)

// encoder accumulates the message and remembers where each name suffix was
// written so later names can reuse it with a compression pointer.
type encoder struct {
	buf     []byte
	offsets map[string]int // suffix key -> offset of its first label
}

// EncodeMessage renders the model into wire format, deriving all counts and
// lengths from the actual data.
func EncodeMessage(m *Message) ([]byte, error) {
	e := &encoder{offsets: map[string]int{}}
	e.buf = make([]byte, 12)
	binary.BigEndian.PutUint16(e.buf[0:2], m.Header.ID)
	if m.Header.Opcode > 0x0F || m.Header.Z > 0x07 || m.Header.Rcode > 0x0F {
		return nil, errAt(2, "opcode, z and rcode must fit in 4/3/4 bits")
	}
	var b2, b3 byte
	if m.Header.QR {
		b2 |= 0x80
	}
	b2 |= (m.Header.Opcode & 0x0F) << 3
	if m.Header.AA {
		b2 |= 0x04
	}
	if m.Header.TC {
		b2 |= 0x02
	}
	if m.Header.RD {
		b2 |= 0x01
	}
	if m.Header.RA {
		b3 |= 0x80
	}
	b3 |= (m.Header.Z & 0x07) << 4
	b3 |= m.Header.Rcode & 0x0F
	e.buf[2], e.buf[3] = b2, b3
	binary.BigEndian.PutUint16(e.buf[4:6], uint16(len(m.Questions)))
	binary.BigEndian.PutUint16(e.buf[6:8], uint16(len(m.Answers)))
	binary.BigEndian.PutUint16(e.buf[8:10], uint16(len(m.Authorities)))
	binary.BigEndian.PutUint16(e.buf[10:12], uint16(len(m.Additionals)))
	for _, q := range m.Questions {
		if err := e.encodeQuestion(q); err != nil {
			return nil, err
		}
	}
	for _, section := range [][]Record{m.Answers, m.Authorities, m.Additionals} {
		for _, r := range section {
			if err := e.encodeRecord(r); err != nil {
				return nil, err
			}
		}
	}
	return e.buf, nil
}

// encodeName writes a name, reusing previously encoded suffixes when the
// pointer offset is representable (<= 0x3FFF).
func (e *encoder) encodeName(name Name) error {
	if err := validateName(name); err != nil {
		return err
	}
	for i := range name {
		key := strings.Join(name[i:], "\x00")
		if off, ok := e.offsets[key]; ok && off <= maxPtr {
			e.buf = append(e.buf, 0xC0|byte(off>>8), byte(off))
			return nil
		}
		if _, ok := e.offsets[key]; !ok {
			e.offsets[key] = len(e.buf)
		}
		e.buf = append(e.buf, byte(len(name[i])))
		e.buf = append(e.buf, name[i]...)
	}
	e.buf = append(e.buf, 0)
	return nil
}

func (e *encoder) encodeQuestion(q Question) error {
	code, err := typeCode(q.Type)
	if err != nil {
		return err
	}
	class, err := classCode(q.Class)
	if err != nil {
		return err
	}
	if err := e.encodeName(q.Name); err != nil {
		return err
	}
	var fixed [4]byte
	binary.BigEndian.PutUint16(fixed[0:2], code)
	binary.BigEndian.PutUint16(fixed[2:4], class)
	e.buf = append(e.buf, fixed[:]...)
	return nil
}

func (e *encoder) encodeRecord(r Record) error {
	code, known := typeCodes[r.Type]
	class, err := classCode(r.Class)
	if err != nil {
		return err
	}
	if !known || class != classIN {
		return errAt(0, "record %q: type %s class %s is not supported for encoding",
			strings.Join(r.Name, "."), r.Type, r.Class)
	}
	if err := e.encodeName(r.Name); err != nil {
		return err
	}
	rdata, err := e.encodeRData(r)
	if err != nil {
		return err
	}
	var fixed [10]byte
	binary.BigEndian.PutUint16(fixed[0:2], code)
	binary.BigEndian.PutUint16(fixed[2:4], class)
	binary.BigEndian.PutUint32(fixed[4:8], r.TTL)
	binary.BigEndian.PutUint16(fixed[8:10], uint16(len(rdata)))
	e.buf = append(e.buf, fixed[:]...)
	e.buf = append(e.buf, rdata...)
	return nil
}

func (e *encoder) encodeRData(r Record) ([]byte, error) {
	switch r.Type {
	case "A":
		ip := net.ParseIP(r.RData.A)
		if ip == nil || ip.To4() == nil {
			return nil, errAt(0, "invalid IPv4 address %q", r.RData.A)
		}
		return ip.To4(), nil
	case "CNAME":
		start := len(e.buf) + 10 // rdata follows the fixed 10-byte header
		return e.encodeNameAt(r.RData.CNAME, start)
	case "MX":
		if r.RData.MX == nil {
			return nil, errAt(0, "MX record requires mx rdata")
		}
		start := len(e.buf) + 10 + 2
		body, err := e.encodeNameAt(r.RData.MX.Exchange, start)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 2, 2+len(body))
		binary.BigEndian.PutUint16(out, r.RData.MX.Preference)
		return append(out, body...), nil
	case "TXT":
		var out []byte
		for _, s := range r.RData.TXT {
			b, err := txtBytes(s)
			if err != nil {
				return nil, err
			}
			if len(b) > 255 {
				return nil, errAt(0, "TXT string of %d bytes exceeds 255", len(b))
			}
			out = append(out, byte(len(b)))
			out = append(out, b...)
		}
		return out, nil
	}
	return nil, errAt(0, "unsupported type %q", r.Type)
}

// encodeNameAt encodes a name as if it started at absolute offset `start`,
// without touching e.buf, and returns the bytes. This keeps compression
// offsets correct for names embedded in rdata.
func (e *encoder) encodeNameAt(name Name, start int) ([]byte, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	saved := e.buf
	e.buf = make([]byte, start)
	copy(e.buf, saved)
	err := e.encodeName(name)
	out := append([]byte(nil), e.buf[start:]...)
	e.buf = saved
	return out, err
}

func txtBytes(s string) ([]byte, error) {
	if strings.HasPrefix(s, "hex:") {
		b, err := hex.DecodeString(s[4:])
		if err != nil {
			return nil, errAt(0, "invalid hex TXT string: %v", err)
		}
		return b, nil
	}
	if !utf8.ValidString(s) {
		return nil, errAt(0, "TXT string is not valid UTF-8; use hex: prefix")
	}
	return []byte(s), nil
}

func typeCode(s string) (uint16, error) {
	if c, ok := typeCodes[s]; ok {
		return c, nil
	}
	return 0, errAt(0, "unsupported type %q", s)
}

func classCode(s string) (uint16, error) {
	if s == "IN" {
		return classIN, nil
	}
	return 0, errAt(0, "unsupported class %q", s)
}

func validateName(name Name) error {
	if wireNameLen(name) > maxNameLen {
		return errAt(0, "name %q exceeds %d bytes in wire form", strings.Join(name, "."), maxNameLen)
	}
	for _, l := range name {
		if len(l) == 0 {
			return errAt(0, "name %q contains an empty label", strings.Join(name, "."))
		}
		if len(l) > maxLabelLen {
			return errAt(0, "label %q exceeds %d bytes", l, maxLabelLen)
		}
		if !isASCII([]byte(l)) {
			return errAt(0, "label %q contains non-ASCII bytes", l)
		}
	}
	return nil
}
