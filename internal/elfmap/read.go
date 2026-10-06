package elfmap

import (
	"bytes"
	"fmt"
	"io"
	"sort"
)

// MaxReadLen caps a single read query at 64 KiB.
const MaxReadLen = 65536

// Piece is one contiguous fragment of a read result: either file bytes or a
// zero-filled range. Zero ranges are represented as intervals instead of
// expanded bytes so huge BSS areas never allocate full virtual memory.
type Piece struct {
	Kind    string `json:"kind"` // "data" or "zero"
	Start   uint64 `json:"start"`
	End     uint64 `json:"end"`
	DataB64 string `json:"data_b64,omitempty"` // base64, kind == "data"
}

// ReadResult is the interval-based representation of a successful read.
type ReadResult struct {
	Address uint64  `json:"address"`
	Length  uint64  `json:"length"`
	Pieces  []Piece `json:"pieces"`
}

// Read walks the virtual address range [addr, addr+length) without
// materializing zero pages. It may cross adjacent segments and their zero-fill
// tails, but touching any unmapped hole fails the entire read. File bytes are
// returned in base64 pieces; zero runs become "zero" interval pieces.
func (m *LoadMap) Read(r io.ReaderAt, addr uint64, length int) (*ReadResult, error) {
	if length < 0 {
		return nil, fmt.Errorf("negative read length %d", length)
	}
	if length > MaxReadLen {
		return nil, fmt.Errorf("read length %d exceeds maximum %d", length, MaxReadLen)
	}
	if length == 0 {
		return &ReadResult{Address: addr, Length: 0, Pieces: []Piece{}}, nil
	}
	end, overflow := addOverflow(addr, uint64(length))
	if overflow {
		return nil, fmt.Errorf("read range [%#x,+%d) overflows 64-bit address space", addr, length)
	}

	segs := sortedSegments(m.Segments)
	result := &ReadResult{Address: addr, Length: uint64(length), Pieces: []Piece{}}

	var dataBuf bytes.Buffer
	dataStart := uint64(0)
	flush := func(cur uint64) error {
		if dataBuf.Len() == 0 {
			return nil
		}
		if dataStart >= cur {
			return fmt.Errorf("internal: empty data piece")
		}
		result.Pieces = append(result.Pieces, Piece{
			Kind:    "data",
			Start:   dataStart,
			End:     cur,
			DataB64: stdBase64(dataBuf.Bytes()),
		})
		dataBuf.Reset()
		return nil
	}

	pos := addr
	si := 0
	for pos < end {
		// Find a segment covering pos; segments are non-overlapping.
		for si < len(segs) {
			sEnd, _ := segs[si].MemEnd()
			if segs[si].Memsz != 0 && pos >= segs[si].Vaddr && pos < sEnd {
				break
			}
			if segs[si].Memsz != 0 && sEnd <= pos {
				si++
				continue
			}
			break
		}
		if si >= len(segs) {
			return nil, fmt.Errorf("address %#x is not mapped (read would touch an unmapped hole)", pos)
		}
		s := segs[si]
		if s.Memsz == 0 || pos < s.Vaddr {
			return nil, fmt.Errorf("address %#x is not mapped (hole before segment %d)", pos, s.Index)
		}
		fileEnd, _ := s.FileEnd()
		memEnd, _ := s.MemEnd()
		chunkEnd := min64(end, memEnd)
		if pos < fileEnd {
			dataEnd := min64(chunkEnd, fileEnd)
			if dataBuf.Len() == 0 {
				dataStart = pos
			}
			off := s.FileOff + (pos - s.Vaddr)
			buf := make([]byte, dataEnd-pos)
			if _, err := r.ReadAt(buf, int64(off)); err != nil {
				return nil, fmt.Errorf("segment %d file read: %w", s.Index, err)
			}
			dataBuf.Write(buf)
			pos = dataEnd
			if pos == chunkEnd {
				if err := flush(pos); err != nil {
					return nil, err
				}
				continue
			}
		}
		// Remaining part of this segment is zero-filled.
		if err := flush(pos); err != nil {
			return nil, err
		}
		zeroEnd := chunkEnd
		result.Pieces = append(result.Pieces, Piece{Kind: "zero", Start: pos, End: zeroEnd})
		pos = zeroEnd
	}
	if err := flush(end); err != nil {
		return nil, err
	}
	return result, nil
}

func sortedSegments(segs []Segment) []Segment {
	out := make([]Segment, len(segs))
	copy(out, segs)
	sort.Slice(out, func(i, j int) bool { return out[i].Vaddr < out[j].Vaddr })
	return out
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
