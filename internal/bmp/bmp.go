// Package bmp decodes a restricted subset of legacy BMP files into an
// in-memory RGBA-free raster. Supported inputs:
//   - 40-byte BITMAPINFOHEADER only
//   - 24-bit uncompressed (BI_RGB)
//   - 8-bit uncompressed (BI_RGB) with palette
//   - 8-bit RLE8 (BI_RLE8) with palette
//
// All structural errors are reported with the byte offset of the offending
// data so callers can pinpoint corrupt files.
package bmp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Image is a decoded raster. Pixels are stored top-down, row-major,
// three bytes per pixel in R, G, B order.
type Image struct {
	Width  int
	Height int
	Pix    []byte // len == Width*Height*3
}

// OffsetError describes a decode failure tied to a byte position in the
// source file.
type OffsetError struct {
	Offset int64
	Msg    string
}

func (e *OffsetError) Error() string {
	return fmt.Sprintf("bmp: byte %d: %s", e.Offset, e.Msg)
}

func errAt(off int64, format string, args ...any) error {
	return &OffsetError{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

const (
	fileHeaderLen = 14
	infoHeaderLen = 40

	biRGB  = 0
	biRLE8 = 1
)

type header struct {
	pixelOffset int64
	width       int32
	height      int32 // signed; negative means top-down
	bpp         uint16
	compression uint32
	imageSize   uint32
	colorsUsed  uint32
	paletteOff  int64
}

// parseHeader validates the file and info headers.
func parseHeader(data []byte) (*header, error) {
	if len(data) < fileHeaderLen+infoHeaderLen {
		return nil, errAt(int64(len(data)), "file too short for BMP headers (%d bytes)", len(data))
	}
	if data[0] != 'B' || data[1] != 'M' {
		return nil, errAt(0, "missing BM signature")
	}
	fileSize := binary.LittleEndian.Uint32(data[2:6])
	if int64(fileSize) != int64(len(data)) {
		return nil, errAt(2, "header file size %d does not match actual size %d", fileSize, len(data))
	}
	pixelOffset := int64(binary.LittleEndian.Uint32(data[10:14]))

	headerSize := binary.LittleEndian.Uint32(data[14:18])
	if headerSize != infoHeaderLen {
		return nil, errAt(14, "unsupported info header size %d (need 40)", headerSize)
	}
	h := &header{pixelOffset: pixelOffset}
	h.width = int32(binary.LittleEndian.Uint32(data[18:22]))
	h.height = int32(binary.LittleEndian.Uint32(data[22:26]))
	planes := binary.LittleEndian.Uint16(data[26:28])
	if planes != 1 {
		return nil, errAt(26, "unsupported plane count %d", planes)
	}
	h.bpp = binary.LittleEndian.Uint16(data[28:30])
	h.compression = binary.LittleEndian.Uint32(data[30:34])
	h.imageSize = binary.LittleEndian.Uint32(data[34:38])
	h.colorsUsed = binary.LittleEndian.Uint32(data[46:50])
	h.paletteOff = fileHeaderLen + infoHeaderLen

	if h.width <= 0 {
		return nil, errAt(18, "invalid width %d", h.width)
	}
	if h.height == 0 {
		return nil, errAt(22, "invalid height 0")
	}
	switch {
	case h.compression == biRGB && (h.bpp == 24 || h.bpp == 8):
	case h.compression == biRLE8 && h.bpp == 8:
	default:
		return nil, errAt(28, "unsupported bpp/compression combination: %d bpp, compression %d", h.bpp, h.compression)
	}
	if h.compression == biRLE8 && h.height < 0 {
		return nil, errAt(22, "RLE8 requires positive (bottom-up) height, got %d", h.height)
	}
	if h.pixelOffset < h.paletteOff {
		return nil, errAt(10, "pixel data offset %d before end of headers", h.pixelOffset)
	}
	if h.pixelOffset > int64(len(data)) {
		return nil, errAt(10, "pixel data offset %d beyond end of file (%d bytes)", h.pixelOffset, len(data))
	}
	return h, nil
}

// palette reads the color table for 8-bit images. Returns entries as
// [n][3]byte in R, G, B order.
func palette(data []byte, h *header) ([][3]byte, error) {
	if h.bpp != 8 {
		return nil, nil
	}
	n := int64(h.colorsUsed)
	if n == 0 {
		n = 256
	}
	if n > 256 {
		return nil, errAt(46, "palette size %d exceeds 256", n)
	}
	need := h.paletteOff + n*4
	if need > h.pixelOffset {
		return nil, errAt(46, "palette of %d entries overruns pixel data offset %d", n, h.pixelOffset)
	}
	if need > int64(len(data)) {
		return nil, errAt(h.paletteOff, "palette truncated: need %d bytes, file has %d", n*4, int64(len(data))-h.paletteOff)
	}
	pal := make([][3]byte, n)
	for i := int64(0); i < n; i++ {
		base := h.paletteOff + i*4
		b, g, r := data[base], data[base+1], data[base+2]
		pal[i] = [3]byte{r, g, b}
	}
	return pal, nil
}

// Decode parses and decodes a BMP file into a top-down RGB raster.
func Decode(data []byte) (*Image, error) {
	h, err := parseHeader(data)
	if err != nil {
		return nil, err
	}
	pal, err := palette(data, h)
	if err != nil {
		return nil, err
	}
	w := int(h.width)
	absH := h.height
	if absH < 0 {
		absH = -absH
	}
	img := &Image{Width: w, Height: int(absH), Pix: make([]byte, w*int(absH)*3)}
	switch h.compression {
	case biRGB:
		err = decodeUncompressed(data, h, pal, img)
	case biRLE8:
		err = decodeRLE8(data, h, pal, img)
	default:
		err = errors.New("bmp: unhandled compression")
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}
