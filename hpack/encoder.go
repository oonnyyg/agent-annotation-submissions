package hpack

import (
	"fmt"
	"unicode/utf8"
)

// Encoder emits HPACK header blocks. It owns one dynamic table,
// independent of any Decoder.
type Encoder struct {
	table         *dynamicTable
	maxSize       int  // negotiated maximum table size
	pendingUpdate int  // table size update to emit at next block start, -1 = none
	failed        bool // set after an encode error; Reset clears it
}

func NewEncoder(maxSize int) *Encoder {
	return &Encoder{table: newDynamicTable(maxSize), maxSize: maxSize, pendingUpdate: -1}
}

// SetMaxSize changes the encoder table capacity and schedules a dynamic
// table size update at the start of the next emitted block.
func (e *Encoder) SetMaxSize(n int) {
	e.maxSize = n
	e.table.setMaxSize(n, nil)
	e.pendingUpdate = n
}

// Failed reports whether the encoder is in the failed state.
func (e *Encoder) Failed() bool { return e.failed }

// Reset clears the failed state so encoding can resume.
func (e *Encoder) Reset() { e.failed = false }

// Encode serializes fields into one header block, returning the block
// bytes and the dynamic table insert/evict log.
func (e *Encoder) Encode(fields []HeaderField) ([]byte, []EvictRecord, error) {
	if e.failed {
		return nil, nil, fmt.Errorf("encoder is in failed state; call Reset")
	}
	for i, f := range fields {
		if !utf8.ValidString(f.Name) || !utf8.ValidString(f.Value) {
			e.failed = true
			return nil, nil, fmt.Errorf("field %d (%q): invalid UTF-8 text", i, f.Name)
		}
	}
	var block []byte
	var log []EvictRecord
	if e.pendingUpdate >= 0 {
		block = appendInt(block, uint64(e.pendingUpdate), 5, 0x20)
		e.pendingUpdate = -1
	}
	for _, f := range fields {
		if f.Sensitive {
			block = e.encodeLiteral(block, f, 0x10, 4) // never indexed
			continue
		}
		if idx := e.table.lookup(f.Name, f.Value); idx != 0 {
			block = appendInt(block, uint64(idx), 7, 0x80) // indexed
			continue
		}
		block = e.encodeLiteral(block, f, 0x40, 6) // incremental indexing
		e.table.insert(f, &log)
	}
	return block, log, nil
}

// encodeLiteral writes a literal field line with the given tag and
// prefix width, reusing a name index when one exists.
func (e *Encoder) encodeLiteral(dst []byte, f HeaderField, tag byte, prefix uint) []byte {
	if idx := e.table.lookupName(f.Name); idx != 0 {
		dst = appendInt(dst, uint64(idx), prefix, tag)
	} else {
		dst = appendInt(dst, 0, prefix, tag)
		dst = appendString(dst, f.Name)
	}
	return appendString(dst, f.Value)
}

// appendString writes a raw (non-Huffman) string literal.
func appendString(dst []byte, s string) []byte {
	dst = appendInt(dst, uint64(len(s)), 7, 0x00)
	return append(dst, s...)
}
