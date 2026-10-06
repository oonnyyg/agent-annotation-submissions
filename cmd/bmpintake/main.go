// Command bmpintake decodes a legacy BMP into a binary PGM grayscale
// image plus a JSON summary, driven by a JSON config file.
//
// Usage: go run ./cmd/bmpintake config.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	bmp "example.com/bmp-rle-raster-intake"
)

type config struct {
	Input         string `json:"input"`
	PGMOutput     string `json:"pgm_output"`
	SummaryOutput string `json:"summary_output"`
	MaxPixels     int64  `json:"max_pixels"`
	Crop          *struct {
		X      int `json:"x"`
		Y      int `json:"y"`
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"crop"`
}

type summary struct {
	SourceWidth  int         `json:"source_width"`
	SourceHeight int         `json:"source_height"`
	Width        int         `json:"width"`
	Height       int         `json:"height"`
	Histogram    [256]uint64 `json:"histogram"`
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bmpintake:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bmpintake <config.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fail(err)
	}
}

func run(configPath string) error {
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.Input == "" || cfg.PGMOutput == "" || cfg.SummaryOutput == "" {
		return fmt.Errorf("config: input, pgm_output and summary_output are required")
	}
	if cfg.MaxPixels <= 0 {
		return fmt.Errorf("config: max_pixels must be positive")
	}

	data, err := os.ReadFile(cfg.Input)
	if err != nil {
		return err
	}
	img, err := bmp.Decode(data)
	if err != nil {
		return err
	}
	if int64(img.Width)*int64(img.Height) > cfg.MaxPixels {
		return fmt.Errorf("image has %d pixels, exceeding max_pixels %d",
			int64(img.Width)*int64(img.Height), cfg.MaxPixels)
	}

	rect := bmp.Rect{X: 0, Y: 0, W: img.Width, H: img.Height}
	if cfg.Crop != nil {
		rect = bmp.Rect{X: cfg.Crop.X, Y: cfg.Crop.Y, W: cfg.Crop.Width, H: cfg.Crop.Height}
	}
	gray, err := img.CropGray(rect)
	if err != nil {
		return err
	}

	sum := summary{
		SourceWidth:  img.Width,
		SourceHeight: img.Height,
		Width:        gray.Width,
		Height:       gray.Height,
		Histogram:    gray.Histogram(),
	}
	sumData, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}

	// All validation passed: publish outputs without overwriting.
	if err := writeNew(cfg.PGMOutput, func(f *os.File) error { return gray.WritePGM(f) }); err != nil {
		return err
	}
	if err := writeNew(cfg.SummaryOutput, func(f *os.File) error {
		_, err := f.Write(append(sumData, '\n'))
		return err
	}); err != nil {
		return err
	}
	fmt.Printf("decoded %s: %dx%d -> cropped %dx%d\n", cfg.Input,
		img.Width, img.Height, gray.Width, gray.Height)
	return nil
}

// writeNew creates path exclusively (never overwrites) and runs fn.
func writeNew(path string, fn func(*os.File) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return fmt.Errorf("publish %s: %w", path, err)
	}
	if err := fn(f); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("publish %s: %w", path, err)
	}
	return f.Close()
}
