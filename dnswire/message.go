package dnswire

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

const headerLen = 12

// Header holds the 12-byte DNS header flags; counts are derived from the
// sections when encoding.
type Header struct {
	ID     uint16 `json:"id"`
	QR     bool   `json:"qr"`
	Opcode uint8  `json:"opcode"`
	AA     bool   `json:"aa"`
	TC     bool   `json:"tc"`
	RD     bool   `json:"rd"`
	RA     bool   `json:"ra"`
	Z      uint8  `json:"z"`
	RCode  uint8  `json:"rcode"`
}

type Question struct {
	Name   NameResult `json:"name"`
	QType  uint16     `json:"qtype"`
	QClass uint16     `json:"qclass"`
}

// Record is a resource record. Data is the decoded form for supported
// types; Raw carries the original RDATA bytes for unknown types.
type Record struct {
	Name  NameResult `json:"name"`
	Type  uint16     `json:"type"`
	Class uint16     `json:"class"`
	TTL   uint32     `json:"ttl"`
	Data  any        `json:"data,omitempty"`
	Raw   string     `json:"raw,omitempty"`
}

type Message struct {
	Header     Header     `json:"header"`
	Questions  []Question `json:"questions"`
	Answers    []Record   `json:"answers"`
	Authority  []Record   `json:"authority"`
	Additional []Record   `json:"additional"`
}

// MXData is the decoded form of MX RDATA.
type MXData struct {
	Preference uint16     `json:"preference"`
	Exchange   NameResult `json:"exchange"`
}

// DecodeMessage parses a raw DNS message and verifies that section counts
// match the bytes actually present.
func DecodeMessage(msg []byte) (*Message, error) {
	if len(msg) < headerLen {
		return nil, errAt(len(msg), "message shorter than %d-byte header", headerLen)
	}
	m := &Message{}
	h := &m.Header
	h.ID = binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	h.QR = flags&0x8000 != 0
	h.Opcode = uint8(flags >> 11 & 0xF)
	h.AA = flags&0x0400 != 0
	h.TC = flags&0x0200 != 0
	h.RD = flags&0x0100 != 0
	h.RA = flags&0x0080 != 0
	h.Z = uint8(flags >> 4 & 0x7)
	h.RCode = uint8(flags & 0xF)

	counts := []int{
		int(binary.BigEndian.Uint16(msg[4:6])),
		int(binary.BigEndian.Uint16(msg[6:8])),
		int(binary.BigEndian.Uint16(msg[8:10])),
		int(binary.BigEndian.Uint16(msg[10:12])),
	}
	off := headerLen
	var err error
	for i := 0; i < counts[0]; i++ {
		var q Question
		q.Name, err = readName(msg, off)
		if err != nil {
			return nil, err
		}
		off += q.Name.OccupiedBytes
		if off+4 > len(msg) {
			return nil, errAt(off, "question %d: fixed fields truncated", i)
		}
		q.QType = binary.BigEndian.Uint16(msg[off : off+2])
		q.QClass = binary.BigEndian.Uint16(msg[off+2 : off+4])
		off += 4
		m.Questions = append(m.Questions, q)
	}
	sections := []*[]Record{&m.Answers, &m.Authority, &m.Additional}
	names := []string{"answer", "authority", "additional"}
	for s, sec := range sections {
		for i := 0; i < counts[s+1]; i++ {
			var r Record
			r.Name, err = readName(msg, off)
			if err != nil {
				return nil, err
			}
			off += r.Name.OccupiedBytes
			if off+10 > len(msg) {
				return nil, errAt(off, "%s record %d: fixed fields truncated", names[s], i)
			}
			r.Type = binary.BigEndian.Uint16(msg[off : off+2])
			r.Class = binary.BigEndian.Uint16(msg[off+2 : off+4])
			r.TTL = binary.BigEndian.Uint32(msg[off+4 : off+8])
			rdlen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
			off += 10
			if off+rdlen > len(msg) {
				return nil, errAt(off, "%s record %d: RDATA of %d bytes exceeds message", names[s], i, rdlen)
			}
			rdata := msg[off : off+rdlen]
			if err := decodeRData(msg, off, rdata, &r); err != nil {
				return nil, err
			}
			off += rdlen
			*sec = append(*sec, r)
		}
	}
	if off != len(msg) {
		return nil, errAt(off, "%d trailing bytes after declared sections", len(msg)-off)
	}
	return m, nil
}

func decodeRData(msg []byte, off int, rdata []byte, r *Record) error {
	if r.Class != 1 { // only IN is supported for structured decode
		r.Raw = fmt.Sprintf("%x", rdata)
		return nil
	}
	switch r.Type {
	case 1: // A
		if len(rdata) != 4 {
			return errAt(off, "A record RDATA must be 4 bytes, got %d", len(rdata))
		}
		r.Data = net.IP(rdata).String()
	case 5: // CNAME
		nr, err := readName(msg, off)
		if err != nil {
			return err
		}
		if nr.OccupiedBytes != len(rdata) {
			return errAt(off, "CNAME target occupies %d bytes, RDATA declares %d", nr.OccupiedBytes, len(rdata))
		}
		r.Data = nr
	case 15: // MX
		if len(rdata) < 3 {
			return errAt(off, "MX RDATA too short for preference and exchange")
		}
		nr, err := readName(msg, off+2)
		if err != nil {
			return err
		}
		if 2+nr.OccupiedBytes != len(rdata) {
			return errAt(off, "MX exchange ends at %d bytes, RDATA declares %d", 2+nr.OccupiedBytes, len(rdata))
		}
		r.Data = MXData{Preference: binary.BigEndian.Uint16(rdata[:2]), Exchange: nr}
	case 16: // TXT
		var strings_ []string
		pos := 0
		for pos < len(rdata) {
			l := int(rdata[pos])
			if pos+1+l > len(rdata) {
				return errAt(off+pos, "TXT string of %d bytes exceeds RDATA boundary", l)
			}
			strings_ = append(strings_, string(rdata[pos+1:pos+1+l]))
			pos += 1 + l
		}
		if strings_ == nil {
			strings_ = []string{}
		}
		r.Data = strings_
	default:
		r.Raw = fmt.Sprintf("%x", rdata)
	}
	return nil
}

// String renders a name in dotted form for diagnostics ("." for root).
func (n Name) String() string {
	if len(n) == 0 {
		return "."
	}
	return strings.Join(n, ".")
}
