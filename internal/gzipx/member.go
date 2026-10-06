package gzipx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

var errHeaderLimit = errors.New("header size limit exceeded")

// headerReader counts header bytes, enforces the header size limit, and
// accumulates the CRC32 used for the optional FHCRC check.
type headerReader struct {
	cr    *countingReader
	n     int64
	limit int64
	sum   hash.Hash32
}

func newHeaderReader(cr *countingReader, limit int64) *headerReader {
	return &headerReader{cr: cr, limit: limit, sum: crc32.NewIEEE()}
}

func (h *headerReader) byte() (byte, error) {
	b, err := h.cr.ReadByte()
	if err != nil {
		if err == io.EOF {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	}
	h.n++
	h.sum.Write([]byte{b})
	if h.limit > 0 && h.n > h.limit {
		return 0, errHeaderLimit
	}
	return b, nil
}

// readMember parses one member starting at the current position.
// It returns kind "eof" when the stream is cleanly exhausted before a
// new member header begins.
func readMember(cr *countingReader, lim Limits, sink io.Writer, m *MemberReport, expandedTotal *uint64) (string, error) {
	h := newHeaderReader(cr, lim.MaxHeaderBytes)

	magic1, err := h.byte()
	if err == io.ErrUnexpectedEOF && h.n == 0 {
		return "eof", nil
	}
	if err != nil {
		return hdrErr(m, err)
	}
	magic2, err := h.byte()
	if err != nil {
		return hdrErr(m, err)
	}
	if magic1 != 0x1f || magic2 != 0x8b {
		if m.Index == 0 {
			return ErrBadMagic, fmt.Errorf("bad magic bytes %#02x %#02x at offset %d", magic1, magic2, m.HeaderOffset)
		}
		return ErrTrailingGarbage, fmt.Errorf("illegal bytes after member %d at offset %d", m.Index-1, m.HeaderOffset)
	}

	method, err := h.byte()
	if err != nil {
		return hdrErr(m, err)
	}
	if method != 8 {
		return ErrUnsupportedAlgo, fmt.Errorf("member %d: unsupported compression method %d at offset %d", m.Index, method, m.HeaderOffset+2)
	}
	flags, err := h.byte()
	if err != nil {
		return hdrErr(m, err)
	}
	if flags&flagReserved != 0 {
		return ErrReservedFlags, fmt.Errorf("member %d: reserved flag bits set (%#04x) at offset %d", m.Index, flags, m.HeaderOffset+3)
	}
	// MTIME(4) XFL(1) OS(1)
	for i := 0; i < 6; i++ {
		if _, err := h.byte(); err != nil {
			return hdrErr(m, err)
		}
	}
	if flags&flagExtra != 0 {
		lo, err := h.byte()
		if err != nil {
			return hdrErr(m, err)
		}
		hi, err := h.byte()
		if err != nil {
			return hdrErr(m, err)
		}
		xlen := int(lo) | int(hi)<<8
		for i := 0; i < xlen; i++ {
			if _, err := h.byte(); err != nil {
				return hdrErr(m, err)
			}
		}
	}
	if flags&flagName != 0 {
		name, err := readZeroTerminated(h)
		if err != nil {
			return hdrErr(m, err)
		}
		m.Name = name
	}
	if flags&flagComment != 0 {
		if _, err := readZeroTerminated(h); err != nil {
			return hdrErr(m, err)
		}
	}
	if flags&flagHCRC != 0 {
		// The CRC16 covers the header up to but not including itself.
		got := uint16(h.sum.Sum32() & 0xffff)
		lo, err := h.byte()
		if err != nil {
			return hdrErr(m, err)
		}
		hi, err := h.byte()
		if err != nil {
			return hdrErr(m, err)
		}
		want := uint16(lo) | uint16(hi)<<8
		if want != got {
			return ErrHeaderCRC, fmt.Errorf("member %d: header CRC mismatch at offset %d", m.Index, cr.n-2)
		}
	}

	m.DataOffset = cr.n
	computedCRC, kind, err := inflate(cr, lim, sink, m, expandedTotal)
	if kind != ErrNone {
		return kind, err
	}

	var trailer [8]byte
	if _, err := io.ReadFull(cr, trailer[:]); err != nil {
		return ErrTrailerTruncated, fmt.Errorf("member %d: truncated trailer at offset %d", m.Index, cr.n)
	}
	m.CRC32 = binary.LittleEndian.Uint32(trailer[0:4])
	m.ISize = binary.LittleEndian.Uint32(trailer[4:8])
	if m.CRC32 != computedCRC {
		return ErrCRCMismatch, fmt.Errorf("member %d: CRC32 mismatch at offset %d", m.Index, cr.n-8)
	}
	if m.ISize != uint32(m.ExpandedBytes) {
		return ErrISizeMismatch, fmt.Errorf("member %d: ISIZE mismatch at offset %d", m.Index, cr.n-4)
	}
	return ErrNone, nil
}

func hdrErr(m *MemberReport, err error) (string, error) {
	if err == errHeaderLimit {
		return ErrHeaderTooLarge, fmt.Errorf("member %d: %v", m.Index, err)
	}
	if err == io.ErrUnexpectedEOF {
		return ErrHeaderTruncated, fmt.Errorf("member %d: truncated header at offset %d", m.Index, m.HeaderOffset)
	}
	return ErrHeaderTruncated, fmt.Errorf("member %d: %v", m.Index, err)
}

func readZeroTerminated(h *headerReader) (string, error) {
	var buf []byte
	for {
		b, err := h.byte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			return string(buf), nil
		}
		buf = append(buf, b)
	}
}
