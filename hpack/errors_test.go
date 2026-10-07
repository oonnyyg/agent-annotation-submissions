package hpack

import (
	"strings"
	"testing"
)

// A truncated integer must fail with a byte offset and poison the
// decoder until Reset; the paired encoder keeps working meanwhile.
func TestTruncatedIntegerFailsAndIsolates(t *testing.T) {
	enc := NewEncoder(4096)
	dec := NewDecoder(4096)
	// Indexed field with 7-bit prefix maxed (0xff) but no continuation.
	_, _, err := dec.Decode([]byte{0xff})
	if err == nil || !strings.Contains(err.Error(), "byte 1") {
		t.Fatalf("expected byte-positioned error, got %v", err)
	}
	if !dec.Failed() {
		t.Fatal("decoder should be failed")
	}
	if _, _, err := dec.Decode([]byte{0x82}); err == nil {
		t.Fatal("failed decoder must reject further blocks")
	}
	// Encoder direction is unaffected.
	if _, _, err := enc.Encode([]HeaderField{{":method", "GET", false}}); err != nil {
		t.Fatalf("encoder should still work: %v", err)
	}
	dec.Reset()
	fields, _, err := dec.Decode([]byte{0x82})
	if err != nil || fields[0].Value != "GET" {
		t.Fatalf("after reset: %v %+v", err, fields)
	}
}

// Out-of-range index error carries the byte offset.
func TestIndexOutOfRange(t *testing.T) {
	dec := NewDecoder(4096)
	_, _, err := dec.Decode([]byte{0x82, 0xff, 0x00}) // index 2 ok, then 127+0=127? no: 0xff 0x00 -> 127+... = 127
	if err == nil || !strings.Contains(err.Error(), "byte 3") {
		t.Fatalf("got %v", err)
	}
}

// Huffman flag and invalid UTF-8 are rejected with positions.
func TestHuffmanAndUTF8Rejected(t *testing.T) {
	dec := NewDecoder(4096)
	// Literal inc-indexing, new name, H bit set on name string.
	_, _, err := dec.Decode([]byte{0x40, 0x81, 0x00})
	if err == nil || !strings.Contains(err.Error(), "Huffman") {
		t.Fatalf("got %v", err)
	}
	dec.Reset()
	// Name "a", value is one invalid UTF-8 byte 0xff.
	_, _, err = dec.Decode([]byte{0x40, 0x01, 'a', 0x01, 0xff})
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("got %v", err)
	}
}

// Encoder rejects invalid UTF-8 input and enters its own failed state.
func TestEncoderRejectsInvalidUTF8(t *testing.T) {
	enc := NewEncoder(4096)
	if _, _, err := enc.Encode([]HeaderField{{"x", "\xff", false}}); err == nil {
		t.Fatal("expected error")
	}
	if !enc.Failed() {
		t.Fatal("encoder should be failed")
	}
	if _, _, err := enc.Encode([]HeaderField{{":method", "GET", false}}); err == nil {
		t.Fatal("failed encoder must reject")
	}
	enc.Reset()
	if _, _, err := enc.Encode([]HeaderField{{":method", "GET", false}}); err != nil {
		t.Fatal(err)
	}
}

// Table size update after a field line, or beyond the negotiated max,
// is a decode error.
func TestTableSizeUpdateRules(t *testing.T) {
	dec := NewDecoder(100)
	// 0x82 = field, then 0x3f 0x40 = update to 95 -> after field.
	_, _, err := dec.Decode([]byte{0x82, 0x3f, 0x40})
	if err == nil || !strings.Contains(err.Error(), "after field") {
		t.Fatalf("got %v", err)
	}
	dec.Reset()
	// Update to 200 exceeds negotiated 100.
	_, _, err = dec.Decode([]byte{0x3f, 0xa9, 0x01})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("got %v", err)
	}
}

// Encoder emits the size update at the start of the next block; the
// decoder applies it and zero capacity still round-trips headers.
func TestResizeAndZeroCapacity(t *testing.T) {
	enc := NewEncoder(4096)
	dec := NewDecoder(4096)
	enc.SetMaxSize(0)
	dec.SetMaxSize(0)
	block, log, err := enc.Encode([]HeaderField{{"x-a", "1", false}})
	if err != nil {
		t.Fatal(err)
	}
	if block[0] != 0x20 { // update to 0
		t.Fatalf("expected leading size update, got %x", block)
	}
	if len(log) != 0 {
		t.Fatalf("zero capacity must not insert: %+v", log)
	}
	fields, _, err := dec.Decode(block)
	if err != nil || len(fields) != 1 || fields[0].Name != "x-a" {
		t.Fatalf("%v %+v", err, fields)
	}
	// Second block: no pending update anymore.
	block2, _, err := enc.Encode([]HeaderField{{"x-b", "2", false}})
	if err != nil {
		t.Fatal(err)
	}
	if block2[0]&0xe0 == 0x20 {
		t.Fatalf("unexpected repeated update: %x", block2)
	}
}

// Eviction: small table evicts oldest entries; oversized entries clear
// the table without being inserted.
func TestEvictionAndOversized(t *testing.T) {
	enc := NewEncoder(128)
	var allLog []EvictRecord
	mk := func(name string) HeaderField { return HeaderField{name, "v", false} } // 34 bytes each
	for _, n := range []string{"x-1", "x-2", "x-3", "x-4"} {
		_, log, err := enc.Encode([]HeaderField{mk(n)})
		if err != nil {
			t.Fatal(err)
		}
		allLog = append(allLog, log...)
	}
	// 4*34=136 > 128, so x-1 must have been evicted.
	var evicted []string
	for _, r := range allLog {
		if r.Kind == "evict" {
			evicted = append(evicted, r.Field.Name)
		}
	}
	if len(evicted) == 0 || evicted[0] != "x-1" {
		t.Fatalf("expected x-1 evicted first, got %v", evicted)
	}
	// Oversized entry clears the table and is not inserted.
	big := HeaderField{"x-big", strings.Repeat("y", 200), false}
	_, log, err := enc.Encode([]HeaderField{big})
	if err != nil {
		t.Fatal(err)
	}
	inserts := 0
	for _, r := range log {
		if r.Kind == "insert" {
			inserts++
		}
	}
	if inserts != 0 || enc.table.size != 0 {
		t.Fatalf("oversized entry must clear table without insert: %+v size=%d", log, enc.table.size)
	}
}
