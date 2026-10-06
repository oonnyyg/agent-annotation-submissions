package elfmap

import (
	"fmt"
	"sort"
)

// PageSize is the fixed x86-64 page granularity used for permission merging.
const PageSize uint64 = 4096

// PageRange describes a run of pages with identical effective permissions.
type PageRange struct {
	Start uint64 // inclusive page-aligned virtual address
	End   uint64 // exclusive page-aligned virtual address
	R     bool
	W     bool
	X     bool
}

// PermString renders flags in rwx order, '-' for missing bits.
func (p PageRange) PermString() string {
	b := [3]byte{'-', '-', '-'}
	if p.R {
		b[0] = 'r'
	}
	if p.W {
		b[1] = 'w'
	}
	if p.X {
		b[2] = 'x'
	}
	return string(b[:])
}

// Pages merges the permissions of all segments intersecting each 4096-byte
// page and returns coalesced runs. Pages touched by multiple segments get the
// union of their permissions.
func (m *LoadMap) Pages() ([]PageRange, error) {
	type span struct {
		start, end uint64
		flags      uint32
	}
	spans := make([]span, 0, len(m.Segments))
	for _, s := range m.Segments {
		if s.Memsz == 0 {
			continue
		}
		end, of := s.MemEnd()
		if of {
			return nil, fmt.Errorf("segment %d end overflows", s.Index)
		}
		startPage := roundDown(s.Vaddr, PageSize)
		endPage := roundUp(end, PageSize)
		spans = append(spans, span{startPage, endPage, s.Flags})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	type ev struct {
		at    uint64
		flags uint32
		open  bool
	}
	var events []ev
	limit := uint64(0)
	for _, sp := range spans {
		events = append(events, ev{sp.start, sp.flags, true}, ev{sp.end, sp.flags, false})
		if sp.end > limit {
			limit = sp.end
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })

	var out []PageRange
	active := map[uint32]int{}
	ei := 0
	page := events[0].at
	for page < limit {
		for ei < len(events) && events[ei].at <= page {
			if events[ei].open {
				active[events[ei].flags]++
			} else {
				active[events[ei].flags]--
			}
			ei++
		}
		next, carry := addOverflow(page, PageSize)
		if carry {
			return nil, fmt.Errorf("page range reaches top of 64-bit address space")
		}
		var r, w, x bool
		for flags, n := range active {
			if n > 0 {
				r = r || flags&pfR != 0
				w = w || flags&pfW != 0
				x = x || flags&pfX != 0
			}
		}
		if n := len(out); n > 0 && out[n-1].End == page &&
			out[n-1].R == r && out[n-1].W == w && out[n-1].X == x {
			out[n-1].End = next
		} else {
			out = append(out, PageRange{Start: page, End: next, R: r, W: w, X: x})
		}
		page = next
	}
	return out, nil
}

// WXPages returns page ranges that are simultaneously writable and
// executable, which is the suspicious-permission condition.
func (m *LoadMap) WXPages() ([]PageRange, error) {
	pages, err := m.Pages()
	if err != nil {
		return nil, err
	}
	var wx []PageRange
	for _, p := range pages {
		if p.W && p.X {
			wx = append(wx, p)
		}
	}
	return wx, nil
}

// EntryStatus reports the audit conclusion for e_entry.
type EntryStatus struct {
	Address uint64
	NoEntry bool // e_entry was zero
	Valid   bool // entry lies inside a file-backed executable segment
	Segment int  // index of the containing segment, -1 when invalid
}

// AuditEntry checks that a nonzero entry belongs to an actual executable
// (PF_X) segment's populated virtual range.
func (m *LoadMap) AuditEntry() (EntryStatus, error) {
	st := EntryStatus{Address: m.Entry, Segment: -1}
	if m.Entry == 0 {
		st.NoEntry = true
		return st, nil
	}
	for _, s := range m.Segments {
		if s.Flags&pfX == 0 || s.Filesz == 0 {
			continue
		}
		fileEnd, of := s.FileEnd()
		if of {
			return st, fmt.Errorf("segment %d file end overflows", s.Index)
		}
		if m.Entry >= s.Vaddr && m.Entry < fileEnd {
			st.Valid = true
			st.Segment = s.Index
			return st, nil
		}
	}
	return st, nil
}

func roundDown(v, align uint64) uint64 {
	return v &^ (align - 1)
}

func roundUp(v, align uint64) uint64 {
	if v == 0 {
		return 0
	}
	r, carry := addOverflow(v, align-1)
	if carry {
		return ^uint64(0) &^ (align - 1)
	}
	return r &^ (align - 1)
}
