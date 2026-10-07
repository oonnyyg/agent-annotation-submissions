package hpack

import (
	"encoding/hex"
	"reflect"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// RFC 7541 Appendix C.3.1: first request, literal with incremental indexing.
func TestDecodeRFCExampleFirstRequest(t *testing.T) {
	d := NewDecoder(4096)
	fields, log, err := d.Decode(mustHex(t, "828684410f7777772e6578616d706c652e636f6d"))
	if err != nil {
		t.Fatal(err)
	}
	want := []HeaderField{
		{":method", "GET", false},
		{":scheme", "http", false},
		{":path", "/", false},
		{":authority", "www.example.com", false},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("got %+v want %+v", fields, want)
	}
	if len(log) != 1 || log[0].Kind != "insert" || log[0].Field.Name != ":authority" {
		t.Fatalf("unexpected table log %+v", log)
	}
}

// RFC 7541 Appendix C.3.2: second request reuses the dynamic table entry.
func TestDecodeRFCExampleSecondRequest(t *testing.T) {
	d := NewDecoder(4096)
	if _, _, err := d.Decode(mustHex(t, "828684410f7777772e6578616d706c652e636f6d")); err != nil {
		t.Fatal(err)
	}
	fields, _, err := d.Decode(mustHex(t, "828684be58086e6f2d6361636865"))
	if err != nil {
		t.Fatal(err)
	}
	want := []HeaderField{
		{":method", "GET", false},
		{":scheme", "http", false},
		{":path", "/", false},
		{":authority", "www.example.com", false},
		{"cache-control", "no-cache", false},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("got %+v want %+v", fields, want)
	}
}

// Cross-block reuse: encoding the same field twice must yield an
// indexed representation in the second block, and a decode round trip
// must preserve order.
func TestCrossBlockReuseAndOrder(t *testing.T) {
	enc := NewEncoder(4096)
	dec := NewDecoder(4096)
	in := []HeaderField{
		{":method", "GET", false},
		{"x-custom", "alpha", false},
		{"x-other", "beta", false},
	}
	if _, _, err := enc.Encode(in); err != nil {
		t.Fatal(err)
	}
	block2, _, err := enc.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	// :method GET is static index 2 (0x82); the two custom fields must
	// now be indexed (0x80|idx), so the whole block is 3 bytes.
	if len(block2) != 3 || block2[0] != 0x82 || block2[1]&0x80 == 0 || block2[2]&0x80 == 0 {
		t.Fatalf("expected fully indexed block, got %x", block2)
	}
	// Replay both blocks through the decoder.
	b1, _, _ := enc.Encode(in) // third encode; still indexed
	_ = b1
	enc2 := NewEncoder(4096)
	dec2 := NewDecoder(4096)
	for i := 0; i < 2; i++ {
		blk, _, err := enc2.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := dec2.Decode(blk)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("block %d: got %+v want %+v", i, got, in)
		}
	}
	_ = dec
}

// Sensitive fields must always use never-indexed, even when an exact
// match exists in the table.
func TestSensitiveNeverIndexed(t *testing.T) {
	enc := NewEncoder(4096)
	dec := NewDecoder(4096)
	plain := []HeaderField{{"x-secret", "hunter2", false}}
	b1, _, err := enc.Encode(plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := dec.Decode(b1); err != nil {
		t.Fatal(err)
	}
	block, log, err := enc.Encode([]HeaderField{{"x-secret", "hunter2", true}})
	if err != nil {
		t.Fatal(err)
	}
	if block[0]&0xf0 != 0x10 {
		t.Fatalf("expected never-indexed tag, got %x", block[0])
	}
	if len(log) != 0 {
		t.Fatalf("sensitive field must not touch the table: %+v", log)
	}
	fields, _, err := dec.Decode(block)
	if err != nil {
		t.Fatal(err)
	}
	if !fields[0].Sensitive || fields[0].Value != "hunter2" {
		t.Fatalf("got %+v", fields)
	}
}
