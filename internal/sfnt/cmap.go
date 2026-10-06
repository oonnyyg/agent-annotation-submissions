package sfnt

import (
	"encoding/binary"
	"fmt"
)

func (f *Font) parseCmap() error {
	b, rec, err := f.tableBytes(tagCmap)
	if err != nil {
		return err
	}
	if len(b) < 4 {
		return fmt.Errorf("cmap table too short")
	}
	version := binary.BigEndian.Uint16(b[0:2])
	if version != 0 {
		return fmt.Errorf("cmap version %d unsupported (only 0)", version)
	}
	numRecords := int(binary.BigEndian.Uint16(b[2:4]))
	if uint64(4)+uint64(numRecords)*8 > uint64(len(b)) {
		return fmt.Errorf("cmap encoding records extend past table end")
	}

	// Priority: (3,10) format 12 (Windows Unicode full repertoire),
	// otherwise (3,1) format 4 (Windows Unicode BMP).
	var off12, off4 uint32
	var count12, count4 int
	for i := 0; i < numRecords; i++ {
		r := b[4+i*8:]
		platform := binary.BigEndian.Uint16(r[0:2])
		encoding := binary.BigEndian.Uint16(r[2:4])
		offset := binary.BigEndian.Uint32(r[4:8])
		if platform != 3 {
			continue
		}
		switch encoding {
		case 10:
			if uint64(offset)+2 > uint64(len(b)) {
				return fmt.Errorf("cmap subtable (3,10) offset out of range")
			}
			if format := binary.BigEndian.Uint16(b[offset : offset+2]); format != 12 {
				return fmt.Errorf("cmap (3,10) subtable has unsupported format %d (only 12)", format)
			}
			off12 = offset
			count12++
		case 1:
			if uint64(offset)+2 > uint64(len(b)) {
				return fmt.Errorf("cmap subtable (3,1) offset out of range")
			}
			if format := binary.BigEndian.Uint16(b[offset : offset+2]); format != 4 {
				return fmt.Errorf("cmap (3,1) subtable has unsupported format %d (only 4)", format)
			}
			off4 = offset
			count4++
		}
	}
	if count12 > 1 {
		return fmt.Errorf("duplicate preferred cmap subtable (platform 3, encoding 10)")
	}
	if count4 > 1 {
		return fmt.Errorf("duplicate fallback cmap subtable (platform 3, encoding 1)")
	}

	switch {
	case count12 == 1:
		// The (3,1) format 4 table is deliberately ignored: maps are never spliced.
		lookup, err := parseFormat12(b, off12)
		if err != nil {
			return err
		}
		f.lookup = lookup
		f.cmapKind = fmt.Sprintf("format12 table=%q offset=%d", tagString(rec.tag), rec.offset)
	case count4 == 1:
		lookup, err := parseFormat4(b, off4)
		if err != nil {
			return err
		}
		f.lookup = lookup
		f.cmapKind = fmt.Sprintf("format4 table=%q offset=%d", tagString(rec.tag), rec.offset)
	default:
		return fmt.Errorf("no usable cmap: require Windows Unicode full repertoire (3,10) format 12 or BMP (3,1) format 4")
	}
	return nil
}

// Lookup maps a rune to a glyph id. Returns 0,true for missing characters.
func (f *Font) Lookup(r rune) (glyphID uint16, missing bool, err error) {
	g, err := f.lookup(r)
	if err != nil {
		return 0, false, err
	}
	if g >= f.numGlyphs {
		return 0, false, fmt.Errorf("cmap returned glyph id %d out of range (numGlyphs=%d)", g, f.numGlyphs)
	}
	return g, g == 0, nil
}

