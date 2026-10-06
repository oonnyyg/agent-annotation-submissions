package main

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func gzipMember(t *testing.T, data string, mutate func(*gzip.Header)) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if mutate != nil {
		mutate(&w.Header)
	}
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestConcatenatedMembers(t *testing.T) {
	m1 := gzipMember(t, "hello ", func(h *gzip.Header) { h.Name = "first.txt" })
	m2 := gzipMember(t, "", nil)
	m3 := gzipMember(t, "world", func(h *gzip.Header) {
		h.Comment = "last"
		h.Extra = []byte("AB\x04\x00data")
	})
	input := append(append(m1, m2...), m3...)
	var out bytes.Buffer
	reps, err := checkMembers(bytes.NewReader(input), &out, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 3 {
		t.Fatalf("got %d members, want 3", len(reps))
	}
	if out.String() != "hello world" {
		t.Fatalf("expanded %q", out.String())
	}
	if reps[0].StartOffset != 0 || reps[0].EndOffset != int64(len(m1)) {
		t.Fatalf("member0 range [%d,%d)", reps[0].StartOffset, reps[0].EndOffset)
	}
	if reps[1].StartOffset != int64(len(m1)) || reps[2].EndOffset != int64(len(input)) {
		t.Fatalf("bad offsets: %+v", reps)
	}
	if reps[0].Name != "first.txt" || reps[2].Comment != "last" {
		t.Fatalf("metadata lost: %+v", reps)
	}
	if reps[1].ExpandedBytes != 0 {
		t.Fatalf("empty member expanded %d bytes", reps[1].ExpandedBytes)
	}
	for i, r := range reps {
		if r.CompressedBytes <= 0 {
			t.Fatalf("member %d compressed bytes %d", i, r.CompressedBytes)
		}
	}
}

func TestCorruptTrailerCRC(t *testing.T) {
	m := gzipMember(t, "payload", nil)
	m[len(m)-5] ^= 0xff // flip a CRC byte
	_, err := checkMembers(bytes.NewReader(m), nil, Limits{})
	ce, ok := err.(*CheckError)
	if !ok || ce.Kind != ErrTrailer || ce.MemberIndex != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestCorruptTrailerSize(t *testing.T) {
	m := gzipMember(t, "payload", nil)
	m[len(m)-1] ^= 0x01 // flip an ISIZE byte
	_, err := checkMembers(bytes.NewReader(m), nil, Limits{})
	if ce, ok := err.(*CheckError); !ok || ce.Kind != ErrTrailer {
		t.Fatalf("got %v", err)
	}
}

func TestCorruptDeflate(t *testing.T) {
	m := gzipMember(t, strings.Repeat("abcdefgh", 64), nil)
	m[20] ^= 0xff // inside deflate stream
	_, err := checkMembers(bytes.NewReader(m), nil, Limits{})
	ce, ok := err.(*CheckError)
	if !ok || (ce.Kind != ErrDeflateCorrupt && ce.Kind != ErrTrailer) {
		t.Fatalf("got %v", err)
	}
}

func TestTruncatedHeader(t *testing.T) {
	m := gzipMember(t, "x", nil)
	_, err := checkMembers(bytes.NewReader(m[:5]), nil, Limits{})
	if ce, ok := err.(*CheckError); !ok || ce.Kind != ErrHeaderTruncated {
		t.Fatalf("got %v", err)
	}
}

func TestTrailingGarbage(t *testing.T) {
	m := gzipMember(t, "ok", nil)
	input := append(m, 0x00, 0x11, 0x22)
	_, err := checkMembers(bytes.NewReader(input), nil, Limits{})
	ce, ok := err.(*CheckError)
	if !ok || ce.Kind != ErrTrailingGarbage || ce.MemberIndex != 1 {
		t.Fatalf("got %v", err)
	}
	if ce.OffsetStart != int64(len(m)) {
		t.Fatalf("garbage offset %d, want %d", ce.OffsetStart, len(m))
	}
}
