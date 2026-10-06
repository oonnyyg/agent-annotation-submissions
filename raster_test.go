package bmpintake

import (
	"bytes"
	"strings"
	"testing"
)

func TestCropOutOfBounds(t *testing.T) {
	img := &Image{Width: 4, Height: 4, RGB: make([]byte, 4*4*3)}
	if _, err := img.CropGray(Rect{3, 0, 2, 2}); err == nil {
		t.Fatal("accepted crop past right edge")
	}
	if _, err := img.CropGray(Rect{0, 3, 2, 2}); err == nil {
		t.Fatal("accepted crop past bottom edge")
	}
	if _, err := img.CropGray(Rect{-1, 0, 2, 2}); err == nil {
		t.Fatal("accepted negative origin")
	}
	if _, err := img.CropGray(Rect{0, 0, 0, 2}); err == nil {
		t.Fatal("accepted zero width")
	}
}

func TestGrayRounding(t *testing.T) {
	img := &Image{Width: 1, Height: 1, RGB: []byte{0, 0, 255}}
	g, err := img.CropGray(Rect{0, 0, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	want := byte((114*255 + 500) / 1000)
	if g.Pix[0] != want {
		t.Fatalf("got %d want %d", g.Pix[0], want)
	}
	img.RGB = []byte{255, 255, 255}
	g, _ = img.CropGray(Rect{0, 0, 1, 1})
	if g.Pix[0] != 255 {
		t.Fatalf("white got %d", g.Pix[0])
	}
}

func TestHistogramAndPGM(t *testing.T) {
	g := &Gray{Width: 2, Height: 2, Pix: []byte{0, 255, 255, 7}}
	h := g.Histogram()
	if h[0] != 1 || h[255] != 2 || h[7] != 1 {
		t.Fatalf("histogram wrong")
	}
	var sum uint64
	for _, c := range h {
		sum += c
	}
	if sum != 4 {
		t.Fatalf("histogram total %d", sum)
	}
	var buf bytes.Buffer
	if err := g.WritePGM(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "P5\n2 2\n255\n") {
		t.Fatalf("header %q", buf.Bytes()[:11])
	}
	if !bytes.Equal(buf.Bytes()[11:], g.Pix) {
		t.Fatalf("payload %v", buf.Bytes()[11:])
	}
}