func parseFormat4(b []byte, start uint32) (func(rune) (uint16, error), error) {
	if uint64(start)+14 > uint64(len(b)) {
		return nil, fmt.Errorf("cmap format 4 header out of range")
	}
	hdr := b[start:]
	length := binary.BigEndian.Uint16(hdr[2:4])
	if uint64(start)+uint64(length) > uint64(len(b)) {
		return nil, fmt.Errorf("cmap format 4 length %d extends past table end", length)
	}
	if length < 16 {
		return nil, fmt.Errorf("cmap format 4 length too small")
	}
	segCountX2 := binary.BigEndian.Uint16(hdr[6:8])
	if segCountX2 == 0 || segCountX2%2 != 0 {
		return nil, fmt.Errorf("cmap format 4 invalid segCountX2 %d", segCountX2)
	}
	segCount := int(segCountX2 / 2)
	endCodeOff := 14
	startCodeOff := endCodeOff + segCountX2 + 2 // + reservedPad
	idDeltaOff := startCodeOff + segCountX2
	idRangeOff := idDeltaOff + segCountX2
	if int(length) < idRangeOff+segCountX2 {
		return nil, fmt.Errorf("cmap format 4 arrays exceed declared length")
	}
	u16 := func(off int) uint16 { return binary.BigEndian.Uint16(hdr[off : off+2]) }
	i16 := func(off int) int16 { return int16(u16(off)) }

	lookup := func(r rune) (uint16, error) {
		if r < 0 || r > 0xFFFF {
			return 0, nil // format 4 covers the BMP only
		}
		c := uint16(r)
		var seg int
		for seg = 0; seg < segCount; seg++ {
			if c <= u16(endCodeOff+2*seg) {
				break
			}
		}
		if seg == segCount {
			return 0, nil
		}
		if c < u16(startCodeOff+2*seg) {
			return 0, nil
		}
		rangeOff := u16(idRangeOff + 2*seg)
		if rangeOff == 0 {
			return uint16(int32(c) + int32(i16(idDeltaOff+2*seg))), nil
		}
		// Offset is relative to the location of the idRangeOffset word itself.
		glyphOff := idRangeOff + 2*seg + int(rangeOff) + 2*int(c-u16(startCodeOff+2*seg))
		if glyphOff+2 > int(length) {
			return 0, nil
		}
		g := u16(glyphOff)
		if g == 0 {
			return 0, nil
		}
		return uint16(int32(g) + int32(i16(idDeltaOff+2*seg))), nil
	}
	return lookup, nil
}

type format12Group struct {
	startCharCode uint32
	endCharCode   uint32
	startGlyphID  uint32
}

func parseFormat12(b []byte, start uint32) (func(rune) (uint16, error), error) {
	if uint64(start)+16 > uint64(len(b)) {
		return nil, fmt.Errorf("cmap format 12 header out of range")
	}
	hdr := b[start:]
	format := binary.BigEndian.Uint16(hdr[0:2])
	if format != 12 {
		return nil, fmt.Errorf("expected cmap format 12, got %d", format)
	}
	length := binary.BigEndian.Uint32(hdr[4:8])
	if uint64(start)+uint64(length) > uint64(len(b)) {
		return nil, fmt.Errorf("cmap format 12 length %d extends past table end", length)
	}
	numGroups := binary.BigEndian.Uint32(hdr[12:16])
	need := uint64(16) + uint64(numGroups)*12
	if uint64(length) < need {
		return nil, fmt.Errorf("cmap format 12 groups exceed declared length")
	}
	groups := make([]format12Group, numGroups)
	for i := uint32(0); i < numGroups; i++ {
		g := hdr[16+i*12:]
		groups[i] = format12Group{
			startCharCode: binary.BigEndian.Uint32(g[0:4]),
			endCharCode:   binary.BigEndian.Uint32(g[4:8]),
			startGlyphID:  binary.BigEndian.Uint32(g[8:12]),
		}
	}
	lookup := func(r rune) (uint16, error) {
		if r < 0 {
			return 0, nil
		}
		c := uint32(r)
		for _, g := range groups {
			if c < g.startCharCode {
				break
			}
			if c <= g.endCharCode {
				return uint16(g.startGlyphID + c - g.startCharCode), nil
			}
		}
		return 0, nil
	}
	return lookup, nil
}
