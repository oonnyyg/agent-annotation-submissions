// Package gzipx inspects gzip files member by member per RFC 1952,
// validating headers, DEFLATE streams, and trailers without buffering
// whole members in memory.
package gzipx

import (
	"bufio"
	"fmt"
	"io"
)

// Error kinds reported in MemberReport.Error and Report.ErrorKind.
const (
	ErrNone             = ""
	ErrNoMembers        = "no_members"
	ErrHeaderTruncated  = "header_truncated"
	ErrBadMagic         = "bad_magic"
	ErrUnsupportedAlgo  = "unsupported_method"
	ErrReservedFlags    = "reserved_flags"
	ErrHeaderCRC        = "header_crc_mismatch"
	ErrHeaderTooLarge   = "header_limit_exceeded"
	ErrDeflateCorrupt   = "deflate_corrupt"
	ErrDeflateTruncated = "deflate_truncated"
	ErrTrailerTruncated = "trailer_truncated"
	ErrCRCMismatch      = "crc_mismatch"
	ErrISizeMismatch    = "isize_mismatch"
	ErrTrailingGarbage  = "trailing_garbage"
	ErrTooManyMembers   = "member_limit_exceeded"
	ErrExpandedLimit    = "expanded_limit_exceeded"
)

const (
	flagText     = 1 << 0
	flagHCRC     = 1 << 1
	flagExtra    = 1 << 2
	flagName     = 1 << 3
	flagComment  = 1 << 4
	flagReserved = 0xE0
)

// Limits bounds resource use during inspection. Zero means unlimited.
type Limits struct {
	MaxHeaderBytes   int64 `json:"max_header_bytes"`
	MaxMembers       int   `json:"max_members"`
	MaxExpandedBytes int64 `json:"max_expanded_bytes"`
}

// MemberReport describes one gzip member.
type MemberReport struct {
	Index           int    `json:"index"`
	HeaderOffset    int64  `json:"header_offset"`
	DataOffset      int64  `json:"data_offset"`
	EndOffset       int64  `json:"end_offset"`
	CompressedBytes int64  `json:"compressed_bytes"`
	ExpandedBytes   uint64 `json:"expanded_bytes"`
	Name            string `json:"name,omitempty"`
	CRC32           uint32 `json:"crc32"`
	ISize           uint32 `json:"isize"`
	Error           string `json:"error,omitempty"`
}

// Report is the result of inspecting a whole gzip stream.
type Report struct {
	OK              bool           `json:"ok"`
	ErrorKind       string         `json:"error_kind,omitempty"`
	Error           string         `json:"error,omitempty"`
	TotalMembers    int            `json:"total_members"`
	CompressedTotal int64          `json:"compressed_total"`
	ExpandedTotal   uint64         `json:"expanded_total"`
	Members         []MemberReport `json:"members"`
}

// countingReader counts bytes consumed. It implements io.ByteReader so
// flate.NewReader reads exactly the bytes of the DEFLATE stream and never
// pre-reads into the next member.
type countingReader struct {
	r *bufio.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// Inspect reads a gzip stream from r member by member, writing expanded
// data to sink (may be nil for check-only). It never loads a whole member
// into memory. On failure OK is false and ErrorKind classifies the first
// failure; Members lists every member processed so far.
func Inspect(r io.Reader, lim Limits, sink io.Writer) Report {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	cr := &countingReader{r: br}
	rep := Report{Members: []MemberReport{}}
	var expandedTotal uint64

	for {
		if lim.MaxMembers > 0 && len(rep.Members) >= lim.MaxMembers {
			// Only fail if another member actually follows.
			if _, err := br.Peek(1); err == nil {
				return fail(rep, ErrTooManyMembers,
					fmt.Sprintf("member count exceeds limit %d", lim.MaxMembers))
			}
			break
		}
		m := MemberReport{Index: len(rep.Members), HeaderOffset: cr.n}
		kind, err := readMember(cr, lim, sink, &m, &expandedTotal)
		m.EndOffset = cr.n
		m.CompressedBytes = m.EndOffset - m.HeaderOffset
		rep.ExpandedTotal = expandedTotal
		rep.CompressedTotal = cr.n
		if kind == "eof" {
			break
		}
		rep.Members = append(rep.Members, m)
		if kind != ErrNone {
			rep.Members[len(rep.Members)-1].Error = kind
			return fail(rep, kind, err.Error())
		}
	}

	rep.TotalMembers = len(rep.Members)
	if rep.TotalMembers == 0 {
		return fail(rep, ErrNoMembers, "input contains no gzip members")
	}
	rep.OK = true
	return rep
}

func fail(rep Report, kind, msg string) Report {
	rep.OK = false
	rep.ErrorKind = kind
	rep.Error = msg
	rep.TotalMembers = len(rep.Members)
	return rep
}
