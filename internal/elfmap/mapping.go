package elfmap

import (
	"debug/elf"
	"fmt"
	"math"
	"os"
	"sort"
)

const maxReadLen = 65536

type segment struct {
	index  int
	flags  elf.ProgFlag
	off    uint64
	vaddr  uint64 // already biased
	filesz uint64
	memsz  uint64
	align  uint64
}

func (s *segment) end() uint64 { return s.vaddr + s.memsz }

func flagString(f elf.ProgFlag) string {
	out := []byte{'-', '-', '-'}
	if f&elf.PF_R != 0 {
		out[0] = 'r'
	}
	if f&elf.PF_W != 0 {
		out[1] = 'w'
	}
	if f&elf.PF_X != 0 {
		out[2] = 'x'
	}
	return string(out)
}

func addOverflow(a, b uint64) bool {
	return a > math.MaxUint64-b
}

// loadSegments parses the ELF and returns validated, biased PT_LOAD segments.
func loadSegments(path string, bias uint64) (*elf.File, []segment, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open ELF: %w", err)
	}
	segs, err := validateSegments(f, path, bias)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, segs, nil
}

func validateSegments(f *elf.File, path string, bias uint64) ([]segment, error) {
	if f.Class != elf.ELFCLASS64 {
		return nil, fmt.Errorf("unsupported class %s: only ELF64", f.Class)
	}
	if f.Data != elf.ELFDATA2LSB {
		return nil, fmt.Errorf("unsupported endianness %s: only little-endian", f.Data)
	}
	if f.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("unsupported machine %s: only x86-64", f.Machine)
	}
	if f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN {
		return nil, fmt.Errorf("unsupported type %s: only executables and shared objects", f.Type)
	}
	if f.Type == elf.ET_EXEC && bias != 0 {
		return nil, fmt.Errorf("ET_EXEC requires load_bias 0, got 0x%x", bias)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	fileSize := uint64(st.Size())

	var segs []segment
	for i, p := range f.Progs {
		if p.Type != elf.PT_LOAD {
			continue
		}
		ph := p.ProgHeader
		if ph.Align != 0 && ph.Align != 1 && ph.Align&(ph.Align-1) != 0 {
			return nil, fmt.Errorf("segment %d: align 0x%x is not 0, 1, or a power of two", i, ph.Align)
		}
		if ph.Filesz > ph.Memsz {
			return nil, fmt.Errorf("segment %d: file size 0x%x exceeds mem size 0x%x", i, ph.Filesz, ph.Memsz)
		}
		if addOverflow(ph.Off, ph.Filesz) {
			return nil, fmt.Errorf("segment %d: file range overflows 64 bits", i)
		}
		if ph.Off+ph.Filesz > fileSize {
			return nil, fmt.Errorf("segment %d: file range [0x%x, 0x%x) exceeds file size 0x%x",
				i, ph.Off, ph.Off+ph.Filesz, fileSize)
		}
		if addOverflow(ph.Vaddr, ph.Memsz) {
			return nil, fmt.Errorf("segment %d: virtual range overflows 64 bits", i)
		}
		if ph.Align > 1 && ph.Off%ph.Align != ph.Vaddr%ph.Align {
			return nil, fmt.Errorf("segment %d: file offset 0x%x and vaddr 0x%x not congruent mod align 0x%x",
				i, ph.Off, ph.Vaddr, ph.Align)
		}
		if ph.Memsz == 0 {
			continue // zero-length segments occupy no space
		}
		if addOverflow(ph.Vaddr, bias) {
			return nil, fmt.Errorf("segment %d: vaddr 0x%x + bias 0x%x overflows", i, ph.Vaddr, bias)
		}
		vaddr := ph.Vaddr + bias
		if addOverflow(vaddr, ph.Memsz) {
			return nil, fmt.Errorf("segment %d: biased virtual range overflows 64 bits", i)
		}
		if f.Type == elf.ET_DYN && ph.Align > 1 && bias%ph.Align != 0 {
			return nil, fmt.Errorf("segment %d: load bias 0x%x violates alignment 0x%x", i, bias, ph.Align)
		}
		segs = append(segs, segment{
			index: i, flags: ph.Flags, off: ph.Off,
			vaddr: vaddr, filesz: ph.Filesz, memsz: ph.Memsz, align: ph.Align,
		})
	}
	sort.Slice(segs, func(a, b int) bool { return segs[a].vaddr < segs[b].vaddr })
	for i := 1; i < len(segs); i++ {
		if segs[i].vaddr < segs[i-1].end() {
			return nil, fmt.Errorf("segments %d and %d overlap in virtual memory",
				segs[i-1].index, segs[i].index)
		}
	}
	return segs, nil
}
