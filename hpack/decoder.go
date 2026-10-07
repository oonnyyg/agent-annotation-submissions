package hpack

import (
	"fmt"
	"unicode/utf8"
)

// Decoder parses HPACK header blocks. It owns one dynamic table,
// independent of any Encoder.
type Decoder struct {
	table   *dynamicTable
	maxSize int  // negotiated maximum; updates beyond this are rejected
	failed  bool // set after a decode error; Reset clears it
}

func NewDecoder(maxSize int) *Decoder {
	return &Decoder{table: newDynamicTable(maxSize), maxSize: maxSize}
}

// Failed reports whether the decoder is in the failed state.
func (d *Decoder) Failed() bool { return d.failed }

// SetMaxSize updates the negotiated maximum table size. Updates from
// the peer beyond this limit are rejected.
func (d *Decoder) SetMaxSize(n int) { d.maxSize = n }

// Reset clears the failed state so decoding can resume.
func (d *Decoder) Reset() { d.failed = false }

// Decode parses one header block. On any error no partial header list
// is returned and the decoder enters the failed state.
func (d *Decoder) Decode(block []byte) ([]HeaderField, []EvictRecord, error) {
	if d.failed {
		return nil, nil, fmt.Errorf("decoder is in failed state; call Reset")
	}
	fields, log, err := d.decode(block)
	if err != nil {
		d.failed = true
		return nil, nil, err
	}
	return fields, log, nil
}

func (d *Decoder) decode(block []byte) ([]HeaderField, []EvictRecord, error) {
	var fields []HeaderField
	var log []EvictRecord
	seenField := false
	off := 0
	for off < len(block) {
		b := block[off]
		switch {
		case b&0x80 != 0: // indexed field
			idx, noff, err := readInt(block, off, 7)
			if err != nil {
				return nil, nil, err
			}
			off = noff
			f, err := d.table.at(int(idx))
			if err != nil {
				return nil, nil, fmt.Errorf("byte %d: %w", off, err)
			}
			fields = append(fields, f)
			seenField = true
		case b&0xe0 == 0x20: // dynamic table size update
			size, noff, err := readInt(block, off, 5)
			if err != nil {
				return nil, nil, err
			}
			if seenField {
				return nil, nil, fmt.Errorf("byte %d: table size update after field line", off)
			}
			if int(size) > d.maxSize {
				return nil, nil, fmt.Errorf("byte %d: table size update %d exceeds negotiated maximum %d", off, size, d.maxSize)
			}
			off = noff
			d.table.setMaxSize(int(size), &log)
		case b&0xc0 == 0x40: // literal with incremental indexing
			f, noff, err := d.readLiteral(block, off, 6)
			if err != nil {
				return nil, nil, err
			}
			off = noff
			d.table.insert(f, &log)
			fields = append(fields, f)
			seenField = true
		case b&0xf0 == 0x00 || b&0xf0 == 0x10: // literal without / never indexed
			f, noff, err := d.readLiteral(block, off, 4)
			if err != nil {
				return nil, nil, err
			}
			off = noff
			if b&0xf0 == 0x10 {
				f.Sensitive = true
			}
			fields = append(fields, f)
			seenField = true
		default:
			return nil, nil, fmt.Errorf("byte %d: unknown representation tag 0x%02x", off, b)
		}
	}
	return fields, log, nil
}

// readLiteral parses a literal field line starting at off.
func (d *Decoder) readLiteral(block []byte, off int, prefix uint) (HeaderField, int, error) {
	nameIdx, off, err := readInt(block, off, prefix)
	if err != nil {
		return HeaderField{}, off, err
	}
	var f HeaderField
	if nameIdx == 0 {
		name, noff, err := readString(block, off)
		if err != nil {
			return HeaderField{}, noff, err
		}
		off = noff
		f.Name = name
	} else {
		ref, err := d.table.at(int(nameIdx))
		if err != nil {
			return HeaderField{}, off, fmt.Errorf("byte %d: %w", off, err)
		}
		f.Name = ref.Name
	}
	value, noff, err := readString(block, off)
	if err != nil {
		return HeaderField{}, noff, err
	}
	f.Value = value
	return f, noff, nil
}

// readString parses a string literal. Huffman-coded strings (H bit set)
// are rejected; this tool only supports raw UTF-8 text.
func readString(block []byte, off int) (string, int, error) {
	if off >= len(block) {
		return "", off, fmt.Errorf("byte %d: missing string length", off)
	}
	if block[off]&0x80 != 0 {
		return "", off, fmt.Errorf("byte %d: Huffman-coded strings are not supported", off)
	}
	length, noff, err := readInt(block, off, 7)
	if err != nil {
		return "", noff, err
	}
	off = noff
	if uint64(len(block)-off) < length {
		return "", off, fmt.Errorf("byte %d: string of %d bytes truncated at end of block", off, length)
	}
	s := string(block[off : off+int(length)])
	if !utf8.ValidString(s) {
		return "", off, fmt.Errorf("byte %d: invalid UTF-8 text", off)
	}
	return s, off + int(length), nil
}
