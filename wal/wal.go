// Package wal implements offline verification of SQLite WAL files and
// reconstruction of a database page image from the last valid commit.
// It never invokes the SQLite engine and never modifies its inputs.
package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	WALHeaderSize   = 32
	FrameHeaderSize = 24

	magicLE uint32 = 0x377f0682 // checksums use little-endian words
	magicBE uint32 = 0x377f0683 // checksums use big-endian words
)

// Header is a parsed WAL header.
type Header struct {
	Magic      uint32
	Version    uint32
	PageSize   uint32
	Checkpoint uint32
	Salt1      uint32
	Salt2      uint32
	Checksum1  uint32
	Checksum2  uint32
	BigEndian  bool // checksum word byte order
}

// ParseHeader parses and validates a 32-byte WAL header.
func ParseHeader(b []byte) (*Header, error) {
	if len(b) < WALHeaderSize {
		return nil, errors.New("WAL header shorter than 32 bytes")
	}
	be := binary.BigEndian
	h := &Header{
		Magic:      be.Uint32(b[0:4]),
		Version:    be.Uint32(b[4:8]),
		PageSize:   be.Uint32(b[8:12]),
		Checkpoint: be.Uint32(b[12:16]),
		Salt1:      be.Uint32(b[16:20]),
		Salt2:      be.Uint32(b[20:24]),
		Checksum1:  be.Uint32(b[24:28]),
		Checksum2:  be.Uint32(b[28:32]),
	}
	switch h.Magic {
	case magicLE:
		h.BigEndian = false
	case magicBE:
		h.BigEndian = true
	default:
		return nil, fmt.Errorf("bad WAL magic 0x%08x", h.Magic)
	}
	if h.Version != 3007000 {
		return nil, fmt.Errorf("unsupported WAL version %d", h.Version)
	}
	if h.PageSize == 1 {
		h.PageSize = 65536
	}
	if h.PageSize < 512 || h.PageSize > 65536 || h.PageSize&(h.PageSize-1) != 0 {
		return nil, fmt.Errorf("invalid WAL page size %d", h.PageSize)
	}
	s1, s2 := checksum(h.BigEndian, 0, 0, b[0:24])
	if s1 != h.Checksum1 || s2 != h.Checksum2 {
		return nil, fmt.Errorf("WAL header checksum mismatch: got (%08x,%08x), want (%08x,%08x)",
			s1, s2, h.Checksum1, h.Checksum2)
	}
	return h, nil
}

// checksum computes the SQLite WAL cumulative checksum over data, whose
// length must be a multiple of 8. Word byte order is explicit, never the
// host byte order.
func checksum(bigEndian bool, s1, s2 uint32, data []byte) (uint32, uint32) {
	var order binary.ByteOrder = binary.LittleEndian
	if bigEndian {
		order = binary.BigEndian
	}
	for i := 0; i+8 <= len(data); i += 8 {
		s1 += order.Uint32(data[i:]) + s2
		s2 += order.Uint32(data[i+4:]) + s1
	}
	return s1, s2
}
