package bmpintake

import (
	"fmt"
	"io"
)

// Rect is a half-open rectangle with origin at the top-left corner.
type Rect struct {
	X, Y, W, H int
}

// Gray is a cropped grayscale raster.
type Gray struct {
	Width, Height int
	Pix           []byte
}

// CropGray crops the image to r (top-left origin, half-open) and
// converts to grayscale using (299R + 587G + 114B + 500) / 1000.
func (img *Image) CropGray(r Rect) (*Gray, error) {
	if r.X < 0 || r.Y < 0 || r.W <= 0 || r.H <= 0 {
		return nil, fmt.Errorf("bmp: invalid crop rectangle %+v", r)
	}
	if r.X+r.W > img.Width || r.Y+r.H > img.Height {
		return nil, fmt.Errorf("bmp: crop rectangle (%d,%d %dx%d) exceeds image bounds %dx%d",
			r.X, r.Y, r.W, r.H, img.Width, img.Height)
	}
	g := &Gray{Width: r.W, Height: r.H, Pix: make([]byte, r.W*r.H)}
	for row := 0; row < r.H; row++ {
		srcRow := r.Y + row
		for col := 0; col < r.W; col++ {
			srcCol := r.X + col
			var rr, gg, bb uint8
			if img.Palette != nil {
				q := img.Palette[img.Pixels[srcRow*img.Width+srcCol]]
				rr, gg, bb = q.R, q.G, q.B
			} else {
				o := (srcRow*img.Width + srcCol) * 3
				rr, gg, bb = img.RGB[o], img.RGB[o+1], img.RGB[o+2]
			}
			v := (299*int(rr) + 587*int(gg) + 114*int(bb) + 500) / 1000
			g.Pix[row*r.W+col] = byte(v)
		}
	}
	return g, nil
}

// Histogram returns the complete 256-bin grayscale histogram.
func (g *Gray) Histogram() [256]uint64 {
	var hist [256]uint64
	for _, v := range g.Pix {
		hist[v]++
	}
	return hist
}

// WritePGM writes a binary (P5) PGM with max value 255.
func (g *Gray) WritePGM(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "P5\n%d %d\n255\n", g.Width, g.Height); err != nil {
		return err
	}
	_, err := w.Write(g.Pix)
	return err
}
