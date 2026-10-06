package tzif

import (
	"encoding/binary"
	"fmt"
)

// ParseError describes a malformed TZif file, with the byte offset involved.
type ParseError struct {
	Offset int
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("tzif: byte %d: %s", e.Offset, e.Msg)
}

func perr(off int, format string, args ...any) *ParseError {
	return &ParseError{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

type header struct {
	version  byte
	utcnt    int64
	stdcnt   int64
	leapcnt  int64
	timecnt  int64
	typecnt  int64
	charcnt  int64
	dataOff  int // offset of this header's data block
	blockLen int // length of this header's data block
}

const headerLen = 44

func parseHeader(b []byte, off int) (header, error) {
	var h header
	if len(b)-off < headerLen {
		return h, perr(off, "truncated header: need %d bytes, have %d", headerLen, len(b)-off)
	}
	hd := b[off : off+headerLen]
	if string(hd[0:4]) != "TZif" {
		return h, perr(off, "bad magic %q, want TZif", string(hd[0:4]))
	}
	h.version = hd[4]
	h.utcnt = int64(binary.BigEndian.Uint32(hd[20:24]))
	h.stdcnt = int64(binary.BigEndian.Uint32(hd[24:28]))
	h.leapcnt = int64(binary.BigEndian.Uint32(hd[28:32]))
	h.timecnt = int64(binary.BigEndian.Uint32(hd[32:36]))
	h.typecnt = int64(binary.BigEndian.Uint32(hd[36:40]))
	h.charcnt = int64(binary.BigEndian.Uint32(hd[40:44]))
	return h, nil
}

// blockLen computes the data block length for a header with the given
// transition-time size (4 or 8 bytes).
func (h header) computeBlockLen(timeSize int64) int64 {
	return h.timecnt*(timeSize+1) + h.typecnt*6 + h.charcnt +
		h.leapcnt*(timeSize+4) + h.stdcnt + h.utcnt
}

// Parse reads a TZif v2 or v3 file and returns the audited Zone built from
// the 64-bit (second) data block. Leap-second data is rejected.
func Parse(b []byte) (*Zone, error) {
	h1, err := parseHeader(b, 0)
	if err != nil {
		return nil, err
	}
	if h1.version != '2' && h1.version != '3' {
		return nil, perr(4, "unsupported version %q: only v2 and v3 are supported", string(h1.version))
	}
	if h1.leapcnt != 0 {
		return nil, perr(28, "leap-second records are not supported (leapcnt=%d)", h1.leapcnt)
	}
	len1 := h1.computeBlockLen(4)
	off2 := int(headerLen + len1)
	if int64(len(b)) < int64(off2) {
		return nil, perr(len(b), "truncated: v1 data block ends at byte %d but file has %d bytes", off2, len(b))
	}
	h2, err := parseHeader(b, off2)
	if err != nil {
		return nil, err
	}
	if h2.version != h1.version {
		return nil, perr(off2+4, "second header version %q differs from first %q", string(h2.version), string(h1.version))
	}
	if h2.leapcnt != 0 {
		return nil, perr(off2+28, "leap-second records are not supported (leapcnt=%d)", h2.leapcnt)
	}
	dataOff := off2 + headerLen
	len2 := h2.computeBlockLen(8)
	if int64(len(b)) < int64(dataOff)+len2 {
		return nil, perr(len(b), "truncated: 64-bit data block ends at byte %d but file has %d bytes", int64(dataOff)+len2, len(b))
	}
	d := b[dataOff : dataOff+int(len2)]
	return parseBlock(d, dataOff, h2)
}

func parseBlock(d []byte, base int, h header) (*Zone, error) {
	if h.timecnt < 2 {
		return nil, perr(base, "need at least two 64-bit transitions, have %d", h.timecnt)
	}
	if h.typecnt < 1 {
		return nil, perr(base, "no local time types (typecnt=0)")
	}
	n := int(h.timecnt)
	trans := make([]int64, n)
	for i := 0; i < n; i++ {
		trans[i] = int64(binary.BigEndian.Uint64(d[i*8:]))
		if i > 0 && trans[i] <= trans[i-1] {
			return nil, perr(base+i*8, "transition %d (%d) is not strictly after previous (%d)", i, trans[i], trans[i-1])
		}
	}
	typesOff := n * 8
	types := make([]int, n)
	for i := 0; i < n; i++ {
		idx := int(d[typesOff+i])
		if int64(idx) >= h.typecnt {
			return nil, perr(base+typesOff+i, "transition %d references type index %d but typecnt=%d", i, idx, h.typecnt)
		}
		types[i] = idx
	}
	ttOff := typesOff + n
	locals := make([]LocalTime, int(h.typecnt))
	for i := 0; i < int(h.typecnt); i++ {
		rec := d[ttOff+i*6:]
		locals[i].Offset = int64(int32(binary.BigEndian.Uint32(rec[0:4])))
		locals[i].IsDST = rec[4] != 0
		locals[i].Abbr = "" // filled below
		_ = rec[5]
	}
	charOff := ttOff + int(h.typecnt)*6
	chars := d[charOff : charOff+int(h.charcnt)]
	for i := 0; i < int(h.typecnt); i++ {
		ai := int(d[ttOff+i*6+5])
		if ai >= len(chars) {
			return nil, perr(base+ttOff+i*6+5, "type %d abbreviation index %d out of range (charcnt=%d)", i, ai, h.charcnt)
		}
		end := ai
		for end < len(chars) && chars[end] != 0 {
			end++
		}
		if end == len(chars) {
			return nil, perr(base+charOff+ai, "type %d abbreviation not NUL-terminated", i)
		}
		locals[i].Abbr = string(chars[ai:end])
	}
	return &Zone{Transitions: trans, Types: types, Locals: locals}, nil
}