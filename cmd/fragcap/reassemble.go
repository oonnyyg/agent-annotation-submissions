package main

import (
	"fmt"
	"net/netip"
	"sort"
)

// fragKey identifies one reassembly group.
type fragKey struct {
	Src, Dst netip.Addr
	Proto    uint8
	ID       uint16
}

type fragment struct {
	offset int
	data   []byte
	record int // pcap record index, for anomaly reporting
}

// fragGroup accumulates fragments for one key until end of file.
type fragGroup struct {
	key      fragKey
	frags    []fragment
	total    int  // -1 until a last fragment (MF=0) is seen
	bad      bool // group invalidated (e.g. non-multiple-of-8 non-last fragment)
	badWhy   string
	badRec   int
}

func newFragGroup(key fragKey) *fragGroup {
	return &fragGroup{key: key, total: -1}
}

func (g *fragGroup) add(pkt *ipPacket, record int) {
	if pkt.MoreFrags && len(pkt.Payload)%8 != 0 {
		g.invalidate(fmt.Sprintf("non-final fragment payload length %d is not a multiple of 8", len(pkt.Payload)), record)
		return
	}
	if !pkt.MoreFrags {
		t := pkt.FragOffset + len(pkt.Payload)
		if g.total >= 0 && g.total != t {
			g.invalidate(fmt.Sprintf("conflicting total length %d vs %d from last fragments", g.total, t), record)
			return
		}
		g.total = t
	}
	if g.total >= 0 && pkt.FragOffset+len(pkt.Payload) > g.total {
		g.invalidate(fmt.Sprintf("fragment [%d,%d) exceeds total length %d", pkt.FragOffset, pkt.FragOffset+len(pkt.Payload), g.total), record)
		return
	}
	g.frags = append(g.frags, fragment{offset: pkt.FragOffset, data: pkt.Payload, record: record})
}

func (g *fragGroup) invalidate(why string, record int) {
	if !g.bad {
		g.bad = true
		g.badWhy = why
		g.badRec = record
	}
}

// gap is a missing byte range [start, end).
type gap struct {
	Start, End int
}

// finalize merges fragments after the whole file has been read.
// Returns the reassembled payload, or an error describing why the
// datagram cannot be confirmed complete. Gaps are returned via gapsOut.
func (g *fragGroup) finalize() (payload []byte, gaps []gap, err error) {
	if g.bad {
		return nil, nil, fmt.Errorf("record %d: %s", g.badRec, g.badWhy)
	}
	if g.total < 0 {
		return nil, nil, errLastFragmentMissing
	}
	buf := make([]byte, g.total)
	filled := make([]bool, g.total)
	frags := append([]fragment(nil), g.frags...)
	sort.SliceStable(frags, func(i, j int) bool { return frags[i].offset < frags[j].offset })
	for _, f := range frags {
		for i, b := range f.data {
			pos := f.offset + i
			if filled[pos] {
				if buf[pos] != b {
					return nil, nil, fmt.Errorf("record %d: conflicting overlapping byte at offset %d", f.record, pos)
				}
				continue // identical duplicate byte: count once
			}
			buf[pos] = b
			filled[pos] = true
		}
	}
	for i := 0; i < g.total; {
		if filled[i] {
			i++
			continue
		}
		j := i
		for j < g.total && !filled[j] {
			j++
		}
		gaps = append(gaps, gap{Start: i, End: j})
		i = j
	}
	if len(gaps) > 0 {
		return nil, gaps, errIncomplete
	}
	return buf, nil, nil
}

var (
	errIncomplete          = fmt.Errorf("missing fragments")
	errLastFragmentMissing = fmt.Errorf("last fragment (MF=0) never seen; total length unknown")
)
