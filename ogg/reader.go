package ogg

import (
	"errors"
	"fmt"
	"io"
)

// StreamResult collects the packets of one logical stream generation.
type StreamResult struct {
	Serial      uint32
	Group       int // 0-based group index, incremented when a serial is reused
	PacketCount int
}

// Audit reads an entire Ogg file from r, validating every page and
// reassembling packets per logical stream. Completed packets are
// delivered through cb exactly once and are not retained, so arbitrarily
// large files can be processed in bounded memory. The callback may return
// an error to abort. The whole file must validate; corruption is reported
// with its file offset and never silently skipped.
func Audit(r io.Reader, maxPacketSize int64, cb func(s *StreamResult, p *Packet) error) ([]*StreamResult, error) {
	streams := map[uint32]*Stream{}
	current := map[uint32]*StreamResult{}
	var results []*StreamResult
	var order []uint32
	groups := map[uint32]int{}

	off := int64(0)
	for {
		p, err := ReadPage(r, off)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		off = p.EndOffset()

		st, ok := streams[p.Serial]
		if !ok {
			order = append(order, p.Serial)
			st = &Stream{Serial: p.Serial, MaxPacketSize: maxPacketSize}
			streams[p.Serial] = st
			current[p.Serial] = &StreamResult{Serial: p.Serial, Group: 0}
			results = append(results, current[p.Serial])
		} else if st.ended {
			// A previous group ended; a new group may only begin with a
			// fresh BOS page for this serial.
			if p.HeaderType&FlagBOS == 0 {
				return nil, &StreamError{Offset: p.Offset, Serial: p.Serial,
					Msg: "stream already ended; new group must start with a BOS page"}
			}
			groups[p.Serial]++
			st = &Stream{Serial: p.Serial, MaxPacketSize: maxPacketSize}
			streams[p.Serial] = st
			current[p.Serial] = &StreamResult{Serial: p.Serial, Group: groups[p.Serial]}
			results = append(results, current[p.Serial])
		}

		pkts, err := st.Feed(p)
		if err != nil {
			return nil, err
		}
		res := current[p.Serial]
		for _, pkt := range pkts {
			res.PacketCount++
			if cb != nil {
				if err := cb(res, pkt); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, serial := range order {
		if err := streams[serial].Finish(); err != nil {
			return nil, err
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("ogg: empty file: no pages found")
	}
	return results, nil
}
