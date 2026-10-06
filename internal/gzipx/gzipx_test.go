package gzipx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"hash/crc32"
	"strings"
	"testing"
)

func gzipData(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// manualMember builds a member with explicit flags, extra field, name,
// comment, and a correct FHCRC when requested.
func manualMember(t *testing.T, data []byte, flags byte, extra, name, comment string, hcrc bool) []byte {
	t.Helper()
	var out bytes.Buffer
	hdr := []byte{0x1f, 0x8b, 8, flags, 0, 0, 0, 0, 0, 255}
	out.Write(hdr)
	if flags&flagExtra != 0 {
		out.Write([]byte{byte(len(extra)), byte(len(extra) >> 8)})
		out.WriteString(extra)
	}
	if flags&flagName != 0 {
		out.WriteString(name)
		out.WriteByte(0)
	}
	if flags&flagComment != 0 {
		out.WriteString(comment)
		out.WriteByte(0)
	}
	if hcrc {
		c := uint16(crc32.ChecksumIEEE(out.Bytes()) & 0xffff)
		out.Write([]byte{byte(c), byte(c >> 8)})
	}
	fw, _ := flate.NewWriter(&out, flate.DefaultCompression)
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	var trailer [8]byte
	binary.LittleEndian.PutUint32(trailer[0:4], crc32.ChecksumIEEE(data))
	binary.LittleEndian.PutUint32(trailer[4:8], uint32(len(data)))
	out.Write(trailer[:])
	return out.Bytes()
}

func TestSingleMember(t *testing.T) {
	payload := []byte("hello gzip world")
	gz := gzipData(t, payload)
	var out bytes.Buffer
	rep := Inspect(bytes.NewReader(gz), Limits{}, &out)
	if !rep.OK {
		t.Fatalf("expected ok, got %+v", rep)
	}
	if rep.TotalMembers != 1 {
		t.Fatalf("members = %d", rep.TotalMembers)
	}
	m := rep.Members[0]
	if m.HeaderOffset != 0 || m.EndOffset != int64(len(gz)) {
		t.Fatalf("offsets %d..%d, file %d", m.HeaderOffset, m.EndOffset, len(gz))
	}
	if m.ExpandedBytes != uint64(len(payload)) || !bytes.Equal(out.Bytes(), payload) {
		t.Fatalf("expanded mismatch")
	}
}

func TestConcatenatedMembers(t *testing.T) {
	a := []byte("first-member-")
	b := []byte("")
	c := []byte("third member payload")
	gz := append(append(gzipData(t, a), gzipData(t, b)...), gzipData(t, c)...)
	var out bytes.Buffer
	rep := Inspect(bytes.NewReader(gz), Limits{}, &out)
	if !rep.OK {
		t.Fatalf("expected ok, got %s: %s", rep.ErrorKind, rep.Error)
	}
	if rep.TotalMembers != 3 {
		t.Fatalf("members = %d, want 3", rep.TotalMembers)
	}
	want := append(append(a, b...), c...)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("expanded = %q, want %q", out.Bytes(), want)
	}
	// Member boundaries must tile the file with no gaps or overlaps.
	for i, m := range rep.Members {
		if i > 0 && m.HeaderOffset != rep.Members[i-1].EndOffset {
			t.Fatalf("gap before member %d", i)
		}
	}
	if rep.Members[2].EndOffset != int64(len(gz)) {
		t.Fatalf("last end %d != file size %d", rep.Members[2].EndOffset, len(gz))
	}
	if rep.Members[1].ExpandedBytes != 0 {
		t.Fatalf("empty member expanded %d bytes", rep.Members[1].ExpandedBytes)
	}
}

func TestHeaderVariants(t *testing.T) {
	payload := []byte("metadata member")
	gz := manualMember(t, payload, flagExtra|flagName|flagComment|flagHCRC, "EXTRADATA", "file.txt", "a comment", true)
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if !rep.OK {
		t.Fatalf("expected ok, got %s: %s", rep.ErrorKind, rep.Error)
	}
	if rep.Members[0].Name != "file.txt" {
		t.Fatalf("name = %q", rep.Members[0].Name)
	}
}

