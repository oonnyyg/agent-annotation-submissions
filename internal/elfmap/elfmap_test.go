package elfmap

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// buildELF assembles a minimal little-endian x86-64 ELF64 image. Program
// header data is filled with distinctive per-segment byte patterns and any
// extra size is zero padded.
func buildELF(t *testing.T, typ uint16, entry uint64, ph []Prog64, extra int) []byte {
	t.Helper()
	phoff := uint64(ehdrSize64)
	size := int(phoff) + len(ph)*phdrSize64 + extra
	for _, p := range ph {
		if e := int(p.Off + p.Filesz); e > size {
			size = e
		}
	}
	buf := make([]byte, size)
	copy(buf[0:4], []byte{0x7f, 'E', 'L', 'F'})
	buf[4] = elfclass64
	buf[5] = elfdata2lsb
	put16(buf[16:], typ)
	put16(buf[18:], emX8664)
	put64(buf[24:], entry)
	put64(buf[32:], phoff)
	put16(buf[54:], phdrSize64)
	put16(buf[56:], uint16(len(ph)))
	for i, p := range ph {
		base := phoff + uint64(i*phdrSize64)
		put32(buf[base:], p.Type)
		put32(buf[base+4:], p.Flags)
		put64(buf[base+8:], p.Off)
		put64(buf[base+16:], p.Vaddr)
		put64(buf[base+24:], p.Paddr)
		put64(buf[base+32:], p.Filesz)
		put64(buf[base+40:], p.Memsz)
		put64(buf[base+48:], p.Align)
		if p.Filesz > 0 {
			for j := uint64(0); j < p.Filesz; j++ {
				buf[p.Off+j] = byte(0xA0 + i)
			}
		}
	}
	return buf
}

func put16(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) }
func put32(b []byte, v uint32) {
	for i := 0; i < 4; i++ {
		b[i] = byte(v >> (8 * i))
	}
}
func put64(b []byte, v uint64) {
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (8 * i))
	}
}

func bytesReader(buf []byte) *sliceReader { return &sliceReader{buf: buf} }

type sliceReader struct{ buf []byte }

func (s *sliceReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(s.buf)) {
		return 0, errRange
	}
	n := copy(p, s.buf[off:])
	if n < len(p) {
		return n, errRange
	}
	return n, nil
}

var errRange = errors.New("read out of range")

