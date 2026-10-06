package elfmap

import (
	"debug/elf"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
)

const pageSize = 4096

// Run executes the full audit and builds the report.
func Run(q *Query) (*Report, error) {
	if q.ELFPath == "" {
		return nil, fmt.Errorf("query missing elf_path")
	}
	f, segs, err := loadSegments(q.ELFPath, uint64(q.LoadBias))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rep := &Report{
		ELFPath:  q.ELFPath,
		Type:     f.Type.String(),
		Machine:  f.Machine.String(),
		LoadBias: q.LoadBias,
		Entry:    HexU64(uint64(f.Entry) + uint64(q.LoadBias)),
	}
	for _, s := range segs {
		rep.Segments = append(rep.Segments, SegmentReport{
			Index: s.index, Flags: flagString(s.flags),
			Off: HexU64(s.off), Vaddr: HexU64(s.vaddr),
			Filesz: HexU64(s.filesz), Memsz: HexU64(s.memsz), Align: HexU64(s.align),
		})
	}
	rep.Pages, rep.WXPages = mergePages(segs)
	rep.EntryStatus = entryStatus(f, segs, uint64(rep.Entry))

	data, err := os.ReadFile(q.ELFPath)
	if err != nil {
		return nil, err
	}
	for _, rq := range q.Reads {
		rep.ReadResults = append(rep.ReadResults, doRead(segs, data, rq))
	}
	return rep, nil
}

func entryStatus(f *elf.File, segs []segment, entry uint64) string {
	if f.Entry == 0 {
		return "no_entry"
	}
	for _, s := range segs {
		if s.flags&elf.PF_X != 0 && entry >= s.vaddr && entry < s.end() {
			return "ok"
		}
	}
	return "entry_not_in_executable_segment"
}

// mergePages computes per-page permissions over 4096-byte pages, OR-ing
// flags of all segments intersecting each page.
func mergePages(segs []segment) ([]PageReport, []HexU64) {
	perms := map[uint64]elf.ProgFlag{}
	for _, s := range segs {
		start := s.vaddr / pageSize * pageSize
		end := (s.end() + pageSize - 1) / pageSize * pageSize
		for p := start; p < end; p += pageSize {
			perms[p] |= s.flags
		}
	}
	var pages []uint64
	for p := range perms {
		pages = append(pages, p)
	}
	sort.Slice(pages, func(a, b int) bool { return pages[a] < pages[b] })
	reps := []PageReport{}
	wx := []HexU64{}
	for _, p := range pages {
		fl := perms[p]
		reps = append(reps, PageReport{Page: HexU64(p), Flags: flagString(fl)})
		if fl&elf.PF_W != 0 && fl&elf.PF_X != 0 {
			wx = append(wx, HexU64(p))
		}
	}
	return reps, wx
}

// doRead reads up to maxReadLen bytes across adjacent segments and zero-fill
// regions without materializing the full virtual space.
func doRead(segs []segment, data []byte, rq ReadQuery) ReadResult {
	res := ReadResult{Vaddr: rq.Vaddr, Length: rq.Length}
	if rq.Length == 0 || rq.Length > maxReadLen {
		res.Error = fmt.Sprintf("length must be 1..%d", maxReadLen)
		return res
	}
	if addOverflow(uint64(rq.Vaddr), rq.Length) {
		res.Error = "address range overflows 64 bits"
		return res
	}
	addr := uint64(rq.Vaddr)
	remaining := rq.Length
	var out []byte
	for remaining > 0 {
		seg := findSegment(segs, addr)
		if seg == nil {
			res.Error = fmt.Sprintf("unmapped address 0x%x", addr)
			return res
		}
		avail := seg.end() - addr
		n := avail
		if n > remaining {
			n = remaining
		}
		offInSeg := addr - seg.vaddr
		if offInSeg < seg.filesz {
			fileN := seg.filesz - offInSeg
			if fileN > n {
				fileN = n
			}
			foff := seg.off + offInSeg
			if foff+fileN > uint64(len(data)) {
				res.Error = fmt.Sprintf("segment %d file range extends past end of file", seg.index)
				return res
			}
			out = append(out, data[foff:foff+fileN]...)
			res.Regions = append(res.Regions, ByteSource{Vaddr: HexU64(addr), Length: HexU64(fileN), Source: "file"})
			if fileN < n {
				res.Regions = append(res.Regions, ByteSource{Vaddr: HexU64(addr + fileN), Length: HexU64(n - fileN), Source: "zero"})
				out = append(out, make([]byte, n-fileN)...)
			}
		} else {
			res.Regions = append(res.Regions, ByteSource{Vaddr: HexU64(addr), Length: HexU64(n), Source: "zero"})
			out = append(out, make([]byte, n)...)
		}
		addr += n
		remaining -= n
	}
	res.OK = true
	res.Data = hex.EncodeToString(out)
	return res
}

func findSegment(segs []segment, addr uint64) *segment {
	i := sort.Search(len(segs), func(i int) bool { return segs[i].end() > addr })
	if i < len(segs) && segs[i].vaddr <= addr {
		return &segs[i]
	}
	return nil
}
