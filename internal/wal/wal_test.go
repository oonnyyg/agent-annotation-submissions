package wal

import (
	"encoding/binary"
	"testing"
)

const testPageSize = 512

// walBuilder constructs synthetic WAL files with correct salts/checksums.
type walBuilder struct {
	order ByteOrder
	salt1 uint32
	salt2 uint32
	buf   []byte
	s1    uint32
	s2    uint32
}

func newWalBuilder(order ByteOrder) *walBuilder {
	b := &walBuilder{order: order, salt1: 0xAABBCCDD, salt2: 0x11223344}
	hdr := make([]byte, WalHeaderSize)
	magic := MagicLE
	if order == BigEndianChecksum {
		magic = MagicBE
	}
	binary.BigEndian.PutUint32(hdr[0:4], magic)
	binary.BigEndian.PutUint32(hdr[4:8], WalVersion)
	binary.BigEndian.PutUint32(hdr[8:12], testPageSize)
	binary.BigEndian.PutUint32(hdr[16:20], b.salt1)
	binary.BigEndian.PutUint32(hdr[20:24], b.salt2)
	b.s1, b.s2 = Checksum(order, 0, 0, hdr[0:24])
	binary.BigEndian.PutUint32(hdr[24:28], b.s1)
	binary.BigEndian.PutUint32(hdr[28:32], b.s2)
	b.buf = hdr
	return b
}

// frame appends a frame whose page data is filled with fill.
func (b *walBuilder) frame(pgno, dbSize uint32, fill byte) *walBuilder {
	hdr := make([]byte, FrameHdrSize)
	binary.BigEndian.PutUint32(hdr[0:4], pgno)
	binary.BigEndian.PutUint32(hdr[4:8], dbSize)
	binary.BigEndian.PutUint32(hdr[8:12], b.salt1)
	binary.BigEndian.PutUint32(hdr[12:16], b.salt2)
	data := make([]byte, testPageSize)
	for i := range data {
		data[i] = fill
	}
	s1, s2 := Checksum(b.order, b.s1, b.s2, hdr[0:8])
	s1, s2 = Checksum(b.order, s1, s2, data)
	binary.BigEndian.PutUint32(hdr[16:20], s1)
	binary.BigEndian.PutUint32(hdr[20:24], s2)
	b.buf = append(b.buf, hdr...)
	b.buf = append(b.buf, data...)
	b.s1, b.s2 = s1, s2
	return b
}

func (b *walBuilder) bytes() []byte { return b.buf }

// makeBaseline builds a minimal valid database image of n pages, each page
// filled with a distinct marker byte.
func makeBaseline(n int) []byte {
	db := make([]byte, n*testPageSize)
	for p := 0; p < n; p++ {
		for i := 0; i < testPageSize; i++ {
			db[p*testPageSize+i] = byte(0x10 + p)
		}
	}
	copy(db[0:16], []byte("SQLite format 3\x00"))
	binary.BigEndian.PutUint16(db[16:18], testPageSize)
	return db
}

func TestScanBothByteOrders(t *testing.T) {
	for _, order := range []ByteOrder{LittleEndianChecksum, BigEndianChecksum} {
		b := newWalBuilder(order)
		b.frame(1, 3, 0xAA).frame(2, 0, 0xBB).frame(2, 3, 0xCC)
		res, err := Scan(b.bytes())
		if err != nil {
			t.Fatalf("order %v: %v", order, err)
		}
		if res.Truncated {
			t.Fatalf("order %v: unexpected truncation: %v", order, res.Stop)
		}
		if len(res.Frames) != 3 {
			t.Fatalf("order %v: got %d frames", order, len(res.Frames))
		}
		if res.LastCommit() != 2 {
			t.Fatalf("order %v: last commit = %d", order, res.LastCommit())
		}
	}
}

func TestCorruptFrameStopsPrefix(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 2, 0x01).frame(2, 2, 0x02)
	data := b.bytes()
	// Corrupt the first frame's page data; the second frame must not be accepted.
	data[WalHeaderSize+FrameHdrSize+10] ^= 0xFF
	res, err := Scan(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 0 {
		t.Fatalf("expected 0 valid frames, got %d", len(res.Frames))
	}
	if !res.Truncated || res.Stop.Offset != WalHeaderSize {
		t.Fatalf("expected stop at first frame, got %+v", res.Stop)
	}
	if res.TailStart != WalHeaderSize || res.TailEnd != int64(len(data)) {
		t.Fatalf("bad tail range %d..%d", res.TailStart, res.TailEnd)
	}
}

func TestIncompleteTailFrame(t *testing.T) {
	b := newWalBuilder(BigEndianChecksum)
	b.frame(1, 1, 0x77)
	data := append(b.bytes(), make([]byte, 100)...) // partial second frame
	res, err := Scan(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 1 || !res.Truncated {
		t.Fatalf("frames=%d truncated=%v", len(res.Frames), res.Truncated)
	}
	want := int64(WalHeaderSize + FrameHdrSize + testPageSize)
	if res.Stop.Offset != want {
		t.Fatalf("stop offset = %d, want %d", res.Stop.Offset, want)
	}
}

func TestBadSaltStopsPrefix(t *testing.T) {
	b := newWalBuilder(LittleEndianChecksum)
	b.frame(1, 1, 0x01)
	data := b.bytes()
	// Flip salt1 in the frame header and fix nothing else.
	binary.BigEndian.PutUint32(data[WalHeaderSize+8:WalHeaderSize+12], 0xDEADBEEF)
	res, err := Scan(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 0 || !res.Truncated {
		t.Fatalf("frames=%d truncated=%v", len(res.Frames), res.Truncated)
	}
}

func TestBadWalHeader(t *testing.T) {
	data := make([]byte, WalHeaderSize)
	binary.BigEndian.PutUint32(data[0:4], 0x12345678)
	if _, err := Scan(data); err == nil {
		t.Fatal("expected header error")
	}
}

func TestParseDBHeader(t *testing.T) {
	db := makeBaseline(2)
	h, err := ParseDBHeader(db)
	if err != nil {
		t.Fatal(err)
	}
	if h.PageSize != testPageSize {
		t.Fatalf("page size = %d", h.PageSize)
	}
	db[0] = 'X'
	if _, err := ParseDBHeader(db); err == nil {
		t.Fatal("expected magic error")
	}
}
