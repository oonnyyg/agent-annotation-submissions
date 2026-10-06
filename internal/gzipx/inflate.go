package gzipx

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// inflate expands one DEFLATE stream, updating the member report and the
// running expanded total. It returns the computed CRC32 of the plaintext.
func inflate(cr *countingReader, lim Limits, sink io.Writer, m *MemberReport, expandedTotal *uint64) (uint32, string, error) {
	fr := flate.NewReader(cr)
	defer fr.Close()

	sum := crc32.NewIEEE()
	buf := make([]byte, 32*1024)
	for {
		n, rerr := fr.Read(buf)
		chunk := buf[:n]
		if n > 0 {
			sum.Write(chunk)
			m.ExpandedBytes += uint64(n)
			*expandedTotal += uint64(n)
			if lim.MaxExpandedBytes > 0 && *expandedTotal > uint64(lim.MaxExpandedBytes) {
				return 0, ErrExpandedLimit, fmt.Errorf("member %d: expanded size exceeds limit %d bytes", m.Index, lim.MaxExpandedBytes)
			}
			if sink != nil {
				if _, err := sink.Write(chunk); err != nil {
					return 0, ErrDeflateCorrupt, fmt.Errorf("writing output: %v", err)
				}
			}
		}
		if rerr == nil {
			continue
		}
		if rerr == io.EOF {
			return sum.Sum32(), ErrNone, nil
		}
		var cerr flate.CorruptInputError
		if errors.As(rerr, &cerr) {
			return 0, ErrDeflateCorrupt, fmt.Errorf("member %d: corrupt deflate stream near compressed offset %d: %v", m.Index, m.DataOffset+int64(cerr), rerr)
		}
		if rerr == io.ErrUnexpectedEOF {
			return 0, ErrDeflateTruncated, fmt.Errorf("member %d: deflate stream truncated near offset %d", m.Index, cr.n)
		}
		return 0, ErrDeflateCorrupt, fmt.Errorf("member %d: %v", m.Index, rerr)
	}
}
