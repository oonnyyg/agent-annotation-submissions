// Command bmpintake ingests a legacy BMP file described by a JSON config
// and publishes a binary PGM grayscale image plus a JSON summary.
//
// Usage: go run ./cmd/bmpintake <config.json>
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"example.com/bmp-rle-raster-intake/internal/bmp"
	"example.com/bmp-rle-raster-intake/internal/intake"
)

type config struct {
	Input         string `json:"input"`
	OutputPGM     string `json:"outputPGM"`
	OutputSummary string `json:"outputSummary"`
	MaxPixels     int64  `json:"maxPixels"`
	Crop          *struct {
		X0 int `json:"x0"`
		Y0 int `json:"y0"`
		X1 int `json:"x1"`
		Y1 int `json:"y1"`
	} `json:"crop"`
}

type summary struct {
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	Histogram [256]uint64 `json:"histogram"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bmpintake <config.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "bmpintake:", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if cfg.Input == "" || cfg.OutputPGM == "" || cfg.OutputSummary == "" {
		return errors.New("config requires input, outputPGM and outputSummary")
	}
	if cfg.MaxPixels <= 0 {
		return errors.New("config requires maxPixels > 0")
	}

	data, err := os.ReadFile(cfg.Input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	img, err := bmp.Decode(data)
	if err != nil {
		return fmt.Errorf("decode %s: %w", cfg.Input, err)
	}
	if int64(img.Width)*int64(img.Height) > cfg.MaxPixels {
		return fmt.Errorf("image %dx%d exceeds maxPixels %d", img.Width, img.Height, cfg.MaxPixels)
	}

	gray := intake.ToGray(img)
	if cfg.Crop != nil {
		gray, err = intake.Crop(gray, intake.Rect{
			X0: cfg.Crop.X0, Y0: cfg.Crop.Y0, X1: cfg.Crop.X1, Y1: cfg.Crop.Y1,
		})
		if err != nil {
			return err
		}
	}
	sum := summary{Width: gray.Width, Height: gray.Height, Histogram: intake.Histogram(gray)}
	sumData, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}

	// Publish only after every validation succeeded, and never overwrite
	// existing targets.
	if err := publish(cfg.OutputPGM, intake.PGM(gray)); err != nil {
		return err
	}
	if err := publish(cfg.OutputSummary, sumData); err != nil {
		os.Remove(cfg.OutputPGM)
		return err
	}
	fmt.Printf("published %s (%dx%d) and %s\n", cfg.OutputPGM, gray.Width, gray.Height, cfg.OutputSummary)
	return nil
}

// publish writes data to path atomically, refusing to overwrite an
// existing file.
func publish(path string, data []byte) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing file %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bmpintake-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("publish %s: %w", path, err)
	}
	return nil
}
