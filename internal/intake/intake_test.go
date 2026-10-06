package intake

import (
	"testing"

	"example.com/bmp-rle-raster-intake/internal/bmp"
)

func rgbImage(w, h int, pix []byte) *bmp.Image {
	return &bmp.Image{Width: w, Height: h, Pix: pix}
}

func TestToGrayRounding(t *testing.T) {
	img := rgbImage(2, 2, []byte{
		255, 255, 255, // white -> 255
		0, 0, 0, // black -> 0
		255, 0, 0, // red -> (299*255+500)/1000 = 76
		0, 255, 0, // green -> (587*255+500)/1000 = 150
	})
	g := ToGray(img)
	want := []byte{255, 0, 76, 150}
	for i, v := range want {
		if g.Pix[i] != v {
			t.Fatalf("pix[%d] = %d, want %d", i, g.Pix[i], v)
		}
	}
}

func TestCropHalfOpen(t *testing.T) {
	g := &GrayImage{Width: 4, Height: 3, Pix: []byte{
		0, 1, 2, 3,
		4, 5, 6, 7,
		8, 9, 10, 11,
	}}
	out, err := Crop(g, Rect{X0: 1, Y0: 1, X1: 3, Y1: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{5, 6, 9, 10}
	if out.Width != 2 || out.Height != 2 {
		t.Fatalf("size = %dx%d", out.Width, out.Height)
	}
	for i, v := range want {
		if out.Pix[i] != v {
			t.Fatalf("pix[%d] = %d, want %d", i, out.Pix[i], v)
		}
	}
}

func TestCropOutOfBounds(t *testing.T) {
	g := &GrayImage{Width: 4, Height: 3, Pix: make([]byte, 12)}
	for _, r := range []Rect{
		{X0: -1, Y0: 0, X1: 2, Y1: 2},
		{X0: 0, Y0: 0, X1: 5, Y1: 2},
		{X0: 0, Y0: 0, X1: 2, Y1: 4},
		{X0: 2, Y0: 0, X1: 2, Y1: 2},
		{X0: 3, Y0: 1, X1: 2, Y1: 2},
	} {
		if _, err := Crop(g, r); err == nil {
			t.Fatalf("rect %+v: expected error", r)
		}
	}
}

func TestHistogramAndPGM(t *testing.T) {
	g := &GrayImage{Width: 2, Height: 2, Pix: []byte{0, 255, 0, 7}}
	h := Histogram(g)
	if h[0] != 2 || h[255] != 1 || h[7] != 1 {
		t.Fatalf("histogram wrong: %v %v %v", h[0], h[7], h[255])
	}
	pgm := PGM(g)
	want := append([]byte("P5\n2 2\n255\n"), 0, 255, 0, 7)
	if string(pgm) != string(want) {
		t.Fatalf("pgm = %q", pgm)
	}
}
