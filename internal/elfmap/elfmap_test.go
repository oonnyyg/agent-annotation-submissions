package elfmap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

type testPhdr struct {
	typ           uint32
	flags         uint32
	off, vaddr    uint64
	filesz, memsz uint64
	align         uint64
}

// buildELF writes a minimal little-endian ELF64 x86-64 file with the given
// type, entry, program headers, and raw content placed at given offsets.
func buildELF(t *testing.T, elfType uint16, entry uint64, phdrs []testPhdr, content map[uint64][]byte) string {
	t.Helper()
	const ehdrSize = 64
	const phdrSize = 56
	size := uint64(ehdrSize + phdrSize*len(phdrs))
	for off, b := range content {
		if off+uint64(len(b)) > size {
			size = off + uint64(len(b))
		}
	}
	buf := make([]byte, size)
	copy(buf[0:4], []byte{0x7f, 'E', 'L', 'F'})
	buf[4] = 2 // ELFCLASS64
	buf[5] = 1 // little-endian
	buf[6] = 1 // version
	binary.LittleEndian.PutUint16(buf[16:], elfType)
	binary.LittleEndian.PutUint16(buf[18:], 62) // EM_X86_64
	binary.LittleEndian.PutUint32(buf[20:], 1)  // version
	binary.LittleEndian.PutUint64(buf[24:], entry)
	binary.LittleEndian.PutUint64(buf[32:], ehdrSize) // phoff
	binary.LittleEndian.PutUint16(buf[52:], ehdrSize)
	binary.LittleEndian.PutUint16(buf[54:], phdrSize)
	binary.LittleEndian.PutUint16(buf[56:], uint16(len(phdrs)))
	for i, p := range phdrs {
		base := ehdrSize + phdrSize*i
		binary.LittleEndian.PutUint32(buf[base:], p.typ)
		binary.LittleEndian.PutUint32(buf[base+4:], p.flags)
		binary.LittleEndian.PutUint64(buf[base+8:], p.off)
		binary.LittleEndian.PutUint64(buf[base+16:], p.vaddr)
		binary.LittleEndian.PutUint64(buf[base+24:], p.vaddr)
		binary.LittleEndian.PutUint64(buf[base+32:], p.filesz)
		binary.LittleEndian.PutUint64(buf[base+40:], p.memsz)
		binary.LittleEndian.PutUint64(buf[base+48:], p.align)
	}
	for off, b := range content {
		copy(buf[off:], b)
	}
	path := filepath.Join(t.TempDir(), "test.elf")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	ptLoad = 1
	pfX    = 1
	pfW    = 2
	pfR    = 4
	etExec = 2
	etDyn  = 3
)

