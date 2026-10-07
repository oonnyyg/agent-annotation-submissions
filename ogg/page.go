// Package ogg implements a strict, read-only auditor for Ogg container
// files as described by RFC 3533. It parses the version-0 paging format,
// verifies the Ogg CRC, tracks logical streams, and reassembles packets
// without decoding any audio or video payload.
package ogg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Header type flags of an Ogg page.
const (
	FlagContinuation = 0x01
	FlagBOS          = 0x02
	FlagEOS          = 0x04
)

const (
	capturePattern = "OggS"
	maxSegments    = 255
	maxPageSize    = 27 + maxSegments + maxSegments*255
)

// Page is a single Ogg page with its file position and raw segment data.
type Page struct {
	Offset     int64  // absolute file offset of the first header byte
	Version    byte   // stream structure version, must be 0
	HeaderType byte   // continuation/BOS/EOS flags
	Granule    uint64 // granule position, preserved verbatim
	Serial     uint32 // logical stream serial number
	Sequence   uint32 // page sequence number within the stream
	CRC        uint32 // checksum stored in the file
	Segments   []byte // segment table (LacingValues)
	Body       []byte // segment data, length is the sum of Segments
}

// PageError describes a malformed page and carries the file offset of the
// offending page so callers can report precise locations.
type PageError struct {
	Offset int64
	Msg    string
}

func (e *PageError) Error() string {
	return fmt.Sprintf("ogg: page at offset %d: %s", e.Offset, e.Msg)
}

// EndOffset returns the offset one past the last byte of the page.
func (p *Page) EndOffset() int64 {
	return p.Offset + int64(27+len(p.Segments)+len(p.Body))
}

// ReadPage reads exactly one page from r, which must be positioned at the
// start of a page. off is the absolute file offset of that position and is
// used only for error reporting. The page header, segment table, and body
// are validated, including the CRC-32 checksum used by Ogg.
func ReadPage(r io.Reader, off int64) (*Page, error) {
	var hdr [27]byte
	n, err := io.ReadFull(r, hdr[:])
	if err != nil {
		if errors.Is(err, io.EOF) && n == 0 {
			return nil, io.EOF
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, &PageError{Offset: off, Msg: "truncated page header"}
		}
		return nil, err
	}
	if string(hdr[0:4]) != capturePattern {
		return nil, &PageError{Offset: off, Msg: fmt.Sprintf("bad capture pattern %q, want \"OggS\"", hdr[0:4])}
	}
	if hdr[4] != 0 {
		return nil, &PageError{Offset: off, Msg: fmt.Sprintf("unsupported stream structure version %d", hdr[4])}
	}
	nseg := int(hdr[26])
	segTable := make([]byte, nseg)
	if _, err := io.ReadFull(r, segTable); err != nil {
		return nil, &PageError{Offset: off, Msg: "truncated segment table"}
	}
	bodyLen := 0
	for _, l := range segTable {
		bodyLen += int(l)
	}
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, &PageError{Offset: off, Msg: "truncated segment data"}
	}

	p := &Page{
		Offset:     off,
		Version:    hdr[4],
		HeaderType: hdr[5],
		Granule:    binary.LittleEndian.Uint64(hdr[6:14]),
		Serial:     binary.LittleEndian.Uint32(hdr[14:18]),
		Sequence:   binary.LittleEndian.Uint32(hdr[18:22]),
		CRC:        binary.LittleEndian.Uint32(hdr[22:26]),
		Segments:   segTable,
		Body:       body,
	}

	// The checksum is computed over the whole page with the CRC field
	// zeroed out.
	crcData := make([]byte, 0, 27+len(segTable)+len(body))
	crcData = append(crcData, hdr[:]...)
	for i := 22; i < 26; i++ {
		crcData[i] = 0
	}
	crcData = append(crcData, segTable...)
	crcData = append(crcData, body...)
	if got := crc32Ogg(crcData); got != p.CRC {
		return nil, &PageError{Offset: off, Msg: fmt.Sprintf("CRC mismatch: computed 0x%08x, stored 0x%08x", got, p.CRC)}
	}
	return p, nil
}
