package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestHeaderCRCVerified(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// gzip.Writer does not emit FHCRC; build a header manually instead.
	// Default header is exactly 10 bytes, so the rest is deflate + trailer.
	w.Close()

	hdr := []byte{0x1f, 0x8b, 8, flagHCRC | flagName, 0, 0, 0, 0, 0, 255}
	hdr = append(hdr, []byte("named\x00")...)
	c := crc32.NewIEEE()
	c.Write(hdr)
	var crcBytes [2]byte
	binary.LittleEndian.PutUint16(crcBytes[:], uint16(c.Sum32()))
	good := append(append([]byte{}, hdr...), crcBytes[:]...)
	// reuse deflate stream + trailer from the gzip.Writer output
	body := buf.Bytes()[10:]
	good = append(good, body...)
	reps, err := checkMembers(bytes.NewReader(good), nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if reps[0].Name != "named" {
		t.Fatalf("name %q", reps[0].Name)
	}

	bad := append([]byte{}, good...)
	bad[10] = 'X' // corrupt name byte, header crc now wrong
	_, err = checkMembers(bytes.NewReader(bad), nil, Limits{})
	if ce, ok := err.(*CheckError); !ok || ce.Kind != ErrBadHeader {
		t.Fatalf("got %v", err)
	}
}

func TestRejectsReservedFlagsAndMethod(t *testing.T) {
	m := gzipMember(t, "x", nil)
	m[3] = 0x80 // reserved flag
	if _, err := checkMembers(bytes.NewReader(m), nil, Limits{}); err == nil {
		t.Fatal("reserved flags accepted")
	}
	m = gzipMember(t, "x", nil)
	m[2] = 7 // unsupported method
	ce, ok := func() (*CheckError, bool) {
		_, err := checkMembers(bytes.NewReader(m), nil, Limits{})
		ce, ok := err.(*CheckError)
		return ce, ok
	}()
	if !ok || ce.Kind != ErrBadHeader {
		t.Fatalf("got %v", ce)
	}
}

func TestLimits(t *testing.T) {
	m1 := gzipMember(t, "aaaa", nil)
	m2 := gzipMember(t, "bbbb", nil)
	input := append(m1, m2...)

	if _, err := checkMembers(bytes.NewReader(input), nil, Limits{MaxMembers: 1}); err == nil {
		t.Fatal("member limit not enforced")
	}
	_, err := checkMembers(bytes.NewReader(input), nil, Limits{MaxExpandedBytes: 5})
	if ce, ok := err.(*CheckError); !ok || ce.Kind != ErrLimit {
		t.Fatalf("got %v", err)
	}
	if _, err := checkMembers(bytes.NewReader(input), nil, Limits{MaxHeaderBytes: 5}); err == nil {
		t.Fatal("header limit not enforced")
	}
	// Exact fit passes.
	if _, err := checkMembers(bytes.NewReader(input), nil, Limits{MaxExpandedBytes: 8, MaxMembers: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestPublishAndCleanup(t *testing.T) {
	dir := t.TempDir()
	good := gzipMember(t, "data", nil)
	in := filepath.Join(dir, "in.gz")
	out := filepath.Join(dir, "out.bin")
	os.WriteFile(in, good, 0o644)

	cfg := Config{Input: in, Output: out}
	if _, err := check(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "data" {
		t.Fatalf("output %q", got)
	}
	// Existing output is never overwritten.
	if _, err := check(cfg); err == nil {
		t.Fatal("overwrote existing output")
	}
	// Failure leaves no output and no temp files.
	os.Remove(out)
	bad := append(good, 0xde, 0xad)
	os.WriteFile(in, bad, 0o644)
	if _, err := check(cfg); err == nil {
		t.Fatal("corrupt input accepted")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "in.gz" {
			t.Fatalf("leftover file %s", e.Name())
		}
	}
	// Check-only mode writes nothing.
	os.WriteFile(in, good, 0o644)
	if _, err := check(Config{Input: in}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("check-only mode created output")
	}
}
