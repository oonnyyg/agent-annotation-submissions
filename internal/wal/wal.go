// Package wal implements offline verification of SQLite WAL files against a
// baseline database image, without using the SQLite engine.
package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	WalHeaderSize        = 32
	FrameHdrSize         = 24
	MagicLE       uint32 = 0x377f0682 // checksums use little-endian words
	MagicBE       uint32 = 0x377f0683 // checksums use big-endian words
	WalVersion           = 3007000
)

// ByteOrder selects the word order used by the WAL checksum algorithm.
type ByteOrder int

const (
	LittleEndianChecksum ByteOrder = iota
	BigEndianChecksum
)

func (b ByteOrder) String() string {
	if b == LittleEndianChecksum {
		return "little-endian"
	}
	return "big-endian"
}

// Header is a parsed WAL header.
type Header struct {
	Version       uint32
	PageSize      uint32
	CheckpointSeq uint32
	Salt1         uint32
	Salt2         uint32
	Checksum1     uint32
	Checksum2     uint32
	Order         ByteOrder
}

// ParseHeader parses and validates the 32-byte WAL header.
func ParseHeader(buf []byte) (*Header, error) {
	if len(buf) < WalHeaderSize {
		return nil, fmt.Errorf("WAL 头不足 %d 字节", WalHeaderSize)
	}
	magic := binary.BigEndian.Uint32(buf[0:4])
	var order ByteOrder
	switch magic {
	case MagicLE:
		order = LittleEndianChecksum
	case MagicBE:
		order = BigEndianChecksum
	default:
		return nil, fmt.Errorf("非法 WAL 魔数 0x%08x", magic)
	}
	h := &Header{
		Version:       binary.BigEndian.Uint32(buf[4:8]),
		PageSize:      binary.BigEndian.Uint32(buf[8:12]),
		CheckpointSeq: binary.BigEndian.Uint32(buf[12:16]),
		Salt1:         binary.BigEndian.Uint32(buf[16:20]),
		Salt2:         binary.BigEndian.Uint32(buf[20:24]),
		Checksum1:     binary.BigEndian.Uint32(buf[24:28]),
		Checksum2:     binary.BigEndian.Uint32(buf[28:32]),
		Order:         order,
	}
	if h.Version != WalVersion {
		return nil, fmt.Errorf("不支持的 WAL 版本 %d", h.Version)
	}
	if !validPageSize(h.PageSize) {
		return nil, fmt.Errorf("非法 WAL 页大小 %d", h.PageSize)
	}
	s1, s2 := Checksum(order, 0, 0, buf[0:24])
	if s1 != h.Checksum1 || s2 != h.Checksum2 {
		return nil, fmt.Errorf("WAL 头校验和不匹配: 期望 %08x %08x, 实际 %08x %08x",
			h.Checksum1, h.Checksum2, s1, s2)
	}
	return h, nil
}

func validPageSize(n uint32) bool {
	return n >= 512 && n <= 65536 && n&(n-1) == 0
}

// Checksum computes the SQLite WAL checksum over data (length must be a
// multiple of 8), continuing from (s1, s2). Words are read in the byte order
// mandated by the WAL magic, never the host byte order.
func Checksum(order ByteOrder, s1, s2 uint32, data []byte) (uint32, uint32) {
	if len(data)%8 != 0 {
		panic("checksum data length must be a multiple of 8")
	}
	for i := 0; i < len(data); i += 8 {
		var x0, x1 uint32
		if order == LittleEndianChecksum {
			x0 = binary.LittleEndian.Uint32(data[i:])
			x1 = binary.LittleEndian.Uint32(data[i+4:])
		} else {
			x0 = binary.BigEndian.Uint32(data[i:])
			x1 = binary.BigEndian.Uint32(data[i+4:])
		}
		s1 += x0 + s2
		s2 += x1 + s1
	}
	return s1, s2
}

// Frame is one verified WAL frame.
type Frame struct {
	Index      int   // 0-based frame index
	Offset     int64 // byte offset of the frame header in the WAL file
	PageNumber uint32
	DBSize     uint32 // non-zero only on commit frames
	Data       []byte // page image (PageSize bytes)
}

// Commit reports whether this frame marks a commit.
func (f *Frame) Commit() bool { return f.DBSize != 0 }

