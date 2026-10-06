package wal

import (
	"encoding/binary"
	"fmt"
)

const maxPageNumber = 0xFFFFFFFE

// Frame describes one verified WAL frame.
type Frame struct {
	Index      int   // 0-based frame index
	Offset     int64 // byte offset of frame header in the WAL file
	PageNumber uint32
	DBSize     uint32 // nonzero only on commit frames
	Commit     bool
	Data       []byte // page content, length PageSize
}

// VerifyResult is the outcome of scanning a WAL file.
type VerifyResult struct {
	Header        *Header
	Frames        []*Frame // valid prefix only
	LastCommit    int      // index into Frames of last commit frame, -1 if none
	FirstError    string   // empty if the whole file was consumed cleanly
	ErrorOffset   int64    // byte offset of the first anomaly, -1 if none
	IgnoredFrom   int64    // offset where the ignored tail begins, -1 if none
	TruncatedTail bool     // file ended with a partial frame
}

// Verify scans frames from the start of walData, stopping at the first
// anomaly. It never skips a bad frame to accept later ones.
func Verify(walData []byte) (*VerifyResult, error) {
	h, err := ParseHeader(walData)
	if err != nil {
		return nil, err
	}
	res := &VerifyResult{Header: h, LastCommit: -1, ErrorOffset: -1, IgnoredFrom: -1}
	s1, s2 := h.Checksum1, h.Checksum2
	frameSize := int64(FrameHeaderSize) + int64(h.PageSize)
	txnMaxPage := uint32(0)

	for off := int64(WALHeaderSize); ; off += frameSize {
		remain := int64(len(walData)) - off
		if remain == 0 {
			break
		}
		if remain < frameSize {
			res.TruncatedTail = true
			res.FirstError = fmt.Sprintf("incomplete trailing frame: %d of %d bytes present", remain, frameSize)
			res.ErrorOffset = off
			res.IgnoredFrom = off
			break
		}
		fh := walData[off : off+FrameHeaderSize]
		data := walData[off+FrameHeaderSize : off+frameSize]
		be := binary.BigEndian
		pgno := be.Uint32(fh[0:4])
		dbsize := be.Uint32(fh[4:8])
		salt1 := be.Uint32(fh[8:12])
		salt2 := be.Uint32(fh[12:16])
		ck1 := be.Uint32(fh[16:20])
		ck2 := be.Uint32(fh[20:24])

		fail := func(msg string) {
			res.FirstError = msg
			res.ErrorOffset = off
			res.IgnoredFrom = off
		}
		if salt1 != h.Salt1 || salt2 != h.Salt2 {
			fail(fmt.Sprintf("frame salt mismatch: got (%08x,%08x), want (%08x,%08x)",
				salt1, salt2, h.Salt1, h.Salt2))
			break
		}
		s1, s2 = checksum(h.BigEndian, s1, s2, fh[0:8])
		s1, s2 = checksum(h.BigEndian, s1, s2, data)
		if s1 != ck1 || s2 != ck2 {
			fail(fmt.Sprintf("frame checksum mismatch: got (%08x,%08x), want (%08x,%08x)",
				s1, s2, ck1, ck2))
			break
		}
		if pgno < 1 || pgno > maxPageNumber {
			fail(fmt.Sprintf("invalid page number %d", pgno))
			break
		}
		commit := dbsize != 0
		if commit && dbsize < pgno {
			fail(fmt.Sprintf("commit database size %d smaller than frame page %d", dbsize, pgno))
			break
		}
		if commit && txnMaxPage > dbsize {
			fail(fmt.Sprintf("commit database size %d smaller than transaction page %d", dbsize, txnMaxPage))
			break
		}
		idx := len(res.Frames)
		res.Frames = append(res.Frames, &Frame{
			Index: idx, Offset: off, PageNumber: pgno,
			DBSize: dbsize, Commit: commit, Data: data,
		})
		if commit {
			res.LastCommit = idx
			txnMaxPage = 0
		} else if pgno > txnMaxPage {
			txnMaxPage = pgno
		}
	}
	return res, nil
}
