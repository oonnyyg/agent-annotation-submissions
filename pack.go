// Package pack implements an offline reader for Git pack files (version 2,
// SHA-1 repositories). It parses pack entries, resolves delta chains, and
// reconstructs objects without invoking the git binary.
package pack

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
)

// Object types as stored in pack entry headers.
const (
	TypeCommit   = 1
	TypeTree     = 2
	TypeBlob     = 3
	TypeTag      = 4
	TypeOfsDelta = 6
	TypeRefDelta = 7
)

// TypeName maps a type code to its Git object name.
func TypeName(code int) string {
	switch code {
	case TypeCommit:
		return "commit"
	case TypeTree:
		return "tree"
	case TypeBlob:
		return "blob"
	case TypeTag:
		return "tag"
	case TypeOfsDelta:
		return "ofs-delta"
	case TypeRefDelta:
		return "ref-delta"
	default:
		return fmt.Sprintf("unknown(%d)", code)
	}
}

// Entry describes a single packed object record.
type Entry struct {
	Offset int64 // byte offset of the entry header within the pack
	Type   int   // pack type code (TypeCommit ... TypeRefDelta)
	Size   int64 // declared inflated size of this entry's data

	// Delta base reference (only for delta entries).
	BaseOffset int64  // OBJ_OFS_DELTA: absolute offset of the base entry
	BaseID     string // OBJ_REF_DELTA: hex object id of the base

	Data []byte // inflated entry payload (delta instructions for deltas)
}

// File is a parsed pack file.
type File struct {
	Entries  []*Entry
	Version  uint32
	Declared uint32
	Checksum [sha1.Size]byte // trailing pack checksum
}

const (
	packSignature = "PACK"
	headerSize    = 12
	checksumSize  = sha1.Size
)

// Parse reads and validates a complete pack file from data.
// It rejects unknown types, truncated data, checksum mismatches,
// declared-count mismatches, and trailing bytes after the checksum.
func Parse(data []byte) (*File, error) {
	if len(data) < headerSize+checksumSize {
		return nil, fmt.Errorf("pack too small: %d bytes", len(data))
	}
	var checksum [checksumSize]byte
	copy(checksum[:], data[len(data)-checksumSize:])
	body := data[:len(data)-checksumSize]
	actual := sha1.Sum(body)
	if actual != checksum {
		return nil, fmt.Errorf("pack checksum mismatch: got %x, want %x", actual, checksum)
	}

	if string(body[:4]) != packSignature {
		return nil, fmt.Errorf("bad pack signature %q", body[:4])
	}
	version := binary.BigEndian.Uint32(body[4:8])
	if version != 2 {
		return nil, fmt.Errorf("unsupported pack version %d (only 2 supported)", version)
	}
	declared := binary.BigEndian.Uint32(body[8:12])

	f := &File{Version: version, Declared: declared, Checksum: checksum}
	off := int64(headerSize)
	for i := uint32(0); i < declared; i++ {
		e, next, err := parseEntry(body, off)
		if err != nil {
			return nil, fmt.Errorf("entry %d at offset %d: %w", i, off, err)
		}
		f.Entries = append(f.Entries, e)
		off = next
	}
	if off != int64(len(body)) {
		if off < int64(len(body)) {
			return nil, fmt.Errorf("%d trailing bytes after %d declared entries", len(body)-int(off), declared)
		}
		return nil, fmt.Errorf("entries overran pack body")
	}
	return f, nil
}

// parseEntry decodes one entry starting at off and returns the entry plus the
// offset of the next entry.
func parseEntry(body []byte, off int64) (*Entry, int64, error) {
	if off >= int64(len(body)) {
		return nil, 0, io.ErrUnexpectedEOF
	}
	e := &Entry{Offset: off}

	// Variable-length type+size header.
	b := body[off]
	off++
	typ := int((b >> 4) & 0x7)
	size := int64(b & 0x0f)
	shift := uint(4)
	for b&0x80 != 0 {
		if off >= int64(len(body)) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		b = body[off]
		off++
		size |= int64(b&0x7f) << shift
		shift += 7
		if shift > 63 {
			return nil, 0, fmt.Errorf("entry size varint too long")
		}
	}
	e.Type = typ
	e.Size = size

	switch typ {
	case TypeCommit, TypeTree, TypeBlob, TypeTag:
		// no extra header
	case TypeOfsDelta:
		if off >= int64(len(body)) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		b = body[off]
		off++
		rel := int64(b & 0x7f)
		for b&0x80 != 0 {
			if off >= int64(len(body)) {
				return nil, 0, io.ErrUnexpectedEOF
			}
			b = body[off]
			off++
			rel = ((rel + 1) << 7) | int64(b&0x7f)
		}
		e.BaseOffset = e.Offset - rel
		if e.BaseOffset < headerSize {
			return nil, 0, fmt.Errorf("ofs-delta base offset %d out of range", e.BaseOffset)
		}
	case TypeRefDelta:
		if off+checksumSize > int64(len(body)) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		e.BaseID = fmt.Sprintf("%x", body[off:off+checksumSize])
		off += checksumSize
	default:
		return nil, 0, fmt.Errorf("unknown object type %d", typ)
	}

	// Inflate the zlib stream; track consumed bytes via the reader.
	br := bytes.NewReader(body[off:])
	zr, err := zlib.NewReader(br)
	if err != nil {
		return nil, 0, fmt.Errorf("zlib stream start: %w", err)
	}
	inflated, err := readAllLimit(zr, e.Size)
	closeErr := zr.Close()
	if err != nil {
		return nil, 0, err
	}
	if closeErr != nil {
		return nil, 0, fmt.Errorf("zlib stream close: %w", closeErr)
	}
	if int64(len(inflated)) != e.Size {
		return nil, 0, fmt.Errorf("inflated size %d does not match declared %d", len(inflated), e.Size)
	}
	e.Data = inflated
	consumed := int64(len(body)) - off - int64(br.Len())
	return e, off + consumed, nil
}

// readAllLimit reads at most limit+1 bytes so oversized streams are detected.
func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("negative declared size %d", limit)
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("zlib inflate: %w", err)
	}
	return buf, nil
}
