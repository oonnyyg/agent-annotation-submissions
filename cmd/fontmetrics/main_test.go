package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func be16(b []byte, off int, v uint16) { binary.BigEndian.PutUint16(b[off:], v) }

func testFont(t *testing.T) []byte {
	t.Helper()
	head := make([]byte, 54)
	be16(head, 18, 1000)
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	be16(maxp, 4, 4)
	hhea := make([]byte, 36)
	be16(hhea, 34, 3) // 3 long metrics, glyph 3 compressed
	hmtx := make([]byte, 3*4+2)
	for i, a := range []uint16{500, 600, 700} {
		be16(hmtx, i*4, a)
	}
	// cmap format 12: A->1, V->2, B->3, U+1F600->3
	cmapSub := make([]byte, 16+4*12)
	be16(cmapSub, 0, 12)
	binary.BigEndian.PutUint32(cmapSub[4:8], uint32(len(cmapSub)))
	binary.BigEndian.PutUint32(cmapSub[12:16], 4)
	groups := [][3]uint32{{'A', 'A', 1}, {'B', 'B', 3}, {'V', 'V', 2}, {0x1F600, 0x1F600, 3}}
	for i, g := range groups {
		o := 16 + i*12
		binary.BigEndian.PutUint32(cmapSub[o:], g[0])
		binary.BigEndian.PutUint32(cmapSub[o+4:], g[1])
		binary.BigEndian.PutUint32(cmapSub[o+8:], g[2])
	}
	cmap := make([]byte, 4+8)
	be16(cmap, 2, 1)
	be16(cmap, 4, 3)
	be16(cmap, 6, 10)
	binary.BigEndian.PutUint32(cmap[8:12], 12)
	cmap = append(cmap, cmapSub...)
	// kern: (1,2) -> -80
	kernBody := make([]byte, 8+6)
	be16(kernBody, 0, 1)
	be16(kernBody, 8, 1)
	be16(kernBody, 10, 2)
	be16(kernBody, 12, 0xFFB0)
	kern := make([]byte, 4+6)
	be16(kern, 2, 1)
	be16(kern, 6, uint16(6+len(kernBody)))
	be16(kern, 8, 0x0001)
	kern = append(kern, kernBody...)

	tables := map[string][]byte{
		"head": head, "maxp": maxp, "hhea": hhea, "hmtx": hmtx,
		"cmap": cmap, "kern": kern,
	}
	order := []string{"cmap", "head", "hhea", "hmtx", "kern", "maxp"}
	out := make([]byte, 12+len(order)*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	be16(out, 4, uint16(len(order)))
	off := 12 + len(order)*16
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

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "test.ttf")
	if err := os.WriteFile(fontPath, testFont(t), 0o644); err != nil {
		t.Fatal(err)
	}
	reqPath := filepath.Join(dir, "req.json")
	req := `{"text": "AV\uD83D\uDE00?", "size": 10, "kern": true}`
	if err := os.WriteFile(reqPath, []byte(req), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := run(fontPath, reqPath, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	var resp response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if len(resp.Glyphs) != 4 {
		t.Fatalf("got %d glyphs; want 4", len(resp.Glyphs))
	}
	want := []struct {
		glyph uint16
		miss  bool
		x     float64
		adj   float64
		adv   float64
	}{
		{1, false, 0, 0, 6},
		{2, false, 5.2, -0.8, 7},
		{3, false, 12.2, 0, 7}, // non-BMP, compressed advance
		{0, true, 19.2, 0, 5},  // missing glyph
	}
	for i, w := range want {
		g := resp.Glyphs[i]
		if g.Glyph != w.glyph || g.Missing != w.miss {
			t.Errorf("glyph %d: got id=%d missing=%v; want id=%d missing=%v", i, g.Glyph, g.Missing, w.glyph, w.miss)
		}
		if math.Abs(g.X-w.x) > 1e-9 || math.Abs(g.Adjustment-w.adj) > 1e-9 || math.Abs(g.Advance-w.adv) > 1e-9 {
			t.Errorf("glyph %d: got x=%v adj=%v adv=%v; want x=%v adj=%v adv=%v",
				i, g.X, g.Adjustment, g.Advance, w.x, w.adj, w.adv)
		}
	}
	if math.Abs(resp.TotalWidth-24.2) > 1e-9 {
		t.Errorf("TotalWidth = %v; want 24.2", resp.TotalWidth)
	}
}
