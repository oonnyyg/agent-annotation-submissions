package sfnt

import (
	"encoding/binary"
	"testing"
)

// buildFont assembles a minimal sfnt from the given tables.
func buildFont(t *testing.T, tables map[string][]byte, order []string) []byte {
	t.Helper()
	n := len(order)
	out := make([]byte, 12+n*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(n))
	off := 12 + n*16
	for i, tag := range order {
		body := tables[tag]
		rec := out[12+i*16:]
		copy(rec[0:4], tag)
		binary.BigEndian.PutUint32(rec[8:12], uint32(off))
		binary.BigEndian.PutUint32(rec[12:16], uint32(len(body)))
		out = append(out, body...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		off = len(out)
	}
	return out
}

func headTable(unitsPerEm uint16) []byte {
	b := make([]byte, 54)
	binary.BigEndian.PutUint16(b[18:20], unitsPerEm)
	return b
}

func maxpTable(numGlyphs uint16) []byte {
	b := make([]byte, 6)
	binary.BigEndian.PutUint32(b[0:4], 0x00010000)
	binary.BigEndian.PutUint16(b[4:6], numGlyphs)
	return b
}

func hheaTable(numLong uint16) []byte {
	b := make([]byte, 36)
	binary.BigEndian.PutUint16(b[34:36], numLong)
	return b
}

// hmtxTable builds long metrics for the given advances plus count
// compressed left-side-bearing-only entries.
func hmtxTable(advances []uint16, compressed int) []byte {
	b := make([]byte, 0, len(advances)*4+compressed*2)
	for _, a := range advances {
		b = binary.BigEndian.AppendUint16(b, a)
		b = binary.BigEndian.AppendUint16(b, 0)
	}
	for i := 0; i < compressed; i++ {
		b = binary.BigEndian.AppendUint16(b, 0)
	}
	return b
}

func cmapHeader(records ...[3]uint32) []byte {
	b := make([]byte, 4, 4+len(records)*8)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(records)))
	for _, r := range records {
		b = binary.BigEndian.AppendUint16(b, uint16(r[0]))
		b = binary.BigEndian.AppendUint16(b, uint16(r[1]))
		b = binary.BigEndian.AppendUint32(b, r[2])
	}
	return b
}

// cmapFormat4For maps single BMP codepoints to glyphs via idDelta=0
// segments with explicit glyphIdArray entries.
func cmapFormat4For(mapping map[uint16]uint16) []byte {
	segs := len(mapping) + 1 // plus 0xFFFF terminator
	length := 16 + segs*8 + len(mapping)*2
	b := make([]byte, length)
	binary.BigEndian.PutUint16(b[0:2], 4)
	binary.BigEndian.PutUint16(b[2:4], uint16(length))
	binary.BigEndian.PutUint16(b[6:8], uint16(segs*2))
	codes := make([]uint16, 0, len(mapping))
	for c := range mapping {
		codes = append(codes, c)
	}
	for i := 0; i < len(codes); i++ {
		for j := i + 1; j < len(codes); j++ {
			if codes[j] < codes[i] {
				codes[i], codes[j] = codes[j], codes[i]
			}
		}
	}
	endOff := 14
	startOff := 16 + segs*2
	deltaOff := 16 + segs*4
	roOff := 16 + segs*6
	glyphOff := 16 + segs*8
	for i, c := range codes {
		binary.BigEndian.PutUint16(b[endOff+i*2:], c)
		binary.BigEndian.PutUint16(b[startOff+i*2:], c)
		// idRangeOffset relative to its own word, pointing at glyph i.
		binary.BigEndian.PutUint16(b[roOff+i*2:], uint16((glyphOff+i*2)-(roOff+i*2)))
		binary.BigEndian.PutUint16(b[glyphOff+i*2:], mapping[c])
	}
	binary.BigEndian.PutUint16(b[endOff+(segs-1)*2:], 0xFFFF)
	binary.BigEndian.PutUint16(b[startOff+(segs-1)*2:], 0xFFFF)
	binary.BigEndian.PutUint16(b[deltaOff+(segs-1)*2:], 1)
	return b
}

// cmapFormat12For maps inclusive ranges to sequential glyphs.
func cmapFormat12For(groups [][3]uint32) []byte {
	b := make([]byte, 16, 16+len(groups)*12)
	binary.BigEndian.PutUint16(b[0:2], 12)
	binary.BigEndian.PutUint32(b[4:8], uint32(16+len(groups)*12))
	binary.BigEndian.PutUint32(b[12:16], uint32(len(groups)))
	for _, g := range groups {
		b = binary.BigEndian.AppendUint32(b, g[0])
		b = binary.BigEndian.AppendUint32(b, g[1])
		b = binary.BigEndian.AppendUint32(b, g[2])
	}
	return b
}

func baseTables(cmap []byte, numGlyphs uint16, advances []uint16) map[string][]byte {
	compressed := int(numGlyphs) - len(advances)
	return map[string][]byte{
		"head": headTable(1000),
		"maxp": maxpTable(numGlyphs),
		"hhea": hheaTable(uint16(len(advances))),
		"hmtx": hmtxTable(advances, compressed),
		"cmap": cmap,
	}
}

func orderOf(tables map[string][]byte) []string {
	order := make([]string, 0, len(tables))
	for tag := range tables {
		order = append(order, tag)
	}
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j] < order[i] {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	return order
}

