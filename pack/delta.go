package pack

import "fmt"

// ApplyDelta applies Git delta instructions to base and returns the result.
// It validates the base size, the declared result size, and every copy/insert
// instruction; any violation returns an error and no partial object.
func ApplyDelta(base, delta []byte) ([]byte, error) {
	pos := 0
	baseSize, n := readVarint(delta)
	if n == 0 {
		return nil, fmt.Errorf("delta: missing base size")
	}
	pos += n
	if baseSize != int64(len(base)) {
		return nil, fmt.Errorf("delta: base size mismatch: header says %d, base is %d", baseSize, len(base))
	}
	resultSize, n := readVarint(delta[pos:])
	if n == 0 {
		return nil, fmt.Errorf("delta: missing result size")
	}
	pos += n
	if resultSize < 0 {
		return nil, fmt.Errorf("delta: negative result size")
	}

	out := make([]byte, 0, resultSize)
	for pos < len(delta) {
		cmd := delta[pos]
		pos++
		if cmd&0x80 != 0 {
			// Copy from base.
			var off, size int64
			for i := 0; i < 4; i++ {
				if cmd&(1<<uint(i)) != 0 {
					if pos >= len(delta) {
						return nil, fmt.Errorf("delta: truncated copy offset")
					}
					off |= int64(delta[pos]) << uint(8*i)
					pos++
				}
			}
			for i := 0; i < 3; i++ {
				if cmd&(0x10<<uint(i)) != 0 {
					if pos >= len(delta) {
						return nil, fmt.Errorf("delta: truncated copy size")
					}
					size |= int64(delta[pos]) << uint(8*i)
					pos++
				}
			}
			if size == 0 {
				size = 0x10000
			}
			if off < 0 || size < 0 || off+size > int64(len(base)) {
				return nil, fmt.Errorf("delta: copy out of range: offset %d size %d, base %d bytes", off, size, len(base))
			}
			out = append(out, base[off:off+size]...)
		} else if cmd != 0 {
			// Insert literal bytes.
			if pos+int(cmd) > len(delta) {
				return nil, fmt.Errorf("delta: truncated insert of %d bytes", cmd)
			}
			out = append(out, delta[pos:pos+int(cmd)]...)
			pos += int(cmd)
		} else {
			return nil, fmt.Errorf("delta: reserved opcode 0")
		}
		if int64(len(out)) > resultSize {
			return nil, fmt.Errorf("delta: result exceeds declared size %d", resultSize)
		}
	}
	if int64(len(out)) != resultSize {
		return nil, fmt.Errorf("delta: result size mismatch: header says %d, produced %d", resultSize, len(out))
	}
	return out, nil
}

// readVarint decodes a little-endian 7-bit varint as used in delta headers.
// Returns the value and bytes consumed; 0 consumed means truncated.
func readVarint(data []byte) (int64, int) {
	var v int64
	var shift uint
	for i := 0; i < len(data); i++ {
		c := data[i]
		v |= int64(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			return v, i + 1
		}
		if shift > 63 {
			return 0, 0
		}
	}
	return 0, 0
}
