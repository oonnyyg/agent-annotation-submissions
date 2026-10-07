// Package ogg implements a strict RFC 3533 (version 0) Ogg page parser
// and logical-stream/packet auditor. It never decodes media payloads.
package ogg

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// Ogg uses CRC-32 with polynomial 0x04c11db7, no reflection, init/xorout 0.
var oggCRCTable = crc32.MakeTable(0x04c11db7)

const (
	capturePattern = "OggS"
	maxSegments    = 255
	headerSize     = 27
	// MaxPageSize bounds a single page: 27-byte header + 255 segment
	// table bytes + 255*255 payload bytes.
	MaxPageSize = headerSize + maxSegments + maxSegments*255
)

// HeaderType flags of a page.
const (
	FlagContinued uint8 = 0x01
	FlagBOS       uint8 = 0x02
	FlagEOS       uint8 = 0x04
)

// Page is a parsed Ogg page. Data holds the raw payload bytes (valid only
// until the next read from the same Reader).
type Page struct {
	Offset       int64 // file offset of the first byte of the page
	Version      uint8
	HeaderType   uint8
	GranulePos   uint64 // raw granule position, kept as-is
	Serial       uint32 // logical stream serial number
	Sequence     uint32 // page sequence number within the stream
	CRC          uint32 // checksum stored in the file
	SegmentTable []uint8
	Data         []byte // payload, length == sum(SegmentTable)
}

// Continued reports whether the page continues a packet from a prior page.
func (p *Page) Continued() bool { return p.HeaderType&FlagContinued != 0 }

// BOS reports whether this is a beginning-of-stream page.
func (p *Page) BOS() bool { return p.HeaderType&FlagBOS != 0 }

// EOS reports whether this is an end-of-stream page.
func (p *Page) EOS() bool { return p.HeaderType&FlagEOS != 0 }

// Reader reads Ogg pages sequentially from an io.Reader, tracking offsets.
type Reader struct {
	r   io.Reader
	off int64
	buf [MaxPageSize]byte
}

// NewReader wraps r. The reader is consumed incrementally; the whole input
// is never held in memory.
func NewReader(r io.Reader) *Reader { return &Reader{r: r} }

// Offset returns the offset of the next byte to be read.
func (rd *Reader) Offset() int64 { return rd.off }

// Next reads and validates the next page. It returns io.EOF only when the
// stream ends exactly on a page boundary. Any structural or checksum error
// includes the file offset and is fatal: the caller must not skip pages.
func (rd *Reader) Next() (*Page, error) {
	hdr := rd.buf[:headerSize]
	if _, err := io.ReadFull(rd.r, hdr); err != nil {
		if err == io.EOF {
			return nil, io.EOF // clean end on a page boundary
		}
		return nil, fmt.Errorf("offset %d: truncated page header: %w", rd.off, err)
	}
	base := rd.off
	rd.off += headerSize

	if string(hdr[0:4]) != capturePattern {
		return nil, fmt.Errorf("offset %d: bad capture pattern %q, expected %q", base, hdr[0:4], capturePattern)
	}
	if hdr[4] != 0 {
		return nil, fmt.Errorf("offset %d: unsupported stream structure version %d", base, hdr[4])
	}
	nseg := int(hdr[26])
	segTable := rd.buf[headerSize : headerSize+nseg]
	if _, err := io.ReadFull(rd.r, segTable); err != nil {
		return nil, fmt.Errorf("offset %d: truncated segment table (%d bytes): %w", rd.off, nseg, err)
	}
	rd.off += int64(nseg)

	payloadLen := 0
	for _, s := range segTable {
		payloadLen += int(s)
	}
	data := rd.buf[headerSize+nseg : headerSize+nseg+payloadLen]
	if _, err := io.ReadFull(rd.r, data); err != nil {
		return nil, fmt.Errorf("offset %d: truncated page payload (%d bytes): %w", rd.off, payloadLen, err)
	}
	rd.off += int64(payloadLen)

	storedCRC := binary.LittleEndian.Uint32(hdr[22:26])
	// CRC covers header+payload with the CRC field itself zeroed.
	crc := crc32.Checksum(hdr[:22], oggCRCTable)
	crc = crc32.Update(crc, oggCRCTable, []byte{0, 0, 0, 0})
	crc = crc32.Update(crc, oggCRCTable, hdr[26:headerSize])
	crc = crc32.Update(crc, oggCRCTable, segTable)
	crc = crc32.Update(crc, oggCRCTable, data)
	if crc != storedCRC {
		return nil, fmt.Errorf("offset %d: CRC mismatch: computed 0x%08x, stored 0x%08x", base, crc, storedCRC)
	}

	page := &Page{
		Offset:       base,
		Version:      hdr[4],
		HeaderType:   hdr[5],
		GranulePos:   binary.LittleEndian.Uint64(hdr[6:14]),
		Serial:       binary.LittleEndian.Uint32(hdr[14:18]),
		Sequence:     binary.LittleEndian.Uint32(hdr[18:22]),
		CRC:          storedCRC,
		SegmentTable: append([]uint8(nil), segTable...),
		Data:         append([]byte(nil), data...),
	}
	return page, nil
}
