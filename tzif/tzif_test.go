package tzif

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// buildTZif constructs a v2/v3 TZif file in memory for tests.
type testType struct {
	off   int32
	isdst bool
	abbr  string
}

func buildTZif(version byte, trans []int64, tind []uint8, types []testType, leapcnt uint32) []byte {
	var chars []byte
	ind := make([]uint8, len(types))
	for i, t := range types {
		ind[i] = uint8(len(chars))
		chars = append(chars, t.abbr...)
		chars = append(chars, 0)
	}
	writeHeader := func(buf *bytes.Buffer, timecnt, typecnt, charcnt, leap uint32) {
		buf.WriteString("TZif")
		buf.WriteByte(version)
		buf.Write(make([]byte, 15))
		for _, v := range []uint32{0, 0, leap, timecnt, typecnt, charcnt} {
			binary.Write(buf, binary.BigEndian, v)
		}
	}
	writeBlock := func(buf *bytes.Buffer, wide bool) {
		for _, t := range trans {
			if wide {
				binary.Write(buf, binary.BigEndian, t)
			} else {
				binary.Write(buf, binary.BigEndian, int32(t))
			}
		}
		buf.Write(tind)
		for i, t := range types {
			binary.Write(buf, binary.BigEndian, t.off)
			if t.isdst {
				buf.WriteByte(1)
			} else {
				buf.WriteByte(0)
			}
			buf.WriteByte(ind[i])
		}
		buf.Write(chars)
		for l := uint32(0); l < leapcnt; l++ {
			if wide {
				binary.Write(buf, binary.BigEndian, int64(0))
			} else {
				binary.Write(buf, binary.BigEndian, int32(0))
			}
			binary.Write(buf, binary.BigEndian, int32(1))
		}
	}
	var buf bytes.Buffer
	writeHeader(&buf, uint32(len(trans)), uint32(len(types)), uint32(len(chars)), leapcnt)
	writeBlock(&buf, false)
	writeHeader(&buf, uint32(len(trans)), uint32(len(types)), uint32(len(chars)), leapcnt)
	writeBlock(&buf, true)
	buf.WriteString("\n") // footer
	return buf.Bytes()
}

// sampleFile: non-hour offsets, transitions crossing 2038.
// Types: +08:45 "XST", +09:15 "XDT" (30-min forward jump), back to +08:45.
func sampleFile() []byte {
	return buildTZif('2',
		[]int64{1500000000, 2200000000, 2300000000, 2500000000},
		[]uint8{0, 1, 0, 1},
		[]testType{
			{off: 8*3600 + 45*60, isdst: false, abbr: "XST"},
			{off: 9*3600 + 15*60, isdst: true, abbr: "XDT"},
		}, 0)
}

func mustParse(t *testing.T, data []byte) *File {
	t.Helper()
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestParseValidV2(t *testing.T) {
	f := mustParse(t, sampleFile())
	if f.Version != '2' {
		t.Errorf("version = %q", f.Version)
	}
	first, last := f.Window()
	if first != 1500000000 || last != 2500000000 {
		t.Errorf("window = [%d,%d)", first, last)
	}
}

func TestParseV3(t *testing.T) {
	data := sampleFile()
	data[4] = '3'
	data[44+(4*4+4+2*6+8)+4] = '3' // second header version byte
	f := mustParse(t, data)
	if f.Version != '3' {
		t.Errorf("version = %q", f.Version)
	}
}

func TestTruncated(t *testing.T) {
	data := sampleFile()
	_, err := Parse(data[:len(data)-10])
	e, ok := err.(*Error)
	if !ok || e.Offset < 0 {
		t.Fatalf("err = %v, want *Error with offset", err)
	}
	if !strings.Contains(e.Msg, "truncated") {
		t.Errorf("msg = %q", e.Msg)
	}
}

func TestUnsortedTransitions(t *testing.T) {
	data := buildTZif('2', []int64{100, 300, 200}, []uint8{0, 0, 0},
		[]testType{{off: 0, abbr: "UTC"}}, 0)
	_, err := Parse(data)
	e := err.(*Error)
	if !strings.Contains(e.Msg, "strictly increasing") {
		t.Fatalf("err = %v", err)
	}
	// 64-bit block starts after header+v1block+header; index 2 at +16.
	v1size := int64(3*4 + 3 + 6 + 4)
	want := int64(44) + v1size + 44 + 16
	if e.Offset != want {
		t.Errorf("offset = %d, want %d", e.Offset, want)
	}
}

func TestLeapSecondsRejected(t *testing.T) {
	data := buildTZif('2', []int64{100, 200}, []uint8{0, 0},
		[]testType{{off: 0, abbr: "UTC"}}, 1)
	_, err := Parse(data)
	if !strings.Contains(err.Error(), "leap") {
		t.Fatalf("err = %v", err)
	}
}

func TestBadTypeIndex(t *testing.T) {
	data := buildTZif('2', []int64{100, 200}, []uint8{0, 5},
		[]testType{{off: 0, abbr: "UTC"}}, 0)
	_, err := Parse(data)
	if !strings.Contains(err.Error(), "type index 5") {
		t.Fatalf("err = %v", err)
	}
}

func TestTooFewTransitions(t *testing.T) {
	data := buildTZif('2', []int64{100}, []uint8{0},
		[]testType{{off: 0, abbr: "UTC"}}, 0)
	_, err := Parse(data)
	if !strings.Contains(err.Error(), "at least 2") {
		t.Fatalf("err = %v", err)
	}
}
