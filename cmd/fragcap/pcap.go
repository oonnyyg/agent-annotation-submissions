package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

// pcapReader reads classic libpcap files (not pcapng).
type pcapReader struct {
	r        io.Reader
	order    binary.ByteOrder
	nano     bool
	snaplen  uint32
	linkType uint32
}

type pcapRecord struct {
	Index     int // 1-based record index in the file
	Sec       uint32
	Subsec    uint32
	OrigLen   uint32
	Data      []byte
	Truncated bool // captured length shorter than on-wire length
}

const maxRecordLen = 16 << 20 // sanity bound for captured record length

func newPcapReader(r io.Reader) (*pcapReader, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("reading pcap magic: %w", err)
	}
	p := &pcapReader{r: r}
	switch string(magic[:]) {
	case "\xd4\xc3\xb2\xa1":
		p.order = binary.LittleEndian
	case "\xa1\xb2\xc3\xd4":
		p.order = binary.BigEndian
	case "\x4d\x3c\xb2\xa1":
		p.order = binary.LittleEndian
		p.nano = true
	case "\xa1\xb2\x3c\x4d":
		p.order = binary.BigEndian
		p.nano = true
	default:
		return nil, fmt.Errorf("unsupported magic %x (pcapng and other formats are not supported)", magic)
	}
	hdr := make([]byte, 20)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, fmt.Errorf("reading pcap global header: %w", err)
	}
	verMajor := p.order.Uint16(hdr[0:2])
	verMinor := p.order.Uint16(hdr[2:4])
	if verMajor != 2 || verMinor != 4 {
		return nil, fmt.Errorf("unsupported pcap version %d.%d", verMajor, verMinor)
	}
	p.snaplen = p.order.Uint32(hdr[12:16])
	p.linkType = p.order.Uint32(hdr[16:20])
	return p, nil
}

// next returns the next record, or io.EOF at a clean end of file.
func (p *pcapReader) next() (*pcapRecord, error) {
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(p.r, hdr); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("record header truncated: %w", err)
	}
	rec := &pcapRecord{
		Sec:     p.order.Uint32(hdr[0:4]),
		Subsec:  p.order.Uint32(hdr[4:8]),
		OrigLen: p.order.Uint32(hdr[12:16]),
	}
	capLen := p.order.Uint32(hdr[8:12])
	if capLen > maxRecordLen {
		return nil, fmt.Errorf("record captured length %d exceeds sanity bound", capLen)
	}
	rec.Data = make([]byte, capLen)
	if _, err := io.ReadFull(p.r, rec.Data); err != nil {
		return nil, fmt.Errorf("record data truncated: %w", err)
	}
	rec.Truncated = capLen < rec.OrigLen
	return rec, nil
}