func TestCrossSegmentReadAndZeroFill(t *testing.T) {
	// Segment 0: file data at 0x400000, 0x100 file, 0x180 mem (0x80 zeros).
	// Segment 1: adjacent at 0x400180, 0x80 file, 0x80 mem.
	ph := []Prog64{
		{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x100, Memsz: 0x180, Align: 0x1000},
		{Type: ptLoad, Flags: pfR | pfX, Off: 0x2180, Vaddr: 0x400180, Filesz: 0x80, Memsz: 0x80, Align: 0x1000},
	}
	buf := buildELF(t, etExec, 0x400180, ph, 0)
	r := bytesReader(buf)
	h, err := ParseHeader(r, int64(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	phdrs, err := ParseProgramHeaders(r, h)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Build(h, phdrs, int64(len(buf)), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Read across: tail of seg0 file, zero fill, seg1 file.
	res, err := m.Read(r, 0x4000f0, 0x100)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pieces) != 3 {
		t.Fatalf("want 3 pieces (data/zero/data), got %d: %+v", len(res.Pieces), res.Pieces)
	}
	if res.Pieces[0].Kind != "data" || res.Pieces[0].Start != 0x4000f0 || res.Pieces[0].End != 0x400100 {
		t.Fatalf("bad piece 0: %+v", res.Pieces[0])
	}
	if res.Pieces[1].Kind != "zero" || res.Pieces[1].Start != 0x400100 || res.Pieces[1].End != 0x400180 {
		t.Fatalf("bad zero piece: %+v", res.Pieces[1])
	}
	if res.Pieces[2].Kind != "data" || res.Pieces[2].Start != 0x400180 || res.Pieces[2].End != 0x4001f0 {
		t.Fatalf("bad piece 2: %+v", res.Pieces[2])
	}
	d0, _ := base64.StdEncoding.DecodeString(res.Pieces[0].DataB64)
	if len(d0) != 0x10 || d0[0] != 0xA0 {
		t.Fatalf("seg0 data mismatch: len=%d byte=%#x", len(d0), d0[0])
	}
	d2, _ := base64.StdEncoding.DecodeString(res.Pieces[2].DataB64)
	if len(d2) != 0x70 || d2[0] != 0xA1 {
		t.Fatalf("seg1 data mismatch: len=%d byte=%#x", len(d2), d2[0])
	}

	// A read touching the gap above seg1 must fail entirely.
	if _, err := m.Read(r, 0x4001f0, 0x20); err == nil {
		t.Fatal("expected failure reading unmapped hole")
	}
	// Length cap.
	if _, err := m.Read(r, 0x400000, MaxReadLen+1); err == nil {
		t.Fatal("expected failure for oversized read")
	}
}

func TestSharedPageWXMerge(t *testing.T) {
	// Two segments meet inside one 4096 page: r-- ending at 0x201010 and
	// rwx starting at 0x201010 makes that page rwx union => wx.
	ph := []Prog64{
		{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x200000, Filesz: 0x1010, Memsz: 0x1010, Align: 0x1000},
		{Type: ptLoad, Flags: pfR | pfW | pfX, Off: 0x2010, Vaddr: 0x201010, Filesz: 0x10, Memsz: 0x10, Align: 0x1000},
	}
	buf := buildELF(t, etDyn, 0x201010, ph, 0)
	r := bytesReader(buf)
	rep, err := AnalyzeReader(r, int64(len(buf)), "test.so", Config{
		LoadBias: 0x100000,
		Reads:    []ReadQuery{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.FileType != "ET_DYN" || rep.LoadBias != 0x100000 {
		t.Fatalf("bad type/bias: %s %#x", rep.FileType, rep.LoadBias)
	}
	if len(rep.WXPages) != 1 || rep.WXPages[0].Start != 0x301000 || rep.WXPages[0].End != 0x302000 {
		t.Fatalf("want single wx page at biased address, got %+v", rep.WXPages)
	}
	if !rep.Entry.Valid || rep.Entry.Address != 0x301010 {
		t.Fatalf("entry audit wrong: %+v", rep.Entry)
	}
}

func TestBiasAlignmentRequirement(t *testing.T) {
	ph := []Prog64{
		{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0, Filesz: 0x10, Memsz: 0x10, Align: 0x1000},
	}
	buf := buildELF(t, etDyn, 0, ph, 0)
	r := bytesReader(buf)
	if _, err := AnalyzeReader(r, int64(len(buf)), "x", Config{LoadBias: 0x10}); err == nil ||
		!strings.Contains(err.Error(), "aligned") {
		t.Fatalf("want alignment error, got %v", err)
	}
}

func TestZeroEntryMeansNoEntry(t *testing.T) {
	ph := []Prog64{{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x10, Memsz: 0x10, Align: 0x1000}}
	buf := buildELF(t, etExec, 0, ph, 0)
	r := bytesReader(buf)
	rep, err := AnalyzeReader(r, int64(len(buf)), "x", Config{Reads: []ReadQuery{}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Entry.Present || !strings.Contains(rep.Entry.Reason, "no entry") {
		t.Fatalf("want no-entry report, got %+v", rep.Entry)
	}
}

func TestCorruptProgramHeaders(t *testing.T) {
	cases := map[string]func(*Prog64){
		"filesz exceeds memsz": func(p *Prog64) { p.Filesz = p.Memsz + 1 },
		"bad alignment":        func(p *Prog64) { p.Align = 3 },
		"noncongruent align":   func(p *Prog64) { p.Off = 0x10; p.Align = 0x1000 },
		"file range outside":   func(p *Prog64) { p.Filesz = 0x10000 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := Prog64{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x10, Memsz: 0x20, Align: 0x1000}
			mutate(&p)
			buf := buildELF(t, etExec, 0x400000, []Prog64{p}, 0)
			r := bytesReader(buf)
			_, err := AnalyzeReader(r, int64(len(buf)), "x", Config{Reads: []ReadQuery{}})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTruncatedHeaderTable(t *testing.T) {
	ph := []Prog64{{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x10, Memsz: 0x10, Align: 0x1000}}
	full := buildELF(t, etExec, 0x400000, ph, 0)
	// Claim 2 program headers but truncate the file before the second one.
	buf := make([]byte, len(full))
	copy(buf, full)
	buf = buf[:ehdrSize64+phdrSize64/2]
	put16(buf[56:], 2)
	r := bytesReader(buf)
	if _, err := AnalyzeReader(r, int64(len(buf)), "x", Config{Reads: []ReadQuery{}}); err == nil {
		t.Fatal("expected out-of-range program header table error")
	}
}

func TestRejectNonOverlapButFileReuse(t *testing.T) {
	// Identical file bytes mapped twice at different VAs is allowed; virtual
	// overlap is not.
	ph := []Prog64{
		{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x10, Memsz: 0x10, Align: 0x1000},
		{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x401000, Filesz: 0x10, Memsz: 0x10, Align: 0x1000},
	}
	buf := buildELF(t, etExec, 0, ph, 0)
	r := bytesReader(buf)
	if _, err := AnalyzeReader(r, int64(len(buf)), "x", Config{Reads: []ReadQuery{}}); err != nil {
		t.Fatalf("file range reuse should be allowed: %v", err)
	}

	ph[1].Vaddr = 0x400008
	ph[1].Off = 0x1008
	buf2 := buildELF(t, etExec, 0, ph, 0)
	if _, err := AnalyzeReader(bytesReader(buf2), int64(len(buf2)), "x", Config{}); err == nil ||
		!strings.Contains(err.Error(), "overlap") {
		t.Fatalf("want virtual overlap error, got %v", err)
	}
}

func TestExecRejectsBias(t *testing.T) {
	ph := []Prog64{{Type: ptLoad, Flags: pfR, Off: 0x1000, Vaddr: 0x400000, Filesz: 0x10, Memsz: 0x10, Align: 0x1000}}
	buf := buildELF(t, etExec, 0, ph, 0)
	_, err := AnalyzeReader(bytesReader(buf), int64(len(buf)), "x", Config{LoadBias: 0x1000})
	if err == nil || (!strings.Contains(err.Error(), "ET_EXEC") && !strings.Contains(err.Error(), "only valid for ET_DYN")) {
		t.Fatalf("want ET_EXEC bias rejection, got %v", err)
	}
}

func TestBadELFIdent(t *testing.T) {
	buf := make([]byte, 128)
	_, err := ParseHeader(bytesReader(buf), int64(len(buf)))
	if err == nil {
		t.Fatal("expected bad magic error")
	}
}
