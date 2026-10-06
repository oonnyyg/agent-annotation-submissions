package main

import (
	"bufio"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// ErrorKind classifies member-level failures.
type ErrorKind string

const (
	ErrHeaderTruncated ErrorKind = "header_truncated"
	ErrBadHeader       ErrorKind = "bad_header"
	ErrDeflateCorrupt  ErrorKind = "deflate_corrupt"
	ErrTrailer         ErrorKind = "trailer_check_failed"
	ErrTrailingGarbage ErrorKind = "trailing_garbage"
	ErrLimit           ErrorKind = "limit_exceeded"
)

// CheckError describes a single failure with member index and byte range.
type CheckError struct {
	Kind        ErrorKind `json:"kind"`
	MemberIndex int       `json:"memberIndex"`
	OffsetStart int64     `json:"offsetStart"`
	OffsetEnd   int64     `json:"offsetEnd"`
	Message     string    `json:"message"`
}

func (e *CheckError) Error() string {
	return fmt.Sprintf("member %d [%d,%d): %s: %s", e.MemberIndex, e.OffsetStart, e.OffsetEnd, e.Kind, e.Message)
}

// MemberReport describes one gzip member.
type MemberReport struct {
	Index           int    `json:"index"`
	StartOffset     int64  `json:"startOffset"`
	EndOffset       int64  `json:"endOffset"` // one past the trailer
	HeaderBytes     int64  `json:"headerBytes"`
	CompressedBytes int64  `json:"compressedBytes"` // deflate stream only
	ExpandedBytes   int64  `json:"expandedBytes"`
	Name            string `json:"name,omitempty"`
	Comment         string `json:"comment,omitempty"`
}

// Limits bound header size, member count and total expanded bytes.
type Limits struct {
	MaxHeaderBytes   int64
	MaxMembers       int
	MaxExpandedBytes int64
}

// countingReader tracks how many bytes the consumer actually consumed.
// It implements io.ByteReader so flate does not wrap it in a bufio.Reader
// and never reads past the end of the deflate stream.
type countingReader struct {
	r   *bufio.Reader
	pos int64
}

func newCountingReader(r io.Reader) *countingReader {
	return &countingReader{r: bufio.NewReader(r)}
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.pos += int64(n)
	return n, err
}

func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.pos++
	}
	return b, err
}

func (c *countingReader) readFull(p []byte) error {
	n, err := io.ReadFull(c.r, p)
	c.pos += int64(n)
	return err
}

const (
	flagText    = 1 << 0
	flagHCRC    = 1 << 1
	flagExtra   = 1 << 2
	flagName    = 1 << 3
	flagComment = 1 << 4
	flagMask    = 1<<5 - 1
)

type memberHeader struct {
	name    string
	comment string
}

// parseHeader reads and validates one member header starting at offset start.
// headerBytes counts toward the caller's header limit.
func parseHeader(c *countingReader, start int64, idx int, maxHeader int64) (*memberHeader, error) {
	fail := func(kind ErrorKind, format string, args ...any) error {
		return &CheckError{Kind: kind, MemberIndex: idx, OffsetStart: start, OffsetEnd: c.pos, Message: fmt.Sprintf(format, args...)}
	}
	var fixed [10]byte
	if err := c.readFull(fixed[:]); err != nil {
		return nil, fail(ErrHeaderTruncated, "reading fixed header: %v", err)
	}
	if fixed[0] != 0x1f || fixed[1] != 0x8b {
		return nil, fail(ErrBadHeader, "bad magic %02x %02x", fixed[0], fixed[1])
	}
	if fixed[2] != 8 {
		return nil, fail(ErrBadHeader, "unsupported compression method %d", fixed[2])
	}
	flg := fixed[3]
	if flg & ^byte(flagMask) != 0 {
		return nil, fail(ErrBadHeader, "reserved flag bits set: 0x%02x", flg)
	}
	crc := crc32.NewIEEE()
	crc.Write(fixed[:])
	headerBytes := int64(len(fixed))
	overLimit := func() bool {
		return maxHeader > 0 && headerBytes > maxHeader
	}
	if overLimit() {
		return nil, fail(ErrLimit, "header exceeds %d bytes", maxHeader)
	}
	h := &memberHeader{}

	if flg&flagExtra != 0 {
		var sz [2]byte
		if err := c.readFull(sz[:]); err != nil {
			return nil, fail(ErrHeaderTruncated, "reading extra length: %v", err)
		}
		crc.Write(sz[:])
		headerBytes += 2
		n := int64(binary.LittleEndian.Uint16(sz[:]))
		if maxHeader > 0 && headerBytes+n > maxHeader {
			return nil, fail(ErrLimit, "header exceeds %d bytes", maxHeader)
		}
		if _, err := io.CopyN(crc, c, n); err != nil {
			return nil, fail(ErrHeaderTruncated, "reading extra field: %v", err)
		}
		headerBytes += n
	}
	readZeroString := func(what string) (string, error) {
		var buf []byte
		for {
			b, err := c.ReadByte()
			if err != nil {
				return "", fail(ErrHeaderTruncated, "reading %s: %v", what, err)
			}
			crc.Write([]byte{b})
			headerBytes++
			if overLimit() {
				return "", fail(ErrLimit, "header exceeds %d bytes", maxHeader)
			}
			if b == 0 {
				return string(buf), nil
			}
			buf = append(buf, b)
		}
	}
	var err error
	if flg&flagName != 0 {
		if h.name, err = readZeroString("name"); err != nil {
			return nil, err
		}
	}
	if flg&flagComment != 0 {
		if h.comment, err = readZeroString("comment"); err != nil {
			return nil, err
		}
	}
	if flg&flagHCRC != 0 {
		var stored [2]byte
		if err := c.readFull(stored[:]); err != nil {
			return nil, fail(ErrHeaderTruncated, "reading header crc: %v", err)
		}
		headerBytes += 2
		want := uint16(crc.Sum32())
		if got := binary.LittleEndian.Uint16(stored[:]); got != want {
			return nil, fail(ErrBadHeader, "header crc mismatch: got 0x%04x want 0x%04x", got, want)
		}
	}
	return h, nil
}

