package bmp

// decodeRLE8 decodes a BI_RLE8 stream. The raster is bottom-up. Pixels
// never written by the stream keep palette index 0. The stream must end
// with an end-of-bitmap marker; running out of bytes mid-command is
// reported as truncation, while a clean command boundary without the
// marker is reported as a missing end marker.
func decodeRLE8(data []byte, h *header, pal [][3]byte, img *Image) error {
	w, ht := img.Width, img.Height
	idx := make([]int, w*ht) // palette indices, zero-initialized
	pos := h.pixelOffset
	end := int64(len(data))
	x, y := 0, 0 // y counts from the bottom row

	put := func(v byte, at int64) error {
		if x >= w || y >= ht {
			return errAt(at, "run crosses image bounds at column %d, row %d (image %dx%d)", x, y, w, ht)
		}
		if int(v) >= len(pal) {
			return errAt(at, "palette index %d out of range (palette has %d entries)", v, len(pal))
		}
		idx[(ht-1-y)*w+x] = int(v)
		x++
		return nil
	}

	for {
		if pos >= end {
			return errAt(pos, "stream ended without end-of-bitmap marker")
		}
		if pos+2 > end {
			return errAt(pos, "truncated command: need 2 bytes, have %d", end-pos)
		}
		count, val := data[pos], data[pos+1]
		cmdAt := pos
		pos += 2
		if count > 0 {
			// Encoded run: repeat val count times; must not cross row width.
			if x+int(count) > w {
				return errAt(cmdAt, "run of %d pixels at column %d crosses row width %d", count, x, w)
			}
			for i := 0; i < int(count); i++ {
				if err := put(val, cmdAt); err != nil {
					return err
				}
			}
			continue
		}
		switch val {
		case 0: // end of line
			x = 0
			y++
			if y > ht {
				return errAt(cmdAt, "end-of-line advanced past image height %d", ht)
			}
		case 1: // end of bitmap
			goto done
		case 2: // delta
			if pos+2 > end {
				return errAt(pos, "truncated delta: need 2 bytes, have %d", end-pos)
			}
			dx, dy := int(data[pos]), int(data[pos+1])
			pos += 2
			if x+dx > w || y+dy > ht {
				return errAt(cmdAt, "delta (%d,%d) from (%d,%d) leaves image %dx%d", dx, dy, x, y, w, ht)
			}
			x += dx
			y += dy
		default: // absolute run of val literal bytes, padded to even
			n := int(val)
			if x+n > w {
				return errAt(cmdAt, "absolute run of %d pixels at column %d crosses row width %d", n, x, w)
			}
			if pos+int64(n) > end {
				return errAt(pos, "truncated absolute run: need %d bytes, have %d", n, end-pos)
			}
			for i := 0; i < n; i++ {
				if err := put(data[pos+int64(i)], pos+int64(i)); err != nil {
					return err
				}
			}
			pos += int64(n)
			if n%2 == 1 {
				if pos >= end {
					return errAt(pos, "truncated absolute run padding byte")
				}
				pos++
			}
		}
	}
done:
	for i, v := range idx {
		c := pal[v]
		img.Pix[i*3], img.Pix[i*3+1], img.Pix[i*3+2] = c[0], c[1], c[2]
	}
	return nil
}
