package sfnt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// Metrics holds the horizontal metric data of a font.
type Metrics struct {
	UnitsPerEm     uint16
	NumGlyphs      uint16
	advances       []uint16 // numberOfHMetrics entries
	numLongMetrics int
	cmap           *cmap
	kern           map[uint32]int16 // key: left<<16 | right
	kernLoaded     bool
}

// LoadMetrics reads head, maxp, hhea and hmtx and validates their
// size relationships.
func (f *Font) LoadMetrics() (*Metrics, error) {
	head := f.table("head")
	if len(head) < 54 {
		return nil, errors.New("head: table too small")
	}
	m := &Metrics{UnitsPerEm: binary.BigEndian.Uint16(head[18:20])}
	if m.UnitsPerEm == 0 {
		return nil, errors.New("head: unitsPerEm is zero")
	}
	maxp := f.table("maxp")
	if len(maxp) < 6 {
		return nil, errors.New("maxp: table too small")
	}
	m.NumGlyphs = binary.BigEndian.Uint16(maxp[4:6])
	hhea := f.table("hhea")
	if len(hhea) < 36 {
		return nil, errors.New("hhea: table too small")
	}
	m.numLongMetrics = int(binary.BigEndian.Uint16(hhea[34:36]))
	if m.numLongMetrics == 0 || m.numLongMetrics > int(m.NumGlyphs) {
		return nil, fmt.Errorf("hhea: numberOfHMetrics %d inconsistent with numGlyphs %d", m.numLongMetrics, m.NumGlyphs)
	}
	hmtx := f.table("hmtx")
	need := m.numLongMetrics*4 + (int(m.NumGlyphs)-m.numLongMetrics)*2
	if len(hmtx) < need {
		return nil, fmt.Errorf("hmtx: table too small: need %d bytes, have %d", need, len(hmtx))
	}
	m.advances = make([]uint16, m.numLongMetrics)
	for i := 0; i < m.numLongMetrics; i++ {
		m.advances[i] = binary.BigEndian.Uint16(hmtx[i*4:])
	}
	cm, err := f.loadCmap()
	if err != nil {
		return nil, err
	}
	m.cmap = cm
	return m, nil
}

// GlyphID maps r to a glyph; missing runes yield glyph 0 and ok=false.
func (m *Metrics) GlyphID(r rune) (id uint16, ok bool) {
	id = m.cmap.glyphID(r)
	if id >= m.NumGlyphs {
		return 0, false
	}
	return id, id != 0
}

// Advance returns the horizontal advance of a glyph in font units.
// Glyphs past the long metric run reuse the last advance width.
func (m *Metrics) Advance(id uint16) uint16 {
	if int(id) >= m.numLongMetrics {
		return m.advances[m.numLongMetrics-1]
	}
	return m.advances[id]
}

// LoadKern parses a legacy 'kern' table. Only version 0 with exactly
// one horizontal format 0 subtable is supported; a missing table means
// zero adjustment, anything else is an explicit error.
func (f *Font) LoadKern(m *Metrics) error {
	data := f.table("kern")
	if data == nil {
		m.kernLoaded = true
		return nil
	}
	if len(data) < 4 {
		return errors.New("kern: table too small")
	}
	if binary.BigEndian.Uint16(data[0:2]) != 0 {
		return errors.New("kern: only version 0 is supported")
	}
	nTables := int(binary.BigEndian.Uint16(data[2:4]))
	if nTables != 1 {
		return fmt.Errorf("kern: expected exactly one subtable, found %d", nTables)
	}
	if len(data) < 10 {
		return errors.New("kern: subtable header exceeds table")
	}
	sub := data[4:]
	if binary.BigEndian.Uint16(sub[0:2]) != 0 {
		return errors.New("kern: subtable version must be 0")
	}
	length := int(binary.BigEndian.Uint16(sub[2:4]))
	if length < 6 || length > len(sub) {
		return errors.New("kern: subtable length out of range")
	}
	coverage := binary.BigEndian.Uint16(sub[4:6])
	if coverage>>8 != 0 {
		return fmt.Errorf("kern: only format 0 subtables are supported, found format %d", coverage>>8)
	}
	if coverage&0x01 == 0 {
		return errors.New("kern: subtable is not horizontal")
	}
	if coverage&0xFE != 0 {
		return fmt.Errorf("kern: unsupported coverage flags 0x%04x", coverage)
	}
	body := sub[6:length]
	if len(body) < 8 {
		return errors.New("kern: format 0 header too small")
	}
	nPairs := int(binary.BigEndian.Uint16(body[0:2]))
	if 8+nPairs*6 > len(body) {
		return errors.New("kern: pair records exceed subtable length")
	}
	m.kern = make(map[uint32]int16, nPairs)
	for i := 0; i < nPairs; i++ {
		rec := body[8+i*6:]
		left := binary.BigEndian.Uint16(rec[0:2])
		right := binary.BigEndian.Uint16(rec[2:4])
		value := int16(binary.BigEndian.Uint16(rec[4:6]))
		m.kern[uint32(left)<<16|uint32(right)] = value
	}
	m.kernLoaded = true
	return nil
}

// Kern returns the kerning adjustment between two glyphs in font units.
func (m *Metrics) Kern(left, right uint16) int16 {
	if !m.kernLoaded {
		return 0
	}
	return m.kern[uint32(left)<<16|uint32(right)]
}

// sortedPairs is a helper for deterministic tests.
func (m *Metrics) sortedKernPairs() []uint32 {
	keys := make([]uint32, 0, len(m.kern))
	for k := range m.kern {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