// limitWriter fails once more than max bytes would be written.
type limitWriter struct {
	w       io.Writer
	max     int64
	written int64
}

var errExpandedLimit = errors.New("expanded byte limit exceeded")

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.max > 0 && l.written+int64(len(p)) > l.max {
		allowed := l.max - l.written
		if allowed > 0 {
			n, _ := l.w.Write(p[:allowed])
			l.written += int64(n)
		}
		return 0, errExpandedLimit
	}
	n, err := l.w.Write(p)
	l.written += int64(n)
	return n, err
}

// checkMembers parses the whole stream member by member. dst may be nil for
// check-only mode. It returns per-member reports and the first error, if any.
func checkMembers(r io.Reader, dst io.Writer, lim Limits) ([]MemberReport, error) {
	c := newCountingReader(r)
	var reports []MemberReport
	var expandedTotal int64
	for idx := 0; ; idx++ {
		start := c.pos
		// Peek: clean EOF before a new member ends the file.
		if _, err := c.r.Peek(1); err == io.EOF {
			if idx == 0 {
				return nil, &CheckError{Kind: ErrHeaderTruncated, MemberIndex: 0, Message: "empty input"}
			}
			return reports, nil
		}
		if lim.MaxMembers > 0 && idx >= lim.MaxMembers {
			return reports, &CheckError{Kind: ErrLimit, MemberIndex: idx, OffsetStart: start, OffsetEnd: start, Message: fmt.Sprintf("member count exceeds %d", lim.MaxMembers)}
		}
		hdr, err := parseHeader(c, start, idx, lim.MaxHeaderBytes)
		if err != nil {
			if ce, ok := err.(*CheckError); ok && idx > 0 &&
				(ce.Kind == ErrBadHeader || ce.Kind == ErrHeaderTruncated) {
				ce.Kind = ErrTrailingGarbage
				ce.Message = "invalid bytes after member: " + ce.Message
			}
			return reports, err
		}
		rep := MemberReport{Index: idx, StartOffset: start, Name: hdr.name, Comment: hdr.comment}
		rep.HeaderBytes = c.pos - start

		fr := flate.NewReader(c)
		crc := crc32.NewIEEE()
		var lw *limitWriter
		var w io.Writer = crc
		if dst != nil {
			w = io.MultiWriter(crc, dst)
		}
		lw = &limitWriter{w: w, max: 0}
		if lim.MaxExpandedBytes > 0 {
			lw.max = lim.MaxExpandedBytes - expandedTotal
		}
		_, copyErr := io.Copy(lw, fr)
		closeErr := fr.Close()
		rep.CompressedBytes = c.pos - start - rep.HeaderBytes
		rep.ExpandedBytes = lw.written
		expandedTotal += lw.written
		if copyErr != nil {
			kind := ErrDeflateCorrupt
			if errors.Is(copyErr, errExpandedLimit) {
				kind = ErrLimit
			}
			return append(reports, rep), &CheckError{Kind: kind, MemberIndex: idx, OffsetStart: start, OffsetEnd: c.pos, Message: copyErr.Error()}
		}
		if closeErr != nil {
			return append(reports, rep), &CheckError{Kind: ErrDeflateCorrupt, MemberIndex: idx, OffsetStart: start, OffsetEnd: c.pos, Message: closeErr.Error()}
		}

		var trailer [8]byte
		if err := c.readFull(trailer[:]); err != nil {
			return append(reports, rep), &CheckError{Kind: ErrTrailer, MemberIndex: idx, OffsetStart: c.pos, OffsetEnd: c.pos, Message: fmt.Sprintf("reading trailer: %v", err)}
		}
		gotCRC := binary.LittleEndian.Uint32(trailer[0:4])
		gotSize := binary.LittleEndian.Uint32(trailer[4:8])
		if gotCRC != crc.Sum32() {
			return append(reports, rep), &CheckError{Kind: ErrTrailer, MemberIndex: idx, OffsetStart: c.pos - 8, OffsetEnd: c.pos, Message: fmt.Sprintf("crc32 mismatch: got 0x%08x want 0x%08x", gotCRC, crc.Sum32())}
		}
		if int64(gotSize) != rep.ExpandedBytes%4294967296 {
			return append(reports, rep), &CheckError{Kind: ErrTrailer, MemberIndex: idx, OffsetStart: c.pos - 8, OffsetEnd: c.pos, Message: fmt.Sprintf("isize mismatch: got %d want %d", gotSize, rep.ExpandedBytes%4294967296)}
		}
		rep.EndOffset = c.pos
		reports = append(reports, rep)
	}
}