// ScanError describes the first anomaly that ended the valid frame prefix.
type ScanError struct {
	Offset int64
	Reason string
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("偏移 %d: %s", e.Offset, e.Reason)
}

// ScanResult holds the verified frame prefix and the first anomaly, if any.
type ScanResult struct {
	Header    *Header
	Frames    []*Frame // contiguous valid prefix
	Truncated bool     // true if scanning stopped before clean EOF
	Stop      *ScanError
	// TailStart/TailEnd describe the ignored byte range after the valid prefix.
	TailStart int64
	TailEnd   int64
}

// Scan verifies frames sequentially from the start of the WAL. It stops at
// the first bad or incomplete frame and never skips ahead.
func Scan(walData []byte) (*ScanResult, error) {
	h, err := ParseHeader(walData)
	if err != nil {
		return nil, err
	}
	res := &ScanResult{Header: h}
	s1, s2 := h.Checksum1, h.Checksum2
	frameSize := int64(FrameHdrSize) + int64(h.PageSize)
	total := int64(len(walData))
	off := int64(WalHeaderSize)
	idx := 0
	for {
		if off == total {
			break // clean EOF exactly on a frame boundary
		}
		if off+frameSize > total {
			res.Truncated = true
			res.Stop = &ScanError{Offset: off, Reason: fmt.Sprintf(
				"不完整尾帧: 需要 %d 字节, 仅剩 %d 字节", frameSize, total-off)}
			break
		}
		hdr := walData[off : off+FrameHdrSize]
		data := walData[off+FrameHdrSize : off+frameSize]
		pgno := binary.BigEndian.Uint32(hdr[0:4])
		dbSize := binary.BigEndian.Uint32(hdr[4:8])
		salt1 := binary.BigEndian.Uint32(hdr[8:12])
		salt2 := binary.BigEndian.Uint32(hdr[12:16])
		ck1 := binary.BigEndian.Uint32(hdr[16:20])
		ck2 := binary.BigEndian.Uint32(hdr[20:24])
		if pgno == 0 {
			res.Truncated = true
			res.Stop = &ScanError{Offset: off, Reason: "帧页号为 0，无效"}
			break
		}
		if salt1 != h.Salt1 || salt2 != h.Salt2 {
			res.Truncated = true
			res.Stop = &ScanError{Offset: off, Reason: fmt.Sprintf(
				"帧盐值 %08x/%08x 与 WAL 头 %08x/%08x 不一致", salt1, salt2, h.Salt1, h.Salt2)}
			break
		}
		ns1, ns2 := Checksum(h.Order, s1, s2, hdr[0:8])
		ns1, ns2 = Checksum(h.Order, ns1, ns2, data)
		if ns1 != ck1 || ns2 != ck2 {
			res.Truncated = true
			res.Stop = &ScanError{Offset: off, Reason: fmt.Sprintf(
				"帧累积校验和不匹配: 期望 %08x %08x, 计算得 %08x %08x", ck1, ck2, ns1, ns2)}
			break
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		res.Frames = append(res.Frames, &Frame{
			Index: idx, Offset: off, PageNumber: pgno, DBSize: dbSize, Data: cp,
		})
		s1, s2 = ns1, ns2
		off += frameSize
		idx++
	}
	if res.Truncated {
		res.TailStart = res.Stop.Offset
		res.TailEnd = total
	}
	return res, nil
}

// LastCommit returns the index (into Frames) of the last commit frame, or -1.
func (r *ScanResult) LastCommit() int {
	last := -1
	for i, f := range r.Frames {
		if f.Commit() {
			last = i
		}
	}
	return last
}

// DBHeader holds the fields of the main database header we validate.
type DBHeader struct {
	PageSize uint32
}

// ParseDBHeader validates the SQLite database header and returns its page size.
func ParseDBHeader(db []byte) (*DBHeader, error) {
	if len(db) < 100 {
		return nil, errors.New("主库文件不足 100 字节，无法包含数据库头")
	}
	const magic = "SQLite format 3\x00"
	if string(db[:16]) != magic {
		return nil, errors.New("主库缺少 'SQLite format 3' 头标识")
	}
	ps := uint32(binary.BigEndian.Uint16(db[16:18]))
	if ps == 1 {
		ps = 65536
	}
	if !validPageSize(ps) {
		return nil, fmt.Errorf("主库页大小 %d 非法", ps)
	}
	return &DBHeader{PageSize: ps}, nil
}
