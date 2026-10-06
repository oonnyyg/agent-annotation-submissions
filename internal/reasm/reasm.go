// Package reasm reassembles IPv4 fragments into UDP datagrams.
package reasm

import (
	"fmt"
	"sort"

	"example.com/pcap-fragment-audit/internal/parse"
)

// Key identifies one fragment group (protocol is always UDP here).
type Key struct {
	Src   [4]byte
	Dst   [4]byte
	Ident uint16
}

func (k Key) String() string {
	return fmt.Sprintf("%d.%d.%d.%d>%d.%d.%d.%d id=%d",
		k.Src[0], k.Src[1], k.Src[2], k.Src[3],
		k.Dst[0], k.Dst[1], k.Dst[2], k.Dst[3], k.Ident)
}

type segment struct {
	off  int
	data []byte
}

// Group accumulates fragments for one Key.
type Group struct {
	Key       Key
	segs      []segment
	total     int  // -1 until the last fragment (MF=0) is seen
	conflict  bool // overlapping bytes disagree or total length conflicts
	truncated bool // a captured fragment was cut short
	records   []int
}

func newGroup(k Key) *Group { return &Group{Key: k, total: -1} }

// Add feeds one decoded fragment into the group.
func (g *Group) Add(p *parse.Packet, record int) {
	g.records = append(g.records, record)
	if p.Truncated {
		g.truncated = true
	}
	end := p.FragOff + len(p.Payload)
	if p.MoreFrag {
		if len(p.Payload)%8 != 0 {
			g.conflict = true // non-last fragment payload must be 8-byte aligned
		}
	} else {
		if g.total >= 0 && g.total != end {
			g.conflict = true // disagreeing last fragments
		}
		g.total = end
	}
	if g.total >= 0 && end > g.total {
		g.conflict = true // content beyond the declared total length
	}
	for _, s := range g.segs {
		lo := max(p.FragOff, s.off)
		hi := min(end, s.off+len(s.data))
		if lo < hi {
			a := p.Payload[lo-p.FragOff : hi-p.FragOff]
			b := s.data[lo-s.off : hi-s.off]
			for i := range a {
				if a[i] != b[i] {
					g.conflict = true // overlapping bytes differ
					break
				}
			}
		}
	}
	g.segs = append(g.segs, segment{off: p.FragOff, data: append([]byte(nil), p.Payload...)})
}

// Gap is a missing byte range [Start, End).
type Gap struct {
	Start int `json:"start"`
	End   int `json:"end"` // -1 when the total length is unknown
}

// Result is the final verdict for a group.
type Result struct {
	Key       Key
	Complete  bool
	UDP       []byte // reassembled datagram when Complete
	Gaps      []Gap
	LastSeen  bool // a last fragment (MF=0) was observed
	Conflict  bool
	Truncated bool
	Records   []int
}

// Finalize decides the outcome after the whole capture has been read.
func (g *Group) Finalize() *Result {
	r := &Result{
		LastSeen:  g.total >= 0,
		Conflict:  g.conflict,
		Truncated: g.truncated,
		Records:   g.records,
	}
	if g.conflict || g.truncated {
		return r
	}
	segs := append([]segment(nil), g.segs...)
	sort.Slice(segs, func(i, j int) bool { return segs[i].off < segs[j].off })
	covered := 0
	limit := g.total
	if limit < 0 {
		for _, s := range segs {
			limit = max(limit, s.off+len(s.data))
		}
	}
	for _, s := range segs {
		if s.off > covered {
			end := s.off
			if g.total < 0 {
				end = -1
			}
			r.Gaps = append(r.Gaps, Gap{Start: covered, End: end})
		}
		covered = max(covered, min(s.off+len(s.data), limit))
	}
	if g.total >= 0 && covered < g.total {
		r.Gaps = append(r.Gaps, Gap{Start: covered, End: g.total})
	}
	if g.total < 0 && len(segs) > 0 {
		r.Gaps = append(r.Gaps, Gap{Start: covered, End: -1})
	}
	if len(r.Gaps) > 0 || g.total < 0 {
		return r
	}
	buf := make([]byte, g.total)
	for _, s := range segs {
		copy(buf[s.off:], s.data)
	}
	r.UDP = buf
	r.Complete = true
	return r
}

// Reassembler holds all in-flight groups.
type Reassembler struct {
	Groups map[Key]*Group
	order  []Key
}

// New creates an empty Reassembler.
func New() *Reassembler {
	return &Reassembler{Groups: make(map[Key]*Group)}
}

// Add routes a decoded packet to its group.
func (r *Reassembler) Add(p *parse.Packet, record int) {
	k := Key{Src: p.SrcIP, Dst: p.DstIP, Ident: p.Ident}
	g, ok := r.Groups[k]
	if !ok {
		g = newGroup(k)
		r.Groups[k] = g
		r.order = append(r.order, k)
	}
	g.Add(p, record)
}

// Finalize returns results in first-seen order.
func (r *Reassembler) Finalize() []*Result {
	out := make([]*Result, 0, len(r.order))
	for _, k := range r.order {
		res := r.Groups[k].Finalize()
		res.Key = k
		res.Records = r.Groups[k].records
		out = append(out, res)
	}
	return out
}
