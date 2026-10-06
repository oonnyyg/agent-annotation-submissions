// Package tzif parses TZif (RFC 8536) version 2 and 3 files and exposes
// the 64-bit transition table for auditing absolute/local time mappings.
package tzif

import (
	"encoding/binary"
	"fmt"
)

// Type is a local time type: a UTC offset with a designation.
type Type struct {
	Offset int64  // seconds east of UTC
	IsDST  bool   // informational only; never used for offset math
	Abbr   string // time zone abbreviation
}

// File is a parsed TZif file using its 64-bit (v2/v3 second) data block.
type File struct {
	Version     byte    // '2' or '3'
	Transitions []int64 // sorted, strictly increasing transition times (UTC seconds)
	TransTypes  []int   // type index applied at and after each transition
	Types       []Type
	InitialType int // type in effect before the first transition
}

// Window returns the half-open [first, last) transition window served.
func (f *File) Window() (first, last int64) {
	return f.Transitions[0], f.Transitions[len(f.Transitions)-1]
}

// Error describes a malformed file, locating the byte offset when known.
type Error struct {
	Offset int64 // byte offset in file, -1 if not applicable
	Msg    string
}

func (e *Error) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("tzif: byte %d: %s", e.Offset, e.Msg)
	}
	return "tzif: " + e.Msg
}

func errAt(off int64, format string, args ...any) error {
	return &Error{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

type header struct {
	version  byte
	isutcnt  uint32
	isstdcnt uint32
	leapcnt  uint32
	timecnt  uint32
	typecnt  uint32
	charcnt  uint32
}

const headerSize = 44

func parseHeader(b []byte, base int64) (header, error) {
	var h header
	if len(b) < headerSize {
		return h, errAt(base+int64(len(b)), "truncated header: need %d bytes, have %d", headerSize, len(b))
	}
	if string(b[0:4]) != "TZif" {
		return h, errAt(base, "bad magic %q, want TZif", string(b[0:4]))
	}
	h.version = b[4]
	h.isutcnt = binary.BigEndian.Uint32(b[20:24])
	h.isstdcnt = binary.BigEndian.Uint32(b[24:28])
	h.leapcnt = binary.BigEndian.Uint32(b[28:32])
	h.timecnt = binary.BigEndian.Uint32(b[32:36])
	h.typecnt = binary.BigEndian.Uint32(b[36:40])
	h.charcnt = binary.BigEndian.Uint32(b[40:44])
	return h, nil
}

// dataSize returns the byte size of a data block for the given header,
// with 64-bit (wide) or 32-bit transition/expiration fields.
func dataSize(h header, wide bool) int64 {
	tsz := int64(4)
	if wide {
		tsz = 8
	}
	return tsz*int64(h.timecnt) + int64(h.timecnt) + 6*int64(h.typecnt) +
		int64(h.charcnt) + (tsz+4)*int64(h.leapcnt) +
		int64(h.isstdcnt) + int64(h.isutcnt)
}

type rawType struct {
	off   int32
	isdst bool
	ind   uint8
}

type block struct {
	trans    []int64
	tind     []uint8
	types    []rawType
	chars    string
	transOff int64 // file offset of transition array (for error reporting)
	tindOff  int64
	typesOff int64
	charsOff int64
}

func parseBlock(b []byte, base int64, h header, wide bool) (*block, error) {
	tsz := int64(4)
	if wide {
		tsz = 8
	}
	need := dataSize(h, wide)
	if int64(len(b)) < need {
		return nil, errAt(base+int64(len(b)), "truncated data block: need %d bytes, have %d", need, len(b))
	}
	bk := &block{transOff: base}
	pos := int64(0)
	bk.trans = make([]int64, h.timecnt)
	for i := range bk.trans {
		if wide {
			bk.trans[i] = int64(binary.BigEndian.Uint64(b[pos : pos+8]))
		} else {
			bk.trans[i] = int64(int32(binary.BigEndian.Uint32(b[pos : pos+4])))
		}
		pos += tsz
	}
	bk.tindOff = base + pos
	bk.tind = make([]uint8, h.timecnt)
	copy(bk.tind, b[pos:pos+int64(h.timecnt)])
	pos += int64(h.timecnt)
	bk.typesOff = base + pos
	bk.types = make([]rawType, h.typecnt)
	for i := range bk.types {
		bk.types[i] = rawType{
			off:   int32(binary.BigEndian.Uint32(b[pos : pos+4])),
			isdst: b[pos+4] != 0,
			ind:   b[pos+5],
		}
		pos += 6
	}
	bk.charsOff = base + pos
	bk.chars = string(b[pos : pos+int64(h.charcnt)])
	return bk, nil
}

// Parse reads a TZif v2 or v3 file and returns the 64-bit data block view.
// Files with leap second records are rejected.
func Parse(data []byte) (*File, error) {
	h1, err := parseHeader(data, 0)
	if err != nil {
		return nil, err
	}
	if h1.version != '2' && h1.version != '3' {
		return nil, errAt(4, "unsupported version %q: only v2 and v3 are supported", string(h1.version))
	}
	// Skip the 32-bit block to locate the second header.
	off := int64(headerSize) + dataSize(h1, false)
	if int64(len(data)) < off {
		return nil, errAt(int64(len(data)), "truncated v1 data block: need %d bytes, have %d", off, len(data))
	}
	h2, err := parseHeader(data[off:], off)
	if err != nil {
		return nil, err
	}
	blkBase := off + headerSize
	if h2.leapcnt > 0 {
		leapOff := blkBase + dataSize(h2, true) - int64(h2.isstdcnt) - int64(h2.isutcnt) - 12*int64(h2.leapcnt)
		return nil, errAt(leapOff, "leap second records (%d) are not supported", h2.leapcnt)
	}
	bk, err := parseBlock(data[blkBase:], blkBase, h2, true)
	if err != nil {
		return nil, err
	}
	if len(bk.trans) < 2 {
		return nil, errAt(bk.transOff, "need at least 2 transitions, have %d", len(bk.trans))
	}
	for i := 1; i < len(bk.trans); i++ {
		if bk.trans[i] <= bk.trans[i-1] {
			return nil, errAt(bk.transOff+int64(i)*8, "transitions not strictly increasing at index %d (%d <= %d)", i, bk.trans[i], bk.trans[i-1])
		}
	}
	f := &File{Version: h1.version, Transitions: bk.trans}
	for i, ti := range bk.tind {
		if int(ti) >= len(bk.types) {
			return nil, errAt(bk.tindOff+int64(i), "transition %d references type index %d, only %d types", i, ti, len(bk.types))
		}
		f.TransTypes = append(f.TransTypes, int(ti))
	}
	for i, rt := range bk.types {
		if int(rt.ind) >= len(bk.chars) {
			return nil, errAt(bk.typesOff+int64(i)*6+5, "type %d designation index %d out of range (charcnt %d)", i, rt.ind, len(bk.chars))
		}
		abbr := bk.chars[rt.ind:]
		for j := 0; j < len(abbr); j++ {
			if abbr[j] == 0 {
				abbr = abbr[:j]
				break
			}
		}
		f.Types = append(f.Types, Type{Offset: int64(rt.off), IsDST: rt.isdst, Abbr: abbr})
	}
	// Initial type: first non-DST type not used by any transition, per RFC 8536;
	// describes pre-window context only, never served for queries.
	f.InitialType = 0
	used := map[int]bool{}
	for _, t := range f.TransTypes {
		used[t] = true
	}
	for i, t := range f.Types {
		if !used[i] && !t.IsDST {
			f.InitialType = i
			break
		}
	}
	return f, nil
}
