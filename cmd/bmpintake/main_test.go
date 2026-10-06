package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func buildRLE8BMP() []byte {
	pal := [][3]byte{{0, 0, 0}, {255, 255, 255}}
	stream := []byte{2, 1, 0, 0, 1, 1, 0, 1} // 2x2: runs + EOL + EOB
	var buf bytes.Buffer
	off := uint32(14 + 40 + len(pal)*4)
	buf.Write([]byte("BM"))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, off)
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, int32(2))
	binary.Write(&buf, binary.LittleEndian, int32(2))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(8))
	binary.Write(&buf, binary.LittleEndian, uint32(1))
	binary.Write(&buf, binary.LittleEndian, uint32(len(stream)))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(2))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	for _, p := range pal {
		buf.Write([]byte{p[2], p[1], p[0], 0})
	}
	buf.Write(stream)
	return buf.Bytes()
}

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.bmp")
	pgm := filepath.Join(dir, "out.pgm")
	sum := filepath.Join(dir, "out.json")
	cfgPath := filepath.Join(dir, "cfg.json")

	if err := os.WriteFile(in, buildRLE8BMP(), 0o666); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"input": in, "pgm_output": pgm, "summary_output": sum,
		"max_pixels": 100,
		"crop":       map[string]int{"x": 0, "y": 0, "width": 1, "height": 2},
	}
	cfgData, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, cfgData, 0o666); err != nil {
		t.Fatal(err)
	}

	if err := run(cfgPath); err != nil {
		t.Fatal(err)
	}
	pgmData, err := os.ReadFile(pgm)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pgmData, []byte("P5\n1 2\n255\n")) {
		t.Fatalf("pgm header %q", pgmData[:10])
	}
	var s struct {
		Width     int         `json:"width"`
		Height    int         `json:"height"`
		Histogram [256]uint64 `json:"histogram"`
	}
	sumData, err := os.ReadFile(sum)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sumData, &s); err != nil {
		t.Fatal(err)
	}
	if s.Width != 1 || s.Height != 2 {
		t.Fatalf("summary size %dx%d", s.Width, s.Height)
	}
	if s.Histogram[0]+s.Histogram[255] != 2 {
		t.Fatalf("histogram does not cover 2 pixels")
	}

	// Second run must refuse to overwrite existing outputs.
	if err := run(cfgPath); err == nil {
		t.Fatal("second run overwrote existing outputs")
	}
}

func TestRunRejectsOverLimit(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.bmp")
	cfgPath := filepath.Join(dir, "cfg.json")
	os.WriteFile(in, buildRLE8BMP(), 0o666)
	cfg := map[string]any{
		"input": in, "pgm_output": filepath.Join(dir, "o.pgm"),
		"summary_output": filepath.Join(dir, "o.json"), "max_pixels": 3,
	}
	cfgData, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, cfgData, 0o666)
	if err := run(cfgPath); err == nil {
		t.Fatal("accepted image over pixel limit")
	}
	if _, err := os.Stat(filepath.Join(dir, "o.pgm")); !os.IsNotExist(err) {
		t.Fatal("output published despite failed validation")
	}
}
