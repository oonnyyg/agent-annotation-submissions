package ogg

import "fmt"

// PageRef identifies one page contributing to a packet.
type PageRef struct {
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
	Sequence uint32 `json:"sequence"`
}

// Packet is a fully reassembled logical packet.
type Packet struct {
	Index   int       // packet index within its stream generation
	Data    []byte    // raw packet bytes
	Pages   []PageRef // pages that carried (parts of) this packet
	Granule uint64    // granule position of the page on which the packet ends
}

// Stream tracks one logical stream generation: page sequencing, BOS/EOS
// state, and reassembly of packets that span pages.
type Stream struct {
	Serial uint32

	started  bool
	ended    bool
	nextSeq  uint32
	pktIndex int

	partial      []byte
	partialPages []PageRef

	// MaxPacketSize, if positive, rejects packets larger than this many
	// bytes. It guards against corrupt segment tables producing
	// unbounded allocations.
	MaxPacketSize int64
}

// StreamError describes a logical-stream violation at a known page offset.
type StreamError struct {
	Offset int64
	Serial uint32
	Msg    string
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("ogg: stream 0x%08x, page at offset %d: %s", e.Serial, e.Offset, e.Msg)
}

func (s *Stream) err(off int64, format string, args ...any) *StreamError {
	return &StreamError{Offset: off, Serial: s.Serial, Msg: fmt.Sprintf(format, args...)}
}

// Feed consumes one page belonging to this stream and returns any packets
// completed by it. Pages must be delivered in file order.
func (s *Stream) Feed(p *Page) ([]*Packet, error) {
	if s.ended {
		return nil, s.err(p.Offset, "page follows end-of-stream page")
	}
	if !s.started {
		if p.HeaderType&FlagBOS == 0 {
			return nil, s.err(p.Offset, "first page of stream lacks BOS flag")
		}
		if p.HeaderType&FlagContinuation != 0 {
			return nil, s.err(p.Offset, "BOS page must not be a continued page")
		}
		s.started = true
		s.nextSeq = p.Sequence + 1
	} else {
		if p.HeaderType&FlagBOS != 0 {
			return nil, s.err(p.Offset, "duplicate BOS flag inside stream")
		}
		if p.Sequence != s.nextSeq {
			return nil, s.err(p.Offset, "page sequence gap: got %d, want %d (missing or reordered page)", p.Sequence, s.nextSeq)
		}
		s.nextSeq++
	}

	continued := p.HeaderType&FlagContinuation != 0
	if continued && s.partial == nil {
		return nil, s.err(p.Offset, "continuation flag set but no packet is pending")
	}
	if !continued && s.partial != nil {
		return nil, s.err(p.Offset, "previous packet unfinished but page lacks continuation flag")
	}

	ref := PageRef{Offset: p.Offset, Length: p.EndOffset() - p.Offset, Sequence: p.Sequence}
	var out []*Packet
	pos := 0
	for _, lace := range p.Segments {
		n := int(lace)
		s.partial = append(s.partial, p.Body[pos:pos+n]...)
		pos += n
		if s.partialPages == nil {
			s.partialPages = []PageRef{}
		}
		s.partialPages = append(s.partialPages, ref)
		if s.MaxPacketSize > 0 && int64(len(s.partial)) > s.MaxPacketSize {
			return nil, s.err(p.Offset, "packet exceeds maximum size of %d bytes", s.MaxPacketSize)
		}
		if lace < 255 {
			// A lacing value below 255 terminates the packet; 0 yields
			// a valid empty packet, and multiples of 255 end with a 0.
			out = append(out, &Packet{
				Index:   s.pktIndex,
				Data:    s.partial,
				Pages:   s.partialPages,
				Granule: p.Granule,
			})
			s.pktIndex++
			s.partial = nil
			s.partialPages = nil
		}
	}

	if p.HeaderType&FlagEOS != 0 {
		if s.partial != nil {
			return nil, s.err(p.Offset, "end-of-stream page leaves an unfinished packet")
		}
		s.ended = true
	}
	return out, nil
}

// Finish validates end-of-file state for the stream.
func (s *Stream) Finish() error {
	if !s.started {
		return &StreamError{Serial: s.Serial, Msg: "stream never started"}
	}
	if !s.ended {
		return &StreamError{Serial: s.Serial, Msg: "stream has no end-of-stream page"}
	}
	if s.partial != nil {
		return &StreamError{Serial: s.Serial, Msg: "file ends with an unfinished packet"}
	}
	return nil
}
