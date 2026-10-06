package bmpintake

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildBMP assembles a BMP file in memory.
func buildBMP(width, height int32, bpp uint16, compression uint32, palette []RGBQuad, pixels []byte) []byte {
	palBytes := len(palette) * 4
	off := 14 + 40 + palBytes
	var buf bytes.Buffer
	buf.Write([]byte("BM"))
	binary.Write(&buf, binary.LittleEndian, uint32(off+len(pixels)))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(off))
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, width)
	binary.Write(&buf, binary.LittleEndian, height)
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, bpp)
	binary.Write(&buf, binary.LittleEndian, compression)
	binary.Write(&buf, binary.LittleEndian, uint32(len(pixels)))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	if palette != nil {
		binary.Write(&buf, binary.LittleEndian, uint32(len(palette)))
	} else {
		binary.Write(&buf, binary.LittleEndian, uint32(0))
	}
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	for _, q := range palette {
		buf.Write([]byte{q.B, q.G, q.R, 0})
	}
	buf.Write(pixels)
	return buf.Bytes()
}

func TestRGB24BottomUpPadding(t *testing.T) {
	// 3x2, stride = 12 (9 bytes data + 3 pad). Bottom row stored first.
	pix := []byte{
		0, 0, 255, 0, 255, 0, 255, 0, 0, 9, 9, 9, // bottom: R G B
		255, 255, 255, 0, 0, 0, 10, 20, 30, 8, 8, 8, // top: white, black, (30,20,10)
	}
	img, err := Decode(buildBMP(3, 2, 24, 0, nil, pix))
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 3 || img.Height != 2 {
		t.Fatalf("size %dx%d", img.Width, img.Height)
	}
	// Top-left must be white.
	if got := img.RGB[0:3]; !bytes.Equal(got, []byte{255, 255, 255}) {
		t.Fatalf("top-left %v", got)
	}
	// Top-right pixel stored as B=10,G=20,R=30.
	if got := img.RGB[6:9]; !bytes.Equal(got, []byte{30, 20, 10}) {
		t.Fatalf("top-right %v", got)
	}
	// Bottom-left must be red.
	o := 3 * 3
	if got := img.RGB[o : o+3]; !bytes.Equal(got, []byte{255, 0, 0}) {
		t.Fatalf("bottom-left %v", got)
	}
}

func TestRGB24TopDown(t *testing.T) {
	pix := []byte{
		255, 0, 0, 0, 0, 0, 0, 0, // top row: blue pixel + pad
		0, 0, 0, 0, 255, 0, 0, 0, // bottom row: green at x=1
	}
	img, err := Decode(buildBMP(2, -2, 24, 0, nil, pix))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGB[0:3]; !bytes.Equal(got, []byte{0, 0, 255}) {
		t.Fatalf("top-left %v", got)
	}
	o := (1*2 + 1) * 3
	if got := img.RGB[o : o+3]; !bytes.Equal(got, []byte{0, 255, 0}) {
		t.Fatalf("bottom-right %v", got)
	}
}

func TestIndexed8Palette(t *testing.T) {
	pal := []RGBQuad{{R: 10, G: 20, B: 30}, {R: 200, G: 100, B: 50}}
	pix := []byte{
		1, 0, 0, 0, // bottom row (width 3, stride 4)
		0, 1, 1, 0, // top row
	}
	img, err := Decode(buildBMP(3, 2, 8, 0, pal, pix))
	if err != nil {
		t.Fatal(err)
	}
	if img.Pixels[0] != 0 || img.Pixels[1] != 1 {
		t.Fatalf("top row %v", img.Pixels[:3])
	}
	g, err := img.CropGray(Rect{0, 0, 3, 2})
	if err != nil {
		t.Fatal(err)
	}
	want := byte((299*200 + 587*100 + 114*50 + 500) / 1000)
	if g.Pix[1] != want {
		t.Fatalf("gray %d want %d", g.Pix[1], want)
	}
}

func TestIndexed8BadIndex(t *testing.T) {
	pal := []RGBQuad{{R: 0, G: 0, B: 0}}
	pix := []byte{5, 0, 0, 0}
	_, err := Decode(buildBMP(1, 1, 8, 0, pal, pix))
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want ParseError, got %v", err)
	}
	wantOff := int64(14 + 40 + 4)
	if pe.Offset != wantOff {
		t.Fatalf("offset %d want %d", pe.Offset, wantOff)
	}
}
