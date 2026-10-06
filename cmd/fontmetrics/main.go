// Command fontmetrics maps text to glyphs and reports per-glyph pen
// positions, kerning adjustments and the total advance of a line.
// It draws no outlines and performs no shaping.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"example.com/sfnt-text-metrics/internal/sfnt"
)

type request struct {
	Text string  `json:"text"`
	Size float64 `json:"size"`
	Kern bool    `json:"kern"`
}

type glyphOut struct {
	Rune       string  `json:"rune"`
	Codepoint  int32   `json:"codepoint"`
	Glyph      uint16  `json:"glyph"`
	Missing    bool    `json:"missing"`
	X          float64 `json:"x"`
	Adjustment float64 `json:"adjustment"`
	Advance    float64 `json:"advance"`
}

type response struct {
	UnitsPerEm uint16     `json:"unitsPerEm"`
	Size       float64    `json:"size"`
	Kerning    bool       `json:"kerning"`
	Glyphs     []glyphOut `json:"glyphs"`
	TotalWidth float64    `json:"totalWidth"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: fontmetrics <font.ttf> <request.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fontmetrics:", err)
		os.Exit(1)
	}
}

func run(fontPath, reqPath string, out io.Writer) error {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return err
	}
	reqData, err := os.ReadFile(reqPath)
	if err != nil {
		return err
	}
	var req request
	if err := json.Unmarshal(reqData, &req); err != nil {
		return fmt.Errorf("request JSON: %w", err)
	}
	if req.Size <= 0 {
		return fmt.Errorf("request: size must be positive, got %v", req.Size)
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		return err
	}
	m, err := font.LoadMetrics()
	if err != nil {
		return err
	}
	if req.Kern {
		if err := font.LoadKern(m); err != nil {
			return err
		}
	}
	scale := req.Size / float64(m.UnitsPerEm)
	resp := response{
		UnitsPerEm: m.UnitsPerEm,
		Size:       req.Size,
		Kerning:    req.Kern,
	}
	var pen float64
	var prev uint16
	for i, r := range req.Text {
		id, ok := m.GlyphID(r)
		adj := 0.0
		if req.Kern && i > 0 {
			adj = float64(m.Kern(prev, id)) * scale
		}
		pen += adj // adjustment lands before the next glyph is placed
		adv := float64(m.Advance(id)) * scale
		resp.Glyphs = append(resp.Glyphs, glyphOut{
			Rune:       string(r),
			Codepoint:  r,
			Glyph:      id,
			Missing:    !ok,
			X:          pen,
			Adjustment: adj,
			Advance:    adv,
		})
		pen += adv
		prev = id
	}
	resp.TotalWidth = pen
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