func TestBadHeaderCRC(t *testing.T) {
	gz := manualMember(t, []byte("x"), flagHCRC, "", "", "", true)
	gz[10] ^= 0xff // corrupt the stored CRC16
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrHeaderCRC {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestTruncatedHeader(t *testing.T) {
	gz := gzipData(t, []byte("data"))[:5]
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrHeaderTruncated {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestCorruptDeflate(t *testing.T) {
	gz := gzipData(t, bytes.Repeat([]byte("abcdefgh"), 64))
	gz[len(gz)/2] ^= 0xff
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK {
		t.Fatal("expected failure")
	}
	if rep.ErrorKind != ErrDeflateCorrupt && rep.ErrorKind != ErrCRCMismatch {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestCRCMismatch(t *testing.T) {
	gz := gzipData(t, []byte("payload"))
	gz[len(gz)-8] ^= 0xff
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrCRCMismatch {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestISizeMismatch(t *testing.T) {
	gz := gzipData(t, []byte("payload"))
	gz[len(gz)-4] ^= 0xff
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrISizeMismatch {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestTruncatedTrailer(t *testing.T) {
	gz := gzipData(t, []byte("payload"))
	gz = gz[:len(gz)-3]
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrTrailerTruncated {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestTrailingGarbage(t *testing.T) {
	gz := append(gzipData(t, []byte("ok")), 0x00, 0x11, 0x22)
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrTrailingGarbage {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
	if rep.Members[len(rep.Members)-1].HeaderOffset != int64(len(gz)-3) {
		t.Fatalf("garbage offset = %d", rep.Members[len(rep.Members)-1].HeaderOffset)
	}
}

func TestReservedFlags(t *testing.T) {
	gz := manualMember(t, []byte("x"), 0x80, "", "", "", false)
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrReservedFlags {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestUnsupportedMethod(t *testing.T) {
	gz := manualMember(t, []byte("x"), 0, "", "", "", false)
	gz[2] = 7
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrUnsupportedAlgo {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestMemberLimit(t *testing.T) {
	gz := append(gzipData(t, []byte("a")), gzipData(t, []byte("b"))...)
	rep := Inspect(bytes.NewReader(gz), Limits{MaxMembers: 1}, nil)
	if rep.OK || rep.ErrorKind != ErrTooManyMembers {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
	// Exactly at the limit is fine.
	rep = Inspect(bytes.NewReader(gz), Limits{MaxMembers: 2}, nil)
	if !rep.OK {
		t.Fatalf("expected ok, got %s", rep.ErrorKind)
	}
}

func TestExpandedLimit(t *testing.T) {
	gz := gzipData(t, []byte(strings.Repeat("x", 1000)))
	rep := Inspect(bytes.NewReader(gz), Limits{MaxExpandedBytes: 100}, nil)
	if rep.OK || rep.ErrorKind != ErrExpandedLimit {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestHeaderLimit(t *testing.T) {
	gz := manualMember(t, []byte("x"), flagExtra, strings.Repeat("E", 500), "", "", false)
	rep := Inspect(bytes.NewReader(gz), Limits{MaxHeaderBytes: 64}, nil)
	if rep.OK || rep.ErrorKind != ErrHeaderTooLarge {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestEmptyInput(t *testing.T) {
	rep := Inspect(bytes.NewReader(nil), Limits{}, nil)
	if rep.OK || rep.ErrorKind != ErrNoMembers {
		t.Fatalf("kind = %s", rep.ErrorKind)
	}
}

func TestISizeModulo(t *testing.T) {
	// ISIZE is the length mod 2^32; verify normal case matches.
	payload := []byte("modulo check")
	gz := gzipData(t, payload)
	rep := Inspect(bytes.NewReader(gz), Limits{}, nil)
	if !rep.OK || rep.Members[0].ISize != uint32(len(payload)) {
		t.Fatalf("isize = %d", rep.Members[0].ISize)
	}
}
