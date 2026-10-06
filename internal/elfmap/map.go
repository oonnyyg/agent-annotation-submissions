package elfmap

import (
	"fmt"
	"sort"
)

// Segment is a validated PT_LOAD segment after relocation.
type Segment struct {
	Index   int
	Flags   uint32
	FileOff uint64
	Vaddr   uint64 // relocated virtual start
	Filesz  uint64
	Memsz   uint64
	Align   uint64
}

// FileEnd returns the first address after file-backed bytes.
func (s Segment) FileEnd() (uint64, bool) {
	return addOverflow(s.Vaddr, s.Filesz)
}

// MemEnd returns the first address after the whole segment.
func (s Segment) MemEnd() (uint64, bool) {
	return addOverflow(s.Vaddr, s.Memsz)
}

// LoadMap is the validated result of mapping PT_LOAD segments.
type LoadMap struct {
	Type     uint16 // ET_EXEC or ET_DYN
	Entry    uint64 // relocated entry; 0 means no usable entry
	Bias     uint64
	Segments []Segment
}

// Build validates PT_LOAD segments of an ET_EXEC/ET_DYN file and constructs
// the virtual load map. For ET_EXEC bias must be zero; for ET_DYN bias comes
// from configuration and must satisfy every nontrivial segment alignment.
func Build(h *Header64, phdrs []Prog64, fileSize int64, bias uint64) (*LoadMap, error) {
	if h.Type == etExec && bias != 0 {
		return nil, fmt.Errorf("ET_EXEC must load at offset 0, got %#x", bias)
	}

	segs := make([]Segment, 0, len(phdrs))
	for i, p := range phdrs {
		if p.Type != ptLoad {
			continue
		}
		if p.Filesz > p.Memsz {
			return nil, fmt.Errorf("segment %d: filesz %d > memsz %d", i, p.Filesz, p.Memsz)
		}
		if !validAlign(p.Align) {
			return nil, fmt.Errorf("segment %d: alignment %#x is not 0, 1, or a power of two", i, p.Align)
		}
		// File range checks. A zero-length segment occupies no file space.
		if p.Filesz != 0 {
			fileEnd, of1 := addOverflow(p.Off, p.Filesz)
			if of1 || fileEnd > uint64(fileSize) {
				return nil, fmt.Errorf("segment %d: file range [%#x,%#x) outside file of size %d",
					i, p.Off, fileEnd, fileSize)
			}
		}
		if p.Align > 1 {
			if p.Off%p.Align != p.Vaddr%p.Align {
				return nil, fmt.Errorf("segment %d: offset %#x not congruent to vaddr %#x mod %#x",
					i, p.Off, p.Vaddr, p.Align)
			}
			if bias%p.Align != 0 {
				return nil, fmt.Errorf("load bias %#x not aligned to segment %d alignment %#x",
					bias, i, p.Align)
			}
		}
		vaddr, vof := addOverflow(p.Vaddr, bias)
		if _, eof := addOverflow(vaddr, p.Memsz); vof || eof {
			return nil, fmt.Errorf("segment %d: virtual range overflows 64-bit address space", i)
		}
		segs = append(segs, Segment{
			Index:   i,
			Flags:   p.Flags,
			FileOff: p.Off,
			Vaddr:   vaddr,
			Filesz:  p.Filesz,
			Memsz:   p.Memsz,
			Align:   p.Align,
		})
	}

	// Reject overlap of non-empty virtual memory ranges. File ranges may be
	// freely reused, so only virtual ranges are checked.
	sorted := make([]Segment, len(segs))
	copy(sorted, segs)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Vaddr < sorted[b].Vaddr })
	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1]
		cur := sorted[i]
		if prev.Memsz == 0 || cur.Memsz == 0 {
			continue
		}
		prevEnd, _ := prev.MemEnd()
		if cur.Vaddr < prevEnd {
			return nil, fmt.Errorf("virtual memory overlap: segment %d [%#x,%#x) with segment %d at %#x",
				prev.Index, prev.Vaddr, prevEnd, cur.Index, cur.Vaddr)
		}
	}

	entry := uint64(0)
	if h.Entry != 0 {
		entryEntry, of := addOverflow(h.Entry, bias)
		if of {
			return nil, fmt.Errorf("entry point %#x plus bias overflows", h.Entry)
		}
		entry = entryEntry
	}

	return &LoadMap{
		Type:     h.Type,
		Entry:    entry,
		Bias:     bias,
		Segments: segs,
	}, nil
}

func validAlign(a uint64) bool {
	return a == 0 || a == 1 || (a&(a-1)) == 0
}
