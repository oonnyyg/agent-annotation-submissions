// Package pack implements an offline reader for Git pack files (version 2,
// SHA-1 repositories). It parses the pack header and zlib-compressed entries,
// verifies the declared object count and the trailing SHA-1 checksum, and
// resolves delta chains without invoking git or touching any repository.
package pack

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
)

// ObjectType identifies the kind of a pack entry.
type ObjectType int

const (
	ObjCommit   ObjectType = 1
	ObjTree     ObjectType = 2
	ObjBlob     ObjectType = 3
	ObjTag      ObjectType = 4
	ObjOfsDelta ObjectType = 6
	ObjRefDelta ObjectType = 7
)

func (t ObjectType) String() string {
	switch t {
	case ObjCommit:
		return "commit"
	case ObjTree:
		return "tree"
	case ObjBlob:
		return "blob"
	case ObjTag:
		return "tag"
	case ObjOfsDelta:
		return "ofs-delta"
	case ObjRefDelta:
		return "ref-delta"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// Entry describes a single parsed pack entry.
type Entry struct {
	Index   int        // position in the pack (0-based)
	Offset  int64      // byte offset of the entry header within the pack
	Type    ObjectType // entry type as stored in the pack
	Size    int64      // declared size: object size, or delta data size for deltas
	Data    []byte     // inflated payload (object body, or delta instructions)
	BaseOff int64      // ObjOfsDelta: absolute offset of the base entry
	BaseID  [20]byte   // ObjRefDelta: identity of the base object
}

// Pack is a fully parsed and verified pack file.
type Pack struct {
	Entries []Entry
	byID    map[[20]byte]int
	byOff   map[int64]int
}

const (
	packMagic   = "PACK"
	packVersion = 2
	trailerLen  = sha1.Size
)

// Parse reads and validates a complete pack file from data.
func Parse(data []byte) (*Pack, error) {
	if len(data) < 12+trailerLen {
		return nil, fmt.Errorf("pack too small: %d bytes", len(data))
	}
	sum := sha1.Sum(data[:len(data)-trailerLen])
	var trailer [trailerLen]byte
	copy(trailer[:], data[len(data)-trailerLen:])
	if sum != trailer {
		return nil, fmt.Errorf("pack checksum mismatch: computed %x, trailer %x", sum, trailer)
	}
	if string(data[:4]) != packMagic {
		return nil, fmt.Errorf("bad pack magic %q", data[:4])
	}
	if v := binary.BigEndian.Uint32(data[4:8]); v != packVersion {
		return nil, fmt.Errorf("unsupported pack version %d", v)
	}
	count := binary.BigEndian.Uint32(data[8:12])

	body := data[:len(data)-trailerLen]
	p := &Pack{
		byID:  make(map[[20]byte]int),
		byOff: make(map[int64]int),
	}
	pos := int64(12)
	for i := uint32(0); i < count; i++ {
		e, next, err := parseEntry(body, pos, int(i))
		if err != nil {
			return nil, fmt.Errorf("entry %d at offset %d: %w", i, pos, err)
		}
		p.Entries = append(p.Entries, *e)
		p.byOff[e.Offset] = int(i)
		pos = next
	}
	if pos != int64(len(body)) {
		return nil, fmt.Errorf("pack has %d trailing bytes after %d declared entries", int64(len(body))-pos, count)
	}
	return p, nil
}

// parseEntry parses one entry starting at pos within body and returns the
// entry plus the offset of the next entry.
func parseEntry(body []byte, pos int64, index int) (*Entry, int64, error) {
	if pos >= int64(len(body)) {
		return nil, 0, fmt.Errorf("truncated entry header")
	}
	e := &Entry{Index: index, Offset: pos}
	c := body[pos]
	pos++
	e.Type = ObjectType((c >> 4) & 0x7)
	e.Size = int64(c & 0x0f)
	shift := uint(4)
	for c&0x80 != 0 {
		if pos >= int64(len(body)) {
			return nil, 0, fmt.Errorf("truncated size varint")
		}
		c = body[pos]
		pos++
		e.Size |= int64(c&0x7f) << shift
		shift += 7
		if shift > 63 {
			return nil, 0, fmt.Errorf("size varint too long")
		}
	}
	switch e.Type {
	case ObjCommit, ObjTree, ObjBlob, ObjTag:
	case ObjOfsDelta:
		if pos >= int64(len(body)) {
			return nil, 0, fmt.Errorf("truncated ofs-delta offset")
		}
		c = body[pos]
		pos++
		rel := int64(c & 0x7f)
		for c&0x80 != 0 {
			if pos >= int64(len(body)) {
				return nil, 0, fmt.Errorf("truncated ofs-delta offset")
			}
			c = body[pos]
			pos++
			rel = ((rel + 1) << 7) | int64(c&0x7f)
		}
		e.BaseOff = e.Offset - rel
		if e.BaseOff < 12 {
			return nil, 0, fmt.Errorf("ofs-delta base offset %d out of range", e.BaseOff)
		}
	case ObjRefDelta:
		if pos+20 > int64(len(body)) {
			return nil, 0, fmt.Errorf("truncated ref-delta base id")
		}
		copy(e.BaseID[:], body[pos:pos+20])
		pos += 20
	default:
		return nil, 0, fmt.Errorf("unknown object type %d", int(e.Type))
	}

	inflated, consumed, err := inflate(body[pos:])
	if err != nil {
		return nil, 0, err
	}
	if int64(len(inflated)) != e.Size {
		return nil, 0, fmt.Errorf("declared size %d but inflated %d bytes", e.Size, len(inflated))
	}
	e.Data = inflated
	return e, pos + consumed, nil
}

// inflate decompresses one zlib stream at the start of data and reports the
// exact number of compressed bytes consumed.
func inflate(data []byte) ([]byte, int64, error) {
	cr := &countingReader{r: bytes.NewReader(data)}
	zr, err := zlib.NewReader(cr)
	if err != nil {
		return nil, 0, fmt.Errorf("zlib stream: %w", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, 0, fmt.Errorf("inflating entry: %w", err)
	}
	if err := zr.Close(); err != nil {
		return nil, 0, fmt.Errorf("closing zlib stream: %w", err)
	}
	return out, cr.n, nil
}

type countingReader struct {
	r *bytes.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// ReadByte keeps zlib from wrapping the reader in a bufio.Reader, which
// would read past the end of the stream and corrupt the consumed count.
func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// IndexByID returns the entry index for a ref-delta base identity.
func (p *Pack) IndexByID(id [20]byte) (int, bool) {
	i, ok := p.byID[id]
	return i, ok
}

// IndexByOffset returns the entry index for an ofs-delta base offset.
func (p *Pack) IndexByOffset(off int64) (int, bool) {
	i, ok := p.byOff[off]
	return i, ok
}

// RegisterID records the resolved identity of an entry so that ref-deltas
// pointing at it (including entries appearing later in the pack) resolve.
func (p *Pack) RegisterID(index int, id [20]byte) {
	p.byID[id] = index
}
