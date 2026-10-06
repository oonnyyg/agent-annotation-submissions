// Package intake converts a decoded BMP raster into a grayscale PGM
// image plus a JSON summary, applying an optional crop rectangle.
package intake

import (
	"fmt"

	"example.com/bmp-rle-raster-intake/internal/bmp"
)

// Rect is a half-open rectangle with the origin at the top-left corner.
type Rect struct {
	X0, Y0, X1, Y1 int
}

// GrayImage is a grayscale raster, one byte per pixel, top-down.
type GrayImage struct {
	Width  int
	Height int
	Pix    []byte
}

// ToGray converts to grayscale using (299R + 587G + 114B + 500) / 1000.
func ToGray(img *bmp.Image) *GrayImage {
	n := img.Width * img.Height
	out := &GrayImage{Width: img.Width, Height: img.Height, Pix: make([]byte, n)}
	for i := 0; i < n; i++ {
		r := int(img.Pix[i*3])
		g := int(img.Pix[i*3+1])
		b := int(img.Pix[i*3+2])
		out.Pix[i] = byte((299*r + 587*g + 114*b + 500) / 1000)
	}
	return out
}

// Crop extracts the half-open rectangle r from g. The rectangle must lie
// fully inside the image and be non-empty.
func Crop(g *GrayImage, r Rect) (*GrayImage, error) {
	if r.X0 < 0 || r.Y0 < 0 || r.X1 > g.Width || r.Y1 > g.Height || r.X0 >= r.X1 || r.Y0 >= r.Y1 {
		return nil, fmt.Errorf("crop rectangle [%d,%d)-[%d,%d) outside image %dx%d or empty",
			r.X0, r.Y0, r.X1, r.Y1, g.Width, g.Height)
	}
	w, h := r.X1-r.X0, r.Y1-r.Y0
	out := &GrayImage{Width: w, Height: h, Pix: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		src := (r.Y0+y)*g.Width + r.X0
		copy(out.Pix[y*w:], g.Pix[src:src+w])
	}
	return out, nil
}

// Histogram returns the 256-bin gray histogram of g.
func Histogram(g *GrayImage) [256]uint64 {
	var hist [256]uint64
	for _, v := range g.Pix {
		hist[v]++
	}
	return hist
}

// PGM encodes g as a binary (P5) PGM image.
func PGM(g *GrayImage) []byte {
	header := fmt.Sprintf("P5\n%d %d\n255\n", g.Width, g.Height)
	out := make([]byte, 0, len(header)+len(g.Pix))
	out = append(out, header...)
	out = append(out, g.Pix...)
	return out
}
