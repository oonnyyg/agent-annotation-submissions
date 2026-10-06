package sfnt

import (
	"encoding/binary"
	"fmt"
)

// Table tags relevant to a TrueType font.
const (
	tagHead = 0x68656164 // "head"
	tagHhea = 0x68686561 // "hhea"
	tagMaxp = 0x6d617870 // "maxp"
	tagHmtx = 0x686d7478 // "hmtx"
	tagCmap = 0x636d6170 // "cmap"
	tagGlyf = 0x676c7966 // "glyf"
	tagLoca = 0x6c6f6361 // "loca"
	tagKern = 0x6b65726e // "kern"
	tagFvar = 0x66766172 // "fvar"
	tagGvar = 0x67766172 // "gvar"
)

type tableRecord struct {
	tag    uint32
	offset uint32
	length uint32
}

// Font is a validated, non-collection, non-variable TrueType font.
type Font struct {
	data       []byte
	tables     map[uint32]tableRecord
	unitsPerEm uint16
	numGlyphs  uint16
	numHMtx    uint16

	advances []uint16 // length numHMtx
	leftSide []int16  // length numGlyphs - numHMtx

	cmapKind string
	lookup   func(rune) (uint16, error)

	kernPairs map[uint32]int16
}

// UnitsPerEm returns the units-per-em value from the head table.
func (f *Font) UnitsPerEm() uint16 { return f.unitsPerEm }

// NumGlyphs returns the number of glyphs from maxp.
func (f *Font) NumGlyphs() uint16 { return f.numGlyphs }

// CmapKind describes the selected character map.
func (f *Font) CmapKind() string { return f.cmapKind }

func (f *Font) table(tag uint32) (tableRecord, bool) {
	t, ok := f.tables[tag]
	return t, ok
}

// tableBytes returns the raw bytes of a table and bounds-checks them.
func (f *Font) tableBytes(tag uint32) ([]byte, tableRecord, error) {
	t, ok := f.tables[tag]
	if !ok {
		return nil, t, fmt.Errorf("missing required table %q", tagString(tag))
	}
	end := uint64(t.offset) + uint64(t.length)
	if end > uint64(len(f.data)) {
		return nil, t, fmt.Errorf("table %q extends past end of file", tagString(tag))
	}
	return f.data[t.offset:end], t, nil
}

func tagString(tag uint32) string {
	return string([]byte{byte(tag >> 24), byte(tag >> 16), byte(tag >> 8), byte(tag)})
}

// Parse parses and validates a single TrueType (.ttf) sfnt font.
func Parse(data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("file too short for an sfnt offset table")
	}
	scaler := binary.BigEndian.Uint32(data[0:4])
	switch scaler {
	case 0x00010000, 0x74727565: // 1.0 or "true"
	case 0x74746366: // "ttcf"
		return nil, fmt.Errorf("font collections (ttcf) are not supported")
	case 0x4f54544f: // "OTTO"
		return nil, fmt.Errorf("CFF-based OpenType fonts are not supported; TrueType outlines required")
	default:
		return nil, fmt.Errorf("unrecognized sfnt scaler type 0x%08x", scaler)
	}

	numTables := binary.BigEndian.Uint16(data[4:6])
	if numTables == 0 {
		return nil, fmt.Errorf("font contains no tables")
	}
	dirEnd := uint64(12) + uint64(numTables)*16
	if dirEnd > uint64(len(data)) {
		return nil, fmt.Errorf("table directory extends past end of file")
	}

	f := &Font{data: data, tables: make(map[uint32]tableRecord, numTables)}
	for i := uint16(0); i < numTables; i++ {
		rec := data[12+uint64(i)*16:]
		tag := binary.BigEndian.Uint32(rec[0:4])
		if _, dup := f.tables[tag]; dup {
			return nil, fmt.Errorf("duplicate table %q in table directory", tagString(tag))
		}
		offset := binary.BigEndian.Uint32(rec[8:12])
		length := binary.BigEndian.Uint32(rec[12:16])
		if uint64(offset)+uint64(length) > uint64(len(data)) {
			return nil, fmt.Errorf("table %q (offset %d, length %d) extends past end of file",
				tagString(tag), offset, length)
		}
		t := tableRecord{tag: tag, offset: offset, length: length}
		f.tables[tag] = t
}

	// Variable fonts are out of scope.
	if _, ok := f.tables[tagFvar]; ok {
		return nil, fmt.Errorf("variable fonts (fvar) are not supported")
	}
	if _, ok := f.tables[tagGvar]; ok {
		return nil, fmt.Errorf("variable fonts (gvar) are not supported")
	}

	for _, req := range []uint32{tagHead, tagHhea, tagMaxp, tagHmtx, tagCmap, tagGlyf, tagLoca} {
		if _, ok := f.tables[req]; !ok {
			return nil, fmt.Errorf("missing required TrueType table %q", tagString(req))
		}
	}

	if err := f.parseHead(); err != nil {
		return nil, err
	}
	if err := f.parseMaxp(); err != nil {
		return nil, err
	}
	if err := f.parseHheaAndHmtx(); err != nil {
		return nil, err
	}
	if err := f.parseCmap(); err != nil {
		return nil, err
	}
	// Validate loca length against head/maxp so that out-of-bounds glyph access
	// cannot occur later.
	if err := f.validateLoca(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Font) parseHead() error {
	b, _, err := f.tableBytes(tagHead)
	if err != nil {
	return err
	}
	if len(b) < 54 {
		return fmt.Errorf("head table too short (%d bytes)", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 0x00010000 {
		return fmt.Errorf("head table has unsupported version")
	}
	upm := binary.BigEndian.Uint16(b[18:20])
	if upm == 0 {
		return fmt.Errorf("head unitsPerEm must not be zero")
	}
	f.unitsPerEm = upm
	return nil
}

func (f *Font) parseMaxp() error {
	b, _, err := f.tableBytes(tagMaxp)
	if err != nil {
		return err
	}
	if len(b) < 6 {
		return fmt.Errorf("maxp table too short")
	}
	if binary.BigEndian.Uint32(b[0:4]) != 0x00010000 {
		return fmt.Errorf("only TrueType maxp version 1.0 is supported")
	}
	f.numGlyphs = binary.BigEndian.Uint16(b[4:6])
	if f.numGlyphs == 0 {
		return fmt.Errorf("maxp numGlyphs must not be zero")
	}
	return nil
}

func (f *Font) validateLoca() error {
	b, _, err := f.tableBytes(tagLoca)
	if err != nil {
		return err
	}
	head, _, _ := f.tableBytes(tagHead)
	indexToLocFormat := int16(binary.BigEndian.Uint16(head[50:52]))
	var entrySize uint64
	switch indexToLocFormat {
	case 0:
		entrySize = 2
	case 1:
		entrySize = 4
	default:
		return fmt.Errorf("head indexToLocFormat %d is unsupported", indexToLocFormat)
	}
	need := entrySize * (uint64(f.numGlyphs) + 1)
	if uint64(len(b)) < need {
		return fmt.Errorf("loca table too short for %d glyphs", f.numGlyphs)
	}
	return nil
}
