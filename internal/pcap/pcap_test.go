package pcap

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func buildFile(t *testing.T, magic uint32, order binary.ByteOrder, records [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint32(hdr[0:4], magic) // magic is read big-endian
	order.PutUint16(hdr[4:6], 2)
	order.PutUint16(hdr[6:8], 4)
	order.PutUint32(hdr[20:24], 1) // Ethernet
	buf.Write(hdr)
	for i, data := range records {
		rh := make([]byte, 16)
		order.PutUint32(rh[0:4], uint32(1700000000+i))
		order.PutUint32(rh[4:8], uint32(123+i))
		order.PutUint32(rh[8:12], uint32(len(data)))
		order.PutUint32(rh[12:16], uint32(len(data)))
		buf.Write(rh)
		buf.Write(data)
	}
	return buf.Bytes()
}

func TestEndiannessAndPrecision(t *testing.T) {
	cases := []struct {
		name  string
		magic uint32
		order binary.ByteOrder
		nano  bool
	}{
		{"micro-be", magicMicroBE, binary.BigEndian, false},
		{"micro-le", magicMicroLE, binary.LittleEndian, false},
		{"nano-be", magicNanoBE, binary.BigEndian, true},
		{"nano-le", magicNanoLE, binary.LittleEndian, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildFile(t, tc.magic, tc.order, [][]byte{{1, 2, 3}})
			rd, err := NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			rec, err := rd.Next()
			if err != nil {
				t.Fatal(err)
			}
			if rec.Index != 1 || rec.Sec != 1700000000 || rec.Subsec != 123 {
				t.Fatalf("bad record: %+v", rec)
			}
			if rec.Nano != tc.nano {
				t.Fatalf("nano=%v want %v", rec.Nano, tc.nano)
			}
			if !bytes.Equal(rec.Data, []byte{1, 2, 3}) {
				t.Fatalf("bad data %v", rec.Data)
			}
			if _, err := rd.Next(); err != io.EOF {
				t.Fatalf("want EOF, got %v", err)
			}
		})
	}
}

func TestTruncatedCaptureFlag(t *testing.T) {
	var buf bytes.Buffer
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint32(hdr[0:4], magicMicroLE)
	binary.LittleEndian.PutUint16(hdr[4:6], 2)
	binary.LittleEndian.PutUint16(hdr[6:8], 4)
	binary.LittleEndian.PutUint32(hdr[20:24], 1)
	buf.Write(hdr)
	rh := make([]byte, 16)
	binary.LittleEndian.PutUint32(rh[8:12], 10)  // captured
	binary.LittleEndian.PutUint32(rh[12:16], 60) // on wire
	buf.Write(rh)
	buf.Write(make([]byte, 10))
	rd, err := NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Truncated {
		t.Fatal("expected Truncated=true")
	}
}

func TestTruncatedRecordData(t *testing.T) {
	data := buildFile(t, magicMicroLE, binary.LittleEndian, [][]byte{{1, 2, 3, 4}})
	rd, err := NewReader(bytes.NewReader(data[:len(data)-2]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Next(); err == nil {
		t.Fatal("expected error for truncated packet data")
	}
}

func TestBadMagicAndLinkType(t *testing.T) {
	if _, err := NewReader(bytes.NewReader(make([]byte, 24))); err == nil {
		t.Fatal("expected bad magic error")
	}
	data := buildFile(t, magicMicroLE, binary.LittleEndian, nil)
	binary.LittleEndian.PutUint32(data[20:24], 101) // raw IP
	if _, err := NewReader(bytes.NewReader(data)); err == nil {
		t.Fatal("expected link type error")
	}
}