func TestCrossSegmentRead(t *testing.T) {
	// seg0: vaddr 0x1000, 4 file bytes + 4 zero-fill; seg1 adjacent at 0x1008.
	path := buildELF(t, etExec, 0x1000, []testPhdr{
		{typ: ptLoad, flags: pfR | pfX, off: 0x2000, vaddr: 0x1000, filesz: 4, memsz: 8, align: 0x1000},
		{typ: ptLoad, flags: pfR, off: 0x3000, vaddr: 0x1008, filesz: 4, memsz: 4, align: 1},
	}, map[uint64][]byte{
		0x2000: {0xde, 0xad, 0xbe, 0xef},
		0x3000: {0x01, 0x02, 0x03, 0x04},
	})
	rep, err := Run(&Query{ELFPath: path, Reads: []ReadQuery{
		{Vaddr: 0x1000, Length: 12},
		{Vaddr: 0x1004, Length: 6},
		{Vaddr: 0x1800, Length: 4}, // hole: must fail
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.ReadResults[0]
	if !r.OK || r.Data != "deadbeef0000000001020304" {
		t.Fatalf("cross-segment read: %+v", r)
	}
	if len(r.Regions) != 3 || r.Regions[1].Source != "zero" {
		t.Fatalf("regions: %+v", r.Regions)
	}
	if rep.ReadResults[2].OK {
		t.Fatal("read into unmapped hole should fail")
	}
	if rep.EntryStatus != "ok" {
		t.Fatalf("entry status: %s", rep.EntryStatus)
	}
}

func TestSharedPagePermissions(t *testing.T) {
	// RX segment and RW segment share page 0x2000 -> merged rwx, reported as W+X.
	path := buildELF(t, etExec, 0, []testPhdr{
		{typ: ptLoad, flags: pfR | pfX, off: 0x200, vaddr: 0x2000, filesz: 0x10, memsz: 0x10, align: 1},
		{typ: ptLoad, flags: pfR | pfW, off: 0x300, vaddr: 0x2f00, filesz: 0x10, memsz: 0x10, align: 1},
	}, map[uint64][]byte{0x300: make([]byte, 0x10)})
	rep, err := Run(&Query{ELFPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.WXPages) != 1 || rep.WXPages[0] != 0x2000 {
		t.Fatalf("wx pages: %v", rep.WXPages)
	}
	var found bool
	for _, p := range rep.Pages {
		if p.Page == 0x2000 {
			found = true
			if p.Flags != "rwx" {
				t.Fatalf("merged flags: %s", p.Flags)
			}
		}
	}
	if !found {
		t.Fatal("page 0x2000 missing")
	}
	if rep.EntryStatus != "no_entry" {
		t.Fatalf("entry status: %s", rep.EntryStatus)
	}
}

func TestCorruptProgramHeaders(t *testing.T) {
	cases := []struct {
		name  string
		phdrs []testPhdr
	}{
		{"filesz_gt_memsz", []testPhdr{{typ: ptLoad, flags: pfR, off: 0x200, vaddr: 0x1000, filesz: 8, memsz: 4, align: 1}}},
		{"bad_align", []testPhdr{{typ: ptLoad, flags: pfR, off: 0x200, vaddr: 0x1000, filesz: 4, memsz: 4, align: 3}}},
		{"not_congruent", []testPhdr{{typ: ptLoad, flags: pfR, off: 0x204, vaddr: 0x1000, filesz: 4, memsz: 4, align: 0x100}}},
		{"overlap", []testPhdr{
			{typ: ptLoad, flags: pfR, off: 0x200, vaddr: 0x1000, filesz: 0x100, memsz: 0x100, align: 1},
			{typ: ptLoad, flags: pfR, off: 0x400, vaddr: 0x1080, filesz: 0x100, memsz: 0x100, align: 1},
		}},
		{"vaddr_overflow", []testPhdr{{typ: ptLoad, flags: pfR, off: 0x200, vaddr: ^uint64(0) - 3, filesz: 4, memsz: 8, align: 1}}},
		{"file_range_past_eof", []testPhdr{{typ: ptLoad, flags: pfR, off: 0x8000, vaddr: 0x1000, filesz: 0x100, memsz: 0x100, align: 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := buildELF(t, etExec, 0, tc.phdrs, nil)
			if _, err := Run(&Query{ELFPath: path}); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestDynBiasAndZeroLength(t *testing.T) {
	path := buildELF(t, etDyn, 0x1000, []testPhdr{
		{typ: ptLoad, flags: pfR | pfX, off: 0x2000, vaddr: 0x1000, filesz: 4, memsz: 4, align: 0x1000},
		{typ: ptLoad, flags: pfR, off: 0x204, vaddr: 0x2000, filesz: 0, memsz: 0, align: 1},
	}, map[uint64][]byte{0x2000: {0xaa, 0xbb, 0xcc, 0xdd}})
	// Misaligned bias must fail.
	if _, err := Run(&Query{ELFPath: path, LoadBias: 0x800}); err == nil {
		t.Fatal("misaligned bias should fail")
	}
	rep, err := Run(&Query{ELFPath: path, LoadBias: 0x400000, Reads: []ReadQuery{{Vaddr: 0x401000, Length: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Segments) != 1 {
		t.Fatalf("zero-length segment should occupy no space: %d segments", len(rep.Segments))
	}
	if !rep.ReadResults[0].OK || rep.ReadResults[0].Data != "aabbccdd" {
		t.Fatalf("biased read: %+v", rep.ReadResults[0])
	}
	if rep.Entry != 0x401000 || rep.EntryStatus != "ok" {
		t.Fatalf("entry: %x %s", uint64(rep.Entry), rep.EntryStatus)
	}
}

func TestExecBiasRejected(t *testing.T) {
	path := buildELF(t, etExec, 0, []testPhdr{
		{typ: ptLoad, flags: pfR, off: 0x200, vaddr: 0x1000, filesz: 4, memsz: 4, align: 1},
	}, nil)
	if _, err := Run(&Query{ELFPath: path, LoadBias: 0x1000}); err == nil {
		t.Fatal("ET_EXEC with nonzero bias should fail")
	}
}
