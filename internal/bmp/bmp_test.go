package bmp

import (
	"encoding/binary"
	"errors"
	"testing"
)

// buildBMP assembles a BMP file from headers, palette and pixel data.
func buildBMP(width, height int32, bpp uint16, compression uint32, pal [][3]byte, pixels []byte) []byte {
	palBytes := len(pal) * 4
	off := fileHeaderLen + infoHeaderLen + palBytes
	total := off + len(pixels)
	buf := make([]byte, total)
	buf[0], buf[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(buf[2:], uint32(total))
	binary.LittleEndian.PutUint32(buf[10:], uint32(off))
	binary.LittleEndian.PutUint32(buf[14:], infoHeaderLen)
	binary.LittleEndian.PutUint32(buf[18:], uint32(width))
	binary.LittleEndian.PutUint32(buf[22:], uint32(height))
	binary.LittleEndian.PutUint16(buf[26:], 1)
	binary.LittleEndian.PutUint16(buf[28:], bpp)
	binary.LittleEndian.PutUint32(buf[30:], compression)
	binary.LittleEndian.PutUint32(buf[34:], uint32(len(pixels)))
	binary.LittleEndian.PutUint32(buf[46:], uint32(len(pal)))
	for i, c := range pal {
		base := fileHeaderLen + infoHeaderLen + i*4
		buf[base], buf[base+1], buf[base+2] = c[2], c[1], c[0]
	}
	copy(buf[off:], pixels)
	return buf
}

func pixel(t *testing.T, img *Image, x, y int) (r, g, b byte) {
	t.Helper()
	i := (y*img.Width + x) * 3
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2]
}

func TestDecode24BottomUpRowPadding(t *testing.T) {
	// 3x2, 24-bit: stride = 12 (9 bytes + 3 padding).
	// Bottom row first: red green blue; top row: white black gray.
	pixels := []byte{
		0, 0, 255, 0, 255, 0, 255, 0, 0, 0, 0, 0, // bottom: R G B
		255, 255, 255, 0, 0, 0, 128, 128, 128, 0, 0, 0, // top: white black gray
	}
	img, err := Decode(buildBMP(3, 2, 24, biRGB, nil, pixels))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b := pixel(t, img, 0, 0); r != 255 || g != 255 || b != 255 {
		t.Fatalf("top-left = %d,%d,%d", r, g, b)
	}
	if r, g, b := pixel(t, img, 2, 0); r != 128 || g != 128 || b != 128 {
		t.Fatalf("top-right = %d,%d,%d", r, g, b)
	}
	if r, g, b := pixel(t, img, 0, 1); r != 255 || g != 0 || b != 0 {
		t.Fatalf("bottom-left = %d,%d,%d", r, g, b)
	}
	if r, g, b := pixel(t, img, 2, 1); r != 0 || g != 0 || b != 255 {
		t.Fatalf("bottom-right = %d,%d,%d", r, g, b)
	}
}

func TestDecode24TopDown(t *testing.T) {
	pixels := []byte{
		0, 0, 255, 0, 255, 0, 255, 0, 0, 0, 0, 0, // top row first
		255, 255, 255, 0, 0, 0, 128, 128, 128, 0, 0, 0,
	}
	img, err := Decode(buildBMP(3, -2, 24, biRGB, nil, pixels))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, b := pixel(t, img, 0, 0); r != 255 || b != 0 {
		t.Fatalf("top-left = %d,_,%d", r, b)
	}
	if r, g, b := pixel(t, img, 0, 1); r != 255 || g != 255 || b != 255 {
		t.Fatalf("bottom-left = %d,%d,%d", r, g, b)
	}
}

func TestDecode8UncompressedPalette(t *testing.T) {
	pal := [][3]byte{{10, 20, 30}, {200, 100, 50}}
	// 3x1 bottom-up, stride 4: indices 1,0,1 + pad.
	pixels := []byte{1, 0, 1, 0}
	img, err := Decode(buildBMP(3, 1, 8, biRGB, pal, pixels))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b := pixel(t, img, 0, 0); r != 200 || g != 100 || b != 50 {
		t.Fatalf("pixel0 = %d,%d,%d", r, g, b)
	}
	if r, g, b := pixel(t, img, 1, 0); r != 10 || g != 20 || b != 30 {
		t.Fatalf("pixel1 = %d,%d,%d", r, g, b)
	}
}

func TestDecode8PaletteIndexOutOfRange(t *testing.T) {
	pal := [][3]byte{{0, 0, 0}, {1, 1, 1}}
	pixels := []byte{5, 0, 0, 0}
	_, err := Decode(buildBMP(3, 1, 8, biRGB, pal, pixels))
	var oe *OffsetError
	if !errors.As(err, &oe) {
		t.Fatalf("want OffsetError, got %v", err)
	}
	wantOff := int64(fileHeaderLen + infoHeaderLen + 8)
	if oe.Offset != wantOff {
		t.Fatalf("offset = %d, want %d", oe.Offset, wantOff)
	}
}

func TestDecodeTruncatedPixelData(t *testing.T) {
	data := buildBMP(3, 2, 24, biRGB, nil, make([]byte, 24))
	data = data[:len(data)-5]
	binary.LittleEndian.PutUint32(data[2:], uint32(len(data)))
	_, err := Decode(data)
	var oe *OffsetError
	if !errors.As(err, &oe) {
		t.Fatalf("want OffsetError, got %v", err)
	}
}

func TestRejectUnsupportedFormats(t *testing.T) {
	cases := []struct {
		name string
		bpp  uint16
		comp uint32
	}{
		{"1-bit", 1, biRGB},
		{"4-bit", 4, biRGB},
		{"16-bit", 16, biRGB},
		{"32-bit", 32, biRGB},
		{"RLE4", 4, 2},
		{"bitfields", 24, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(buildBMP(1, 1, tc.bpp, tc.comp, nil, make([]byte, 8)))
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestRejectBadHeaderSize(t *testing.T) {
	data := buildBMP(1, 1, 24, biRGB, nil, make([]byte, 4))
	binary.LittleEndian.PutUint32(data[14:], 12) // BITMAPCOREHEADER
	if _, err := Decode(data); err == nil {
		t.Fatal("expected rejection of 12-byte header")
	}
}
