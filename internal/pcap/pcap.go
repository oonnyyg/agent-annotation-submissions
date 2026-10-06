// Package pcap reads classic libpcap capture files.
package pcap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Magic numbers for the classic pcap global header.
const (
	magicMicroBE = 0xa1b2c3d4
	magicMicroLE = 0xd4c3b2a1
	magicNanoBE  = 0xa1b23c4d
	magicNanoLE  = 0x4d3cb2a1
)

// Record is a single captured packet.
type Record struct {
	Index     int    // 1-based record number in the file
	Sec       uint32 // timestamp seconds
	Subsec    uint32 // timestamp microseconds or nanoseconds
	Nano      bool   // true if Subsec is nanoseconds
	InclLen   uint32 // bytes captured
	OrigLen   uint32 // bytes on the wire
	Data      []byte // captured bytes (len == InclLen)
	Truncated bool   // InclLen < OrigLen
}

// Reader iterates records of a classic pcap file.
type Reader struct {
	r     io.Reader
	order binary.ByteOrder
	nano  bool
	index int
}

// NewReader parses the global header and returns a record reader.
func NewReader(r io.Reader) (*Reader, error) {
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, fmt.Errorf("reading global header: %w", err)
	}
	magic := binary.BigEndian.Uint32(hdr[0:4])
	var order binary.ByteOrder
	var nano bool
	switch magic {
	case magicMicroBE:
		order, nano = binary.BigEndian, false
	case magicMicroLE:
		order, nano = binary.LittleEndian, false
	case magicNanoBE:
		order, nano = binary.BigEndian, true
	case magicNanoLE:
		order, nano = binary.LittleEndian, true
	default:
		return nil, fmt.Errorf("unsupported magic 0x%08x (not a classic pcap file)", magic)
	}
	if major, minor := order.Uint16(hdr[4:6]), order.Uint16(hdr[6:8]); major != 2 || minor != 4 {
		return nil, fmt.Errorf("unsupported pcap version %d.%d", major, minor)
	}
	if link := order.Uint32(hdr[20:24]); link != 1 {
		return nil, fmt.Errorf("unsupported link type %d (only Ethernet is supported)", link)
	}
	return &Reader{r: r, order: order, nano: nano}, nil
}

// Next returns the next record, or io.EOF when the file is exhausted.
func (rd *Reader) Next() (*Record, error) {
	hdr := make([]byte, 16)
	_, err := io.ReadFull(rd.r, hdr)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("record %d: truncated record header: %w", rd.index+1, err)
	}
	rd.index++
	rec := &Record{
		Index:   rd.index,
		Sec:     rd.order.Uint32(hdr[0:4]),
		Subsec:  rd.order.Uint32(hdr[4:8]),
		Nano:    rd.nano,
		InclLen: rd.order.Uint32(hdr[8:12]),
		OrigLen: rd.order.Uint32(hdr[12:16]),
	}
	const maxIncl = 1 << 24 // sanity bound against corrupt length fields
	if rec.InclLen > maxIncl {
		return nil, fmt.Errorf("record %d: implausible captured length %d", rec.Index, rec.InclLen)
	}
	rec.Data = make([]byte, rec.InclLen)
	if _, err := io.ReadFull(rd.r, rec.Data); err != nil {
		return nil, fmt.Errorf("record %d: truncated packet data: %w", rec.Index, err)
	}
	rec.Truncated = rec.InclLen < rec.OrigLen
	return rec, nil
}
