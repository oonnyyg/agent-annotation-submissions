// Package bmpintake decodes legacy BMP files (40-byte info header,
// 24-bit and 8-bit uncompressed, 8-bit RLE8) into a normalized
// top-down raster and reports corrupt files with byte offsets.
package bmpintake

import (
	"encoding/binary"
	"fmt"
)

const (
	fileHeaderSize = 14
	infoHeaderSize = 40

	compressionRGB  = 0
	compressionRLE8 = 1
)

// Image is a decoded BMP with rows ordered top-down.
type Image struct {
	Width   int
	Height  int
	Pixels  []byte    // palette indices (8-bit) or nil for 24-bit
	RGB     []byte    // 3 bytes per pixel when Palette is nil
	Palette []RGBQuad // non-nil for 8-bit images
}

// RGBQuad is one palette entry.
type RGBQuad struct {
	R, G, B uint8
}

// ParseError describes a corrupt file at a specific byte offset.
type ParseError struct {
	Offset int64
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("bmp: byte offset %d: %s", e.Offset, e.Msg)
}

func errAt(off int64, format string, args ...any) *ParseError {
	return &ParseError{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

type header struct {
	pixelOffset int64
	width       int
	height      int // signed as stored
	bpp         int
	compression uint32
	palette     []RGBQuad
}

func parseHeader(data []byte) (*header, error) {
	if len(data) < fileHeaderSize+infoHeaderSize {
		return nil, errAt(int64(len(data)), "file too short for BMP headers (%d bytes)", len(data))
	}
	if data[0] != 'B' || data[1] != 'M' {
		return nil, errAt(0, "missing BM signature")
	}
	pixelOffset := int64(binary.LittleEndian.Uint32(data[10:14]))

	const ih = fileHeaderSize
	headerSize := binary.LittleEndian.Uint32(data[ih : ih+4])
	if headerSize != infoHeaderSize {
		return nil, errAt(ih, "unsupported info header size %d (only 40-byte BITMAPINFOHEADER)", headerSize)
	}
	width := int(int32(binary.LittleEndian.Uint32(data[ih+4 : ih+8])))
	height := int(int32(binary.LittleEndian.Uint32(data[ih+8 : ih+12])))
	if width <= 0 {
		return nil, errAt(ih+4, "invalid width %d", width)
	}
	if height == 0 {
		return nil, errAt(ih+8, "invalid height 0")
	}
	planes := binary.LittleEndian.Uint16(data[ih+12 : ih+14])
	if planes != 1 {
		return nil, errAt(ih+12, "invalid plane count %d", planes)
	}
	bpp := int(binary.LittleEndian.Uint16(data[ih+14 : ih+16]))
	compression := binary.LittleEndian.Uint32(data[ih+16 : ih+20])
	colorsUsed := binary.LittleEndian.Uint32(data[ih+32 : ih+36])

	h := &header{
		pixelOffset: pixelOffset,
		width:       width,
		height:      height,
		bpp:         bpp,
		compression: compression,
	}

	switch compression {
	case compressionRGB:
		if bpp != 24 && bpp != 8 {
			return nil, errAt(ih+14, "unsupported bit depth %d for uncompressed image (need 24 or 8)", bpp)
		}
	case compressionRLE8:
		if bpp != 8 {
			return nil, errAt(ih+14, "RLE8 compression requires 8-bit depth, got %d", bpp)
		}
		if height < 0 {
			return nil, errAt(ih+8, "RLE8 images must use positive (bottom-up) height")
		}
	default:
		return nil, errAt(ih+16, "unsupported compression %d (only 0=BI_RGB and 1=BI_RLE8)", compression)
	}

	if bpp == 8 {
		entries := int(colorsUsed)
		if entries == 0 {
			entries = 256
		}
		if entries > 256 {
			return nil, errAt(ih+32, "palette entry count %d exceeds 256", entries)
		}
		palStart := int64(fileHeaderSize + infoHeaderSize)
		palEnd := palStart + int64(entries)*4
		if palEnd > int64(len(data)) {
			return nil, errAt(palStart, "palette of %d entries extends past end of file", entries)
		}
		h.palette = make([]RGBQuad, entries)
		for i := 0; i < entries; i++ {
			o := palStart + int64(i)*4
			h.palette[i] = RGBQuad{R: data[o+2], G: data[o+1], B: data[o]}
		}
		if pixelOffset < palEnd {
			return nil, errAt(10, "pixel data offset %d overlaps palette ending at %d", pixelOffset, palEnd)
		}
	} else {
		if pixelOffset < fileHeaderSize+infoHeaderSize {
			return nil, errAt(10, "pixel data offset %d overlaps headers", pixelOffset)
		}
	}
	if pixelOffset > int64(len(data)) {
		return nil, errAt(10, "pixel data offset %d past end of file (%d bytes)", pixelOffset, len(data))
	}
	return h, nil
}
