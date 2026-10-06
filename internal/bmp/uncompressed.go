package bmp

// decodeUncompressed handles BI_RGB 24-bit and 8-bit rasters. Rows are
// padded to a 4-byte boundary. Positive height means rows are stored
// bottom-up; negative height means top-down.
func decodeUncompressed(data []byte, h *header, pal [][3]byte, img *Image) error {
	w := img.Width
	var rowBytes int64
	if h.bpp == 24 {
		rowBytes = int64(w) * 3
	} else {
		rowBytes = int64(w)
	}
	stride := (rowBytes + 3) &^ 3
	need := h.pixelOffset + stride*int64(img.Height)
	if need > int64(len(data)) {
		have := int64(len(data)) - h.pixelOffset
		rows := have / stride
		return errAt(h.pixelOffset+rows*stride+have%stride,
			"pixel data truncated: need %d rows of %d bytes, have %d bytes",
			img.Height, stride, have)
	}
	topDown := h.height < 0
	for y := 0; y < img.Height; y++ {
		srcY := y
		if !topDown {
			srcY = img.Height - 1 - y
		}
		row := data[h.pixelOffset+int64(srcY)*stride:]
		dst := img.Pix[y*w*3:]
		if h.bpp == 24 {
			for x := 0; x < w; x++ {
				b, g, r := row[x*3], row[x*3+1], row[x*3+2]
				dst[x*3], dst[x*3+1], dst[x*3+2] = r, g, b
			}
		} else {
			for x := 0; x < w; x++ {
				idx := row[x]
				if int(idx) >= len(pal) {
					return errAt(h.pixelOffset+int64(srcY)*stride+int64(x),
						"palette index %d out of range (palette has %d entries)", idx, len(pal))
				}
				c := pal[idx]
				dst[x*3], dst[x*3+1], dst[x*3+2] = c[0], c[1], c[2]
			}
		}
	}
	return nil
}
