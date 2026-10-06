// Package sfnt parses the table directory of a single TrueType sfnt file
// and exposes validated access to the tables needed for text metrics.
package sfnt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type tableRecord struct {
	tag    string
	offset uint32
	length uint32
}

// Font is a validated, non-collection, non-variable TrueType font.
type Font struct {
	data   []byte
	tables map[string]tableRecord
}

// Parse reads the sfnt offset table and table directory, rejecting
// collections, CFF-based fonts, variable fonts, duplicate tags and
// out-of-range table extents.
func Parse(data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, errors.New("sfnt: file too small for offset table")
	}
	switch tag := string(data[0:4]); tag {
	case "\x00\x01\x00\x00", "true":
		// TrueType outlines, supported.
	case "ttcf":
		return nil, errors.New("sfnt: TrueType collections are not supported")
	case "OTTO":
		return nil, errors.New("sfnt: CFF-based OpenType fonts are not supported")
	default:
		return nil, fmt.Errorf("sfnt: unrecognized sfnt version %q", tag)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	dirEnd := 12 + numTables*16
	if numTables == 0 || dirEnd > len(data) {
		return nil, fmt.Errorf("sfnt: table directory (%d tables) exceeds file size", numTables)
	}
	f := &Font{data: data, tables: make(map[string]tableRecord, numTables)}
	for i := 0; i < numTables; i++ {
		rec := data[12+i*16 : 12+(i+1)*16]
		tag := string(rec[0:4])
		off := binary.BigEndian.Uint32(rec[8:12])
		length := binary.BigEndian.Uint32(rec[12:16])
		if _, dup := f.tables[tag]; dup {
			return nil, fmt.Errorf("sfnt: duplicate table tag %q", tag)
		}
		if uint64(off)+uint64(length) > uint64(len(data)) {
			return nil, fmt.Errorf("sfnt: table %q range [%d, %d) exceeds file size %d", tag, off, off+length, len(data))
		}
		f.tables[tag] = tableRecord{tag: tag, offset: off, length: length}
	}
	for _, required := range []string{"head", "hhea", "hmtx", "maxp", "cmap"} {
		if _, ok := f.tables[required]; !ok {
			return nil, fmt.Errorf("sfnt: required table %q is missing", required)
		}
	}
	for _, variable := range []string{"fvar", "gvar"} {
		if _, ok := f.tables[variable]; ok {
			return nil, fmt.Errorf("sfnt: variable fonts are not supported (found %q table)", variable)
		}
	}
	return f, nil
}

// table returns the raw bytes of a table, or nil if absent.
func (f *Font) table(tag string) []byte {
	rec, ok := f.tables[tag]
	if !ok {
		return nil
	}
	return f.data[rec.offset : rec.offset+rec.length]
}
