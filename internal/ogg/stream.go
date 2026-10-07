package ogg

import (
	"bytes"
	"fmt"
	"io"
)

// PageRef identifies a page by file offset and total on-disk length.
type PageRef struct {
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

// Packet is a reassembled logical packet.
type Packet struct {
	Serial     uint32
	Index      uint64 // packet index within its logical stream
	Data       []byte // raw packet bytes, length bounded by MaxPacketSize
	GranulePos uint64 // granule position of the page the packet ends on, as-is
	Pages      []PageRef
	EndOffset  int64 // file offset just past the last contributing page
}

// AuditOptions configures an Audit run.
type AuditOptions struct {
	// MaxPacketSize rejects packets larger than this many bytes.
	// Zero uses DefaultMaxPacketSize.
	MaxPacketSize int64
	// OnPage, if set, is invoked for each validated page before its
	// packets are emitted.
	OnPage func(*Page)
}

// DefaultMaxPacketSize is the default per-packet size cap (64 MiB).
const DefaultMaxPacketSize = 64 << 20

// PacketHandler receives each completed packet. The Data slice is reused
// after the call returns; copy it if it must be retained.
type PacketHandler func(*Packet) error

type streamState struct {
	serial    uint32
	bosSeen   bool
	ended     bool
	expectSeq uint32
	pending   bytes.Buffer
	pages     []PageRef
	pktIndex  uint64
}

// Audit reads pages from rd, validates per-stream ordering and grouping,
// reassembles packets and invokes handle for each. The whole file must
// validate; any error is fatal and includes a file offset.
func Audit(rd *Reader, opts AuditOptions, handle PacketHandler) error {
	maxPkt := opts.MaxPacketSize
	if maxPkt <= 0 {
		maxPkt = DefaultMaxPacketSize
	}
	streams := map[uint32]*streamState{}
	var order []*streamState
	groupOpen := false    // current group has active (non-ended) streams
	groupHasData := false // current group has seen non-BOS pages

	finishPacket := func(s *streamState, p *Page, pageLen int64) error {
		pkt := &Packet{
			Serial:     s.serial,
			Index:      s.pktIndex,
			Data:       append([]byte(nil), s.pending.Bytes()...),
			GranulePos: p.GranulePos,
			Pages:      append([]PageRef(nil), s.pages...),
			EndOffset:  p.Offset + pageLen,
		}
		s.pending.Reset()
		s.pages = s.pages[:0]
		s.pktIndex++
		return handle(pkt)
	}

	for {
		page, err := rd.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		pageLen := int64(headerSize + len(page.SegmentTable) + len(page.Data))

		s, known := streams[page.Serial]
		if !known {
			if !page.BOS() {
				return fmt.Errorf("offset %d: stream 0x%08x begins without BOS flag", page.Offset, page.Serial)
			}
			if groupOpen && groupHasData {
				return fmt.Errorf("offset %d: BOS for new stream 0x%08x after data pages of the current group", page.Offset, page.Serial)
			}
			s = &streamState{serial: page.Serial, expectSeq: page.Sequence}
			streams[page.Serial] = s
			order = append(order, s)
			groupOpen = true
		} else {
			if s.ended {
				return fmt.Errorf("offset %d: page for stream 0x%08x after its EOS page", page.Offset, page.Serial)
			}
			if page.BOS() {
				return fmt.Errorf("offset %d: duplicate BOS flag for stream 0x%08x", page.Offset, page.Serial)
			}
			if page.Sequence != s.expectSeq {
				return fmt.Errorf("offset %d: stream 0x%08x page sequence gap: got %d, expected %d (lost or reordered page)",
					page.Offset, page.Serial, page.Sequence, s.expectSeq)
			}
		}
		s.bosSeen = true
		s.expectSeq = page.Sequence + 1 // uint32 wrap-around is legal
		if !page.BOS() {
			groupHasData = true
		}

		if page.Continued() && s.pending.Len() == 0 {
			return fmt.Errorf("offset %d: stream 0x%08x continued flag with no packet in progress", page.Offset, page.Serial)
		}
		if !page.Continued() && s.pending.Len() > 0 {
			return fmt.Errorf("offset %d: stream 0x%08x previous packet unfinished but page lacks continued flag", page.Offset, page.Serial)
		}

		s.pages = append(s.pages, PageRef{Offset: page.Offset, Length: pageLen})

		// Walk the segment table: a segment < 255 terminates a packet.
		pos := 0
		for _, seg := range page.SegmentTable {
			n := int(seg)
			s.pending.Write(page.Data[pos : pos+n])
			pos += n
			if int64(s.pending.Len()) > maxPkt {
				return fmt.Errorf("offset %d: stream 0x%08x packet exceeds maximum size %d bytes",
					page.Offset, page.Serial, maxPkt)
			}
			if n < 255 {
				if err := finishPacket(s, page, pageLen); err != nil {
					return err
				}
			}
		}
		if s.pending.Len() == 0 {
			// No packet in progress: page refs belong only to packets
			// completed on this page.
			s.pages = s.pages[:0]
		}
		if opts.OnPage != nil {
			opts.OnPage(page)
		}

		if page.EOS() {
			if s.pending.Len() > 0 {
				return fmt.Errorf("offset %d: stream 0x%08x EOS with an unfinished packet", page.Offset, page.Serial)
			}
			s.ended = true
		}
		// A new group may start once every stream of the group has ended.
		allEnded := true
		for _, st := range order {
			if !st.ended {
				allEnded = false
				break
			}
		}
		if allEnded {
			groupOpen = false
			groupHasData = false
			streams = map[uint32]*streamState{}
			order = nil
		}
	}

	for _, s := range order {
		if s.pending.Len() > 0 {
			return fmt.Errorf("offset %d: stream 0x%08x ends with an unfinished packet (%d bytes pending)",
				rd.Offset(), s.serial, s.pending.Len())
		}
		if !s.ended {
			return fmt.Errorf("offset %d: stream 0x%08x missing EOS page", rd.Offset(), s.serial)
		}
	}
	return nil
}
