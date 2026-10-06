package elfmap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	ehdrSize64 = 64
	phdrSize64 = 56

	eiIdent     = 16
	eiClass     = 4
	eiData      = 5
	elfclass64  = 2
	elfdata2lsb = 1

	etExec = 2
	etDyn  = 3

	emX8664 = 62

	ptLoad = 1

	pfX = 1
	pfW = 2
	pfR = 4
)

// Header64 is the parsed subset of the ELF64 header needed by the auditor.
type Header64 struct {
	Type    uint16
	Machine uint16
	Entry   uint64
	Phoff   uint64
	Phentsz uint16
	Phnum   uint16
}

// Prog64 is one ELF64 program header.
type Prog64 struct {
	Type   uint32
	Flags  uint32
	Off    uint64
	Vaddr  uint64
	Paddr  uint64
	Filesz uint64
	Memsz  uint64
	Align  uint64
}

// ParseHeader reads and validates the ELF identification bytes and fixed
// ELF64 header. Only little-endian x86-64 executables and shared objects are
// accepted.
func ParseHeader(r io.ReaderAt, fileSize int64) (*Header64, error) {
	if fileSize < ehdrSize64 {
		return nil, fmt.Errorf("file too small for ELF64 header: %d bytes", fileSize)
	}
	var ident [eiIdent]byte
	if _, err := r.ReadAt(ident[:], 0); err != nil {
		return nil, err
	}
	if ident[0] != 0x7f || ident[1] != 'E' || ident[2] != 'L' || ident[3] != 'F' {
		return nil, errors.New("bad ELF magic")
	}
	if ident[eiClass] != elfclass64 {
		return nil, fmt.Errorf("unsupported ELF class %d (only ELF64)", ident[eiClass])
	}
	if ident[eiData] != elfdata2lsb {
		return nil, fmt.Errorf("unsupported endianness %d (only little-endian)", ident[eiData])
	}

	var raw [ehdrSize64]byte
	if _, err := r.ReadAt(raw[:], 0); err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	h := &Header64{
		Type:    le.Uint16(raw[16:18]),
		Machine: le.Uint16(raw[18:20]),
		Entry:   le.Uint64(raw[24:32]),
		Phoff:   le.Uint64(raw[32:40]),
		Phentsz: le.Uint16(raw[54:56]),
		Phnum:   le.Uint16(raw[56:58]),
	}
	if h.Type != etExec && h.Type != etDyn {
		return nil, fmt.Errorf("unsupported e_type %d (only ET_EXEC/ET_DYN)", h.Type)
	}
	if h.Machine != emX8664 {
		return nil, fmt.Errorf("unsupported e_machine %d (only x86-64)", h.Machine)
	}
	if h.Phentsz != phdrSize64 {
		return nil, fmt.Errorf("unexpected e_phentsize %d (want %d)", h.Phentsz, phdrSize64)
	}
	if h.Phnum == 0 {
		return h, nil
	}
	tableSize, mOverflow := mulOverflow(uint64(h.Phnum), uint64(h.Phentsz))
	end, overflow := addOverflow(h.Phoff, tableSize)
	if mOverflow || overflow || end > uint64(fileSize) {
		return nil, fmt.Errorf("program header table out of file range: off=%d num=%d", h.Phoff, h.Phnum)
	}
	return h, nil
}

// ParseProgramHeaders reads every program header described by the header.
func ParseProgramHeaders(r io.ReaderAt, h *Header64) ([]Prog64, error) {
	phdrs := make([]Prog64, 0, h.Phnum)
	var raw [phdrSize64]byte
	le := binary.LittleEndian
	for i := uint16(0); i < h.Phnum; i++ {
		base := int64(h.Phoff) + int64(i)*phdrSize64
		if _, err := r.ReadAt(raw[:], base); err != nil {
			return nil, err
		}
		p := Prog64{
			Type:   le.Uint32(raw[0:4]),
			Flags:  le.Uint32(raw[4:8]),
			Off:    le.Uint64(raw[8:16]),
			Vaddr:  le.Uint64(raw[16:24]),
			Paddr:  le.Uint64(raw[24:32]),
			Filesz: le.Uint64(raw[32:40]),
			Memsz:  le.Uint64(raw[40:48]),
			Align:  le.Uint64(raw[48:56]),
		}
		phdrs = append(phdrs, p)
	}
	return phdrs, nil
}

func addOverflow(a, b uint64) (uint64, bool) {
	s := a + b
	return s, s < a
}

func mulOverflow(a, b uint64) (uint64, bool) {
	if a != 0 && (a*b)/a != b {
		return 0, true
	}
	return a * b, false
}
