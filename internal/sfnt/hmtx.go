package sfnt

import (
	"encoding/binary"
	"fmt"
)

func (f *Font) parseHheaAndHmtx() error {
	hh, _, err := f.tableBytes(tagHhea)
	if err != nil {
		return err
	}
	if len(hh) < 36 {
		return fmt.Errorf("hhea table too short")
	}
	f.numHMtx = binary.BigEndian.Uint16(hh[34:36])
	if f.numHMtx == 0 {
		return fmt.Errorf("hhea numberOfHMetrics must not be zero")
	}
	if f.numHMtx > f.numGlyphs {
		return fmt.Errorf("hhea numberOfHMetrics (%d) exceeds maxp numGlyphs (%d)",
			f.numHMtx, f.numGlyphs)
	}

	hm, _, err := f.tableBytes(tagHmtx)
	if err != nil {
		return err
	}
	longSize := uint64(f.numHMtx) * 4
	shortCount := uint64(f.numGlyphs) - uint64(f.numHMtx)
	need := longSize + shortSize*2
	if uint64(len(hm)) < need {
		return fmt.Errorf("hmtx table too short: need %d bytes, have %d", need, len(hm))
	}

	f.advances = make([]uint16, f.numHMtx)
	for i := uint16(0); i < f.numHMtx; i++ {
		off := uint64(i) * 4
		f.advances[i] = binary.BigEndian.Uint16(hm[off : off+2])
	}
	f.leftSide = make([]int16, shortCount)
	for i := uint64(0); i < shortCount; i++ {
		off := longSize + i*2
		f.leftSide[i] = int16(binary.BigEndian.Uint16(hm[off : off+2]))
	}
	return nil
}

// AdvanceWidth returns the unscaled horizontal advance of a glyph, in font units.
// Glyphs beyond the last long horizontal metric repeat the last advance width.
func (f *Font) AdvanceWidth(glyphID uint16) (uint16, error) {
	if glyphID >= f.numGlyphs {
		return 0, fmt.Errorf("glyph id %d out of range (numGlyphs=%d)", glyphID, f.numGlyphs)
	}
	if glyphID < f.numHMtx {
		return f.advances[glyphID], nil
	}
	return f.advances[f.numHMtx-1], nil
}
