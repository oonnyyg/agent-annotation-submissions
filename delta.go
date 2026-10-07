package pack

import "fmt"

// ApplyDelta applies Git delta instructions in delta to base and returns the
// reconstructed object. It validates the base size, the declared result size,
// and every copy/insert instruction; any violation fails the whole operation
// without producing a partial object.
func ApplyDelta(base, delta []byte) ([]byte, error) {
	pos := 0
	readVarint := func() (int64, error) {
		var v int64
		var shift uint
		for {
			if pos >= len(delta) {
				return 0, fmt.Errorf("truncated delta header")
			}
			b := delta[pos]
			pos++
			v |= int64(b&0x7f) << shift
			shift += 7
			if shift > 63 {
				return 0, fmt.Errorf("delta size varint too long")
			}
			if b&0x80 == 0 {
				return v, nil
			}
		}
	}

	baseSize, err := readVarint()
	if err != nil {
		return nil, err
	}
	if baseSize != int64(len(base)) {
		return nil, fmt.Errorf("delta base size mismatch: header says %d, base is %d", baseSize, len(base))
	}
	resultSize, err := readVarint()
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, resultSize)
	for pos < len(delta) {
		op := delta[pos]
		pos++
		if op&0x80 != 0 {
			// Copy from base: offset/size bytes present per flag bits.
			var cpOff, cpSize int64
			for i := 0; i < 4; i++ {
				if op&(1<<uint(i)) != 0 {
					if pos >= len(delta) {
						return nil, fmt.Errorf("truncated copy offset")
					}
					cpOff |= int64(delta[pos]) << (8 * uint(i))
					pos++
				}
			}
			for i := 0; i < 3; i++ {
				if op&(0x10<<uint(i)) != 0 {
					if pos >= len(delta) {
						return nil, fmt.Errorf("truncated copy size")
					}
					cpSize |= int64(delta[pos]) << (8 * uint(i))
					pos++
				}
			}
			if cpSize == 0 {
				cpSize = 0x10000
			}
			if cpOff < 0 || cpSize < 0 || cpOff+cpSize > int64(len(base)) {
				return nil, fmt.Errorf("copy out of range: offset %d size %d, base %d", cpOff, cpSize, len(base))
			}
			out = append(out, base[cpOff:cpOff+cpSize]...)
		} else if op != 0 {
			// Insert literal bytes.
			if pos+int(op) > len(delta) {
				return nil, fmt.Errorf("truncated insert of %d bytes", op)
			}
			out = append(out, delta[pos:pos+int(op)]...)
			pos += int(op)
		} else {
			return nil, fmt.Errorf("illegal delta opcode 0x00")
		}
		if int64(len(out)) > resultSize {
			return nil, fmt.Errorf("delta produced more than declared result size %d", resultSize)
		}
	}
	if int64(len(out)) != resultSize {
		return nil, fmt.Errorf("delta result size %d does not match declared %d", len(out), resultSize)
	}
	return out, nil
}
