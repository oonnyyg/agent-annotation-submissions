package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBMP builds a minimal 24-bit bottom-up BMP.
func writeBMP(t *testing.T, path string, w, h int32, pixels []byte) {
	t.Helper()
	off := 14 + 40
	total := off + len(pixels)
	buf := make([]byte, total)
	buf[0], buf[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(buf[2:], uint32(total))
	binary.LittleEndian.PutUint32(buf[10:], uint32(off))
	binary.LittleEndian.PutUint32(buf[14:], 40)
	binary.LittleEndian.PutUint32(buf[18:], uint32(w))
	binary.LittleEndian.PutUint32(buf[22:], uint32(h))
	binary.LittleEndian.PutUint16(buf[26:], 1)
	binary.LittleEndian.PutUint16(buf[28:], 24)
	copy(buf[off:], pixels)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, path string, cfg map[string]any) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.bmp")
	// 2x2: bottom row blue, white; top row black, red.
	pixels := []byte{
		255, 0, 0, 255, 255, 255, 0, 0,
		0, 0, 0, 0, 0, 255, 0, 0,
	}
	writeBMP(t, in, 2, 2, pixels)
	pgmPath := filepath.Join(dir, "out.pgm")
	sumPath := filepath.Join(dir, "out.json")
	cfgPath := filepath.Join(dir, "cfg.json")
	writeConfig(t, cfgPath, map[string]any{
		"input": in, "outputPGM": pgmPath, "outputSummary": sumPath,
		"maxPixels": 100,
		"crop":      map[string]int{"x0": 1, "y0": 0, "x1": 2, "y1": 2},
	})
	if err := run(cfgPath); err != nil {
		t.Fatal(err)
	}
	pgm, err := os.ReadFile(pgmPath)
	if err != nil {
		t.Fatal(err)
	}
	// Cropped column: top red -> 76, bottom white -> 255.
	want := append([]byte("P5\n1 2\n255\n"), 76, 255)
	if string(pgm) != string(want) {
		t.Fatalf("pgm = %q, want %q", pgm, want)
	}
	sumData, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	var sum struct {
		Width     int
		Height    int
		Histogram [256]uint64
	}
	if err := json.Unmarshal(sumData, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Width != 1 || sum.Height != 2 || sum.Histogram[76] != 1 || sum.Histogram[255] != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	// Second run must refuse to overwrite.
	if err := run(cfgPath); err == nil || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func TestMaxPixelsEnforced(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.bmp")
	writeBMP(t, in, 2, 2, make([]byte, 16))
	cfgPath := filepath.Join(dir, "cfg.json")
	pgmPath := filepath.Join(dir, "out.pgm")
	writeConfig(t, cfgPath, map[string]any{
		"input": in, "outputPGM": pgmPath,
		"outputSummary": filepath.Join(dir, "out.json"),
		"maxPixels":     3,
	})
	if err := run(cfgPath); err == nil || !strings.Contains(err.Error(), "maxPixels") {
		t.Fatalf("expected maxPixels rejection, got %v", err)
	}
	if _, err := os.Stat(pgmPath); !os.IsNotExist(err) {
		t.Fatal("no output should be published on failure")
	}
}

func TestCorruptInputReportsOffset(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.bmp")
	if err := os.WriteFile(in, []byte("not a bmp"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "cfg.json")
	writeConfig(t, cfgPath, map[string]any{
		"input": in, "outputPGM": filepath.Join(dir, "o.pgm"),
		"outputSummary": filepath.Join(dir, "o.json"), "maxPixels": 100,
	})
	err := run(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "byte") {
		t.Fatalf("expected byte-offset error, got %v", err)
	}
}
