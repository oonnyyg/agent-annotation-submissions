package bmpintake

// Decode parses and decodes a BMP file into a top-down raster.
func Decode(data []byte) (*Image, error) {
	h, err := parseHeader(data)
	if err != nil {
		return nil, err
	}
	absHeight := h.height
	if absHeight < 0 {
		absHeight = -absHeight
	}
	img := &Image{Width: h.width, Height: absHeight, Palette: h.palette}

	switch {
	case h.compression == compressionRGB && h.bpp == 24:
		err = decodeRGB24(data, h, img)
	case h.compression == compressionRGB && h.bpp == 8:
		err = decodeIndexed8(data, h, img)
	default:
		err = decodeRLE8(data, h, img)
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

// rowFor maps a stored row index (0 = first row in file) to the
// top-down output row, honoring the height sign.
func rowFor(h *header, storedRow, absHeight int) int {
	if h.height < 0 {
		return storedRow
	}
	return absHeight - 1 - storedRow
}

func rowStride(width, bpp int) int {
	bits := width * bpp
	return (bits + 31) / 32 * 4
}

func decodeRGB24(data []byte, h *header, img *Image) error {
	stride := rowStride(h.width, 24)
	absHeight := img.Height
	need := h.pixelOffset + int64(stride)*int64(absHeight)
	if need > int64(len(data)) {
		return errAt(h.pixelOffset, "pixel data truncated: need %d bytes, file has %d", need, len(data))
	}
	img.RGB = make([]byte, h.width*absHeight*3)
	for row := 0; row < absHeight; row++ {
		src := h.pixelOffset + int64(row)*int64(stride)
		dst := rowFor(h, row, absHeight) * h.width * 3
		for x := 0; x < h.width; x++ {
			b := data[src+int64(x)*3]
			g := data[src+int64(x)*3+1]
			r := data[src+int64(x)*3+2]
			img.RGB[dst+x*3] = r
			img.RGB[dst+x*3+1] = g
			img.RGB[dst+x*3+2] = b
		}
	}
	return nil
}

func decodeIndexed8(data []byte, h *header, img *Image) error {
	stride := rowStride(h.width, 8)
	absHeight := img.Height
	need := h.pixelOffset + int64(stride)*int64(absHeight)
	if need > int64(len(data)) {
		return errAt(h.pixelOffset, "pixel data truncated: need %d bytes, file has %d", need, len(data))
	}
	img.Pixels = make([]byte, h.width*absHeight)
	for row := 0; row < absHeight; row++ {
		src := h.pixelOffset + int64(row)*int64(stride)
		dst := rowFor(h, row, absHeight) * h.width
		for x := 0; x < h.width; x++ {
			idx := data[src+int64(x)]
			if int(idx) >= len(h.palette) {
				return errAt(src+int64(x), "palette index %d out of range (%d entries)", idx, len(h.palette))
			}
			img.Pixels[dst+x] = idx
		}
	}
	return nil
}