func parseMetrics(t *testing.T, tables map[string][]byte) *Metrics {
	t.Helper()
	data := buildFont(t, tables, orderOf(tables))
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m, err := f.LoadMetrics()
	if err != nil {
		t.Fatalf("LoadMetrics: %v", err)
	}
	return m
}

func TestFormat4MappingAndMissing(t *testing.T) {
	sub := cmapFormat4For(map[uint16]uint16{'A': 1, 'B': 2})
	cmap := cmapHeader([3]uint32{3, 1, 4 + 8})
	cmap = append(cmap, sub...)
	m := parseMetrics(t, baseTables(cmap, 3, []uint16{500, 600, 700}))
	if id, ok := m.GlyphID('A'); !ok || id != 1 {
		t.Errorf("GlyphID('A') = %d, %v; want 1, true", id, ok)
	}
	if id, ok := m.GlyphID('Z'); ok || id != 0 {
		t.Errorf("GlyphID('Z') = %d, %v; want 0, false", id, ok)
	}
}

func TestFormat12NonBMP(t *testing.T) {
	sub := cmapFormat12For([][3]uint32{{0x1F600, 0x1F601, 5}})
	cmap := cmapHeader([3]uint32{3, 10, 4 + 8})
	cmap = append(cmap, sub...)
	m := parseMetrics(t, baseTables(cmap, 8, []uint16{500}))
	if id, ok := m.GlyphID(0x1F600); !ok || id != 5 {
		t.Errorf("GlyphID(U+1F600) = %d, %v; want 5, true", id, ok)
	}
	if id, ok := m.GlyphID(0x1F601); !ok || id != 6 {
		t.Errorf("GlyphID(U+1F601) = %d, %v; want 6, true", id, ok)
	}
	if _, ok := m.GlyphID(0x1F602); ok {
		t.Error("GlyphID(U+1F602) should be missing")
	}
}

func TestFormat12PreferredOverFormat4(t *testing.T) {
	sub4 := cmapFormat4For(map[uint16]uint16{'A': 1})
	sub12 := cmapFormat12For([][3]uint32{{'A', 'A', 9}})
	cmap := cmapHeader([3]uint32{3, 1, 4 + 16}, [3]uint32{3, 10, 4 + 16 + uint32(len(sub4))})
	cmap = append(cmap, sub4...)
	cmap = append(cmap, sub12...)
	m := parseMetrics(t, baseTables(cmap, 10, []uint16{500}))
	if id, _ := m.GlyphID('A'); id != 9 {
		t.Errorf("GlyphID('A') = %d; want 9 from format 12", id)
	}
}

func TestCompressedHmtx(t *testing.T) {
	sub := cmapFormat12For([][3]uint32{{'A', 'C', 1}})
	cmap := cmapHeader([3]uint32{3, 10, 4 + 8})
	cmap = append(cmap, sub...)
	// 4 glyphs, only 2 long metrics; glyphs 2..3 reuse advance 400.
	m := parseMetrics(t, baseTables(cmap, 4, []uint16{500, 400}))
	if got := m.Advance(3); got != 400 {
		t.Errorf("Advance(3) = %d; want 400 (last long metric)", got)
	}
	if got := m.Advance(0); got != 500 {
		t.Errorf("Advance(0) = %d; want 500", got)
	}
}

func TestDuplicateSubtableRejected(t *testing.T) {
	sub := cmapFormat12For([][3]uint32{{'A', 'A', 1}})
	cmap := cmapHeader([3]uint32{3, 10, 4 + 16}, [3]uint32{3, 10, 4 + 16 + uint32(len(sub))})
	cmap = append(cmap, sub...)
	cmap = append(cmap, sub...)
	tables := baseTables(cmap, 2, []uint16{500})
	data := buildFont(t, tables, orderOf(tables))
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := f.LoadMetrics(); err == nil {
		t.Error("expected duplicate (3,10) subtable error")
	}
}

func TestDuplicateTableRejected(t *testing.T) {
	tables := baseTables(cmapHeader(), 1, []uint16{500})
	order := append(orderOf(tables), "head")
	if _, err := Parse(buildFont(t, tables, order)); err == nil {
		t.Error("expected duplicate table tag error")
	}
}

func TestCollectionAndVariableRejected(t *testing.T) {
	tables := baseTables(cmapHeader(), 1, []uint16{500})
	data := buildFont(t, tables, orderOf(tables))
	copy(data[0:4], "ttcf")
	if _, err := Parse(data); err == nil {
		t.Error("expected collection rejection")
	}
	data = buildFont(t, tables, orderOf(tables))
	copy(data[0:4], "OTTO")
	if _, err := Parse(data); err == nil {
		t.Error("expected CFF rejection")
	}
	tables["fvar"] = make([]byte, 16)
	data = buildFont(t, tables, orderOf(tables))
	if _, err := Parse(data); err == nil {
		t.Error("expected variable font rejection")
	}
}

func TestOutOfRangeTableRejected(t *testing.T) {
	tables := baseTables(cmapHeader(), 1, []uint16{500})
	data := buildFont(t, tables, orderOf(tables))
	// Corrupt the first table record's length to exceed the file.
	binary.BigEndian.PutUint32(data[12+12:12+16], uint32(len(data))*2)
	if _, err := Parse(data); err == nil {
		t.Error("expected out-of-range table error")
	}
}
