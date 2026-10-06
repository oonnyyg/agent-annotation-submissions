package delta

import (
	"fmt"
	"sort"
)

// Generate builds a delta that reconstructs newData using only the
// signature of the old file. The old file itself is never read.
// It returns the delta plus counts of reused and literal bytes.
func Generate(sig *Signature, newData []byte) (*Delta, int64, int64, error) {
	if err := sig.Validate(); err != nil {
		return nil, 0, 0, fmt.Errorf("invalid signature: %w", err)
	}

	// Index blocks by weak sum; indices stay in ascending offset order.
	byWeak := make(map[uint32][]int, len(sig.Blocks))
	for i, b := range sig.Blocks {
		byWeak[b.Weak] = append(byWeak[b.Weak], i)
	}

	// Distinct block lengths, longest first (last block may be shorter).
	lengths := []int{sig.BlockSize}
	if n := len(sig.Blocks); n > 0 {
		if last := sig.Blocks[n-1].Length; last != sig.BlockSize {
			lengths = append(lengths, last)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(lengths)))

	d := &Delta{
		Format:       DeltaFormat,
		SourceSize:   sig.FileSize,
		SourceSHA256: sig.SHA256,
		TargetSize:   int64(len(newData)),
		TargetSHA256: sha256Hex(newData),
		Ops:          []Op{},
	}

	var reused, literal int64
	var lit []byte
	flushLiteral := func() {
		if len(lit) > 0 {
			d.Ops = append(d.Ops, Op{Op: "literal", Data: lit})
			literal += int64(len(lit))
			lit = nil
		}
	}

	// Rollers per block length, rebuilt whenever the scan position jumps.
	rollers := make(map[int]*roller, len(lengths))
	resetRollers := func(pos int) {
		for _, L := range lengths {
			if pos+L <= len(newData) {
				rollers[L] = newRoller(newData[pos : pos+L])
			} else {
				rollers[L] = nil
			}
		}
	}

	pos := 0
	resetRollers(0)
	for pos < len(newData) {
		bestLen, bestOff := 0, int64(0)
		for _, L := range lengths {
			if L <= bestLen || pos+L > len(newData) {
				continue
			}
			r := rollers[L]
			if r == nil {
				continue
			}
			cands := byWeak[r.sum()]
			if len(cands) == 0 {
				continue
			}
			strong := sha256Hex(newData[pos : pos+L])
			for _, idx := range cands {
				b := sig.Blocks[idx]
				if b.Length == L && b.Strong == strong {
					// Longest match wins (lengths scanned descending);
					// first candidate has the smallest old offset.
					bestLen, bestOff = L, b.Offset
					break
				}
			}
		}
		if bestLen > 0 {
			flushLiteral()
			d.Ops = append(d.Ops, Op{Op: "copy", Offset: bestOff, Length: bestLen})
			reused += int64(bestLen)
			pos += bestLen
			resetRollers(pos)
			continue
		}
		lit = append(lit, newData[pos])
		// Roll both windows one byte forward.
		for _, L := range lengths {
			if r := rollers[L]; r != nil && pos+L+1 <= len(newData) {
				r.roll(newData[pos], newData[pos+L])
			} else if pos+L+1 <= len(newData) {
				rollers[L] = newRoller(newData[pos+1 : pos+1+L])
			} else {
				rollers[L] = nil
			}
		}
		pos++
	}
	flushLiteral()
	return d, reused, literal, nil
}
