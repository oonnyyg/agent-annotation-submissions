package bmpintake

// decodeRLE8 decodes BI_RLE8 pixel data. Unwritten pixels keep
// palette index zero. The stream must end with an EOB marker.
func decodeRLE8(data []byte, h *header, img *Image) error {
	w, absHeight := h.width, img.Height
	img.Pixels = make([]byte, w*absHeight) // zero-filled: palette index 0

	pos := h.pixelOffset
	x, y := 0, 0 // stored row 0 = bottom row for positive height

	put := func(idx byte, off int64) error {
		if int(idx) >= len(h.palette) {
			return errAt(off, "palette index %d out of range (%d entries)", idx, len(h.palette))
		}
		if y >= absHeight || x >= w {
			return errAt(off, "cursor out of bounds at (%d, %d) for %dx%d image", x, y, w, absHeight)
		}
		img.Pixels[rowFor(h, y, absHeight)*w+x] = idx
		x++
		return nil
	}

	for {
		if pos >= int64(len(data)) {
			return errAt(pos, "missing end-of-bitmap marker (stream exhausted)")
		}
		if pos+1 >= int64(len(data)) {
			return errAt(pos, "truncated RLE command: single byte remains")
		}
		count, val := data[pos], data[pos+1]
		cmdOff := pos
		pos += 2

		if count > 0 {
			// Encoded mode: repeat val count times; must not cross row width.
			if x+int(count) > w {
				return errAt(cmdOff, "run of %d pixels at x=%d crosses row width %d", count, x, w)
			}
			if int(val) >= len(h.palette) {
				return errAt(cmdOff+1, "palette index %d out of range (%d entries)", val, len(h.palette))
			}
			for i := 0; i < int(count); i++ {
				if err := put(val, cmdOff+1); err != nil {
					return err
				}
			}
			continue
		}

		switch val {
		case 0: // end of line
			x = 0
			y++
			if y > absHeight {
				return errAt(cmdOff, "end-of-line moved cursor past image bottom (row %d of %d)", y, absHeight)
			}
		case 1: // end of bitmap
			return nil
		case 2: // delta
			if pos+1 >= int64(len(data)) {
				return errAt(pos, "truncated delta command")
			}
			dx, dy := int(data[pos]), int(data[pos+1])
			pos += 2
			x += dx
			y += dy
			if x > w || y > absHeight {
				return errAt(cmdOff, "delta (%d, %d) moved cursor out of image to (%d, %d) for %dx%d", dx, dy, x, y, w, absHeight)
			}
		default: // absolute mode: val literal pixels, padded to even
			n := int(val)
			if x+n > w {
				return errAt(cmdOff, "absolute run of %d pixels at x=%d crosses row width %d", n, x, w)
			}
			if pos+int64(n) > int64(len(data)) {
				return errAt(pos, "truncated absolute run: need %d bytes, %d remain", n, int64(len(data))-pos)
			}
			for i := 0; i < n; i++ {
				if err := put(data[pos+int64(i)], pos+int64(i)); err != nil {
					return err
				}
			}
			pos += int64(n)
			if n%2 == 1 {
				if pos >= int64(len(data)) {
					return errAt(pos, "truncated absolute run: missing pad byte")
				}
				pos++
			}
		}
	}
}
