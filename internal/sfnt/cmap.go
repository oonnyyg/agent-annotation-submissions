package sfnt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// cmap maps runes to glyph IDs using one selected subtable.
type cmap struct {
	format4 *cmapFormat4
	groups  []cmapGroup // format 12
}

type cmapGroup struct {
	start, end, startGlyph uint32
}

// loadCmap picks the Windows Unicode full repertoire (platform 3,
// encoding 10, format 12) subtable if present, otherwise the Windows
// Unicode BMP (platform 3, encoding 1, format 4) subtable. Duplicate
// subtables of the same preferred encoding are rejected; mappings from
// two subtables are never combined.
func (f *Font) loadCmap() (*cmap, error) {
	data := f.table("cmap")
	if len(data) < 4 {
		return nil, errors.New("cmap: table too small")
	}
	numTables := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+numTables*8 {
		return nil, errors.New("cmap: encoding records exceed table size")
	}
	var off12, off4 = -1, -1
	for i := 0; i < numTables; i++ {
		rec := data[4+i*8 : 4+(i+1)*8]
		platform := binary.BigEndian.Uint16(rec[0:2])
		encoding := binary.BigEndian.Uint16(rec[2:4])
		off := binary.BigEndian.Uint32(rec[4:8])
		if uint64(off)+2 > uint64(len(data)) {
			return nil, fmt.Errorf("cmap: subtable %d offset out of range", i)
		}
		if platform != 3 {
			continue
		}
		switch encoding {
		case 10:
			if off12 >= 0 {
				return nil, errors.New("cmap: duplicate Windows Unicode full repertoire subtable")
			}
			if binary.BigEndian.Uint16(data[off:off+2]) != 12 {
				return nil, fmt.Errorf("cmap: (3,10) subtable has unsupported format %d", binary.BigEndian.Uint16(data[off:off+2]))
			}
			off12 = int(off)
		case 1:
			if off4 >= 0 {
				return nil, errors.New("cmap: duplicate Windows Unicode BMP subtable")
			}
			if binary.BigEndian.Uint16(data[off:off+2]) != 4 {
				return nil, fmt.Errorf("cmap: (3,1) subtable has unsupported format %d", binary.BigEndian.Uint16(data[off:off+2]))
			}
			off4 = int(off)
		}
	}
	switch {
	case off12 >= 0:
		return parseCmap12(data[off12:])
	case off4 >= 0:
		return parseCmap4(data[off4:])
	default:
		return nil, errors.New("cmap: no Windows Unicode (3,10) or (3,1) subtable found")
	}
}

func parseCmap12(data []byte) (*cmap, error) {
	if len(data) < 16 {
		return nil, errors.New("cmap format 12: header too small")
	}
	length := binary.BigEndian.Uint32(data[4:8])
	if uint64(length) > uint64(len(data)) || length < 16 {
		return nil, errors.New("cmap format 12: declared length exceeds table")
	}
	data = data[:length]
	n := binary.BigEndian.Uint32(data[12:16])
	if uint64(16)+uint64(n)*12 > uint64(length) {
		return nil, errors.New("cmap format 12: groups exceed subtable length")
	}
	groups := make([]cmapGroup, 0, n)
	prevEnd := uint32(0)
	for i := uint32(0); i < n; i++ {
		rec := data[16+i*12 : 16+(i+1)*12]
		g := cmapGroup{
			start:      binary.BigEndian.Uint32(rec[0:4]),
			end:        binary.BigEndian.Uint32(rec[4:8]),
			startGlyph: binary.BigEndian.Uint32(rec[8:12]),
		}
		if g.end < g.start || g.end > 0x10FFFF {
			return nil, fmt.Errorf("cmap format 12: invalid group range [%d, %d]", g.start, g.end)
		}
		if i > 0 && g.start <= prevEnd {
			return nil, errors.New("cmap format 12: overlapping or unsorted groups")
		}
		prevEnd = g.end
		groups = append(groups, g)
	}
	return &cmap{groups: groups}, nil
}

// glyphID returns the glyph for r, or 0 (.notdef) when unmapped.
func (c *cmap) glyphID(r rune) uint16 {
	if c.format4 != nil {
		return c.format4.glyphID(r)
	}
	lo, hi := 0, len(c.groups)
	for lo < hi {
		mid := (lo + hi) / 2
		g := c.groups[mid]
		switch {
		case uint32(r) < g.start:
			hi = mid
		case uint32(r) > g.end:
			lo = mid + 1
		default:
			return uint16(g.startGlyph + uint32(r) - g.start)
		}
	}
	return 0
}
