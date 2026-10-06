// Package torrent parses v1 torrent metainfo and validates content paths.
package torrent

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"

	"example.com/torrent-piece-auditor/internal/bencode"
)

// File describes one content file in declaration (concatenation) order.
type File struct {
	Path   []string // path components relative to the content root
	Length int64
	Offset int64 // byte offset of this file in the concatenated stream
}

// Meta is the parsed torrent metainfo.
type Meta struct {
	Name        string
	PieceLength int64
	Pieces      [][20]byte
	Files       []File // single-file torrents yield exactly one entry
	TotalSize   int64
	InfoHash    [20]byte // SHA-1 over the raw bencoded info dictionary
	Warnings    []string
}

// Parse decodes raw torrent bytes and validates structural constraints.
func Parse(data []byte) (*Meta, error) {
	root, err := bencode.Decode(data)
	if err != nil {
		return nil, err
	}
	if root.Kind != bencode.Dict {
		return nil, errors.New("torrent: top-level value is not a dictionary")
	}
	m := &Meta{}
	if dups := bencode.DuplicateKeys(root); len(dups) > 0 {
		return nil, fmt.Errorf("torrent: duplicate dictionary keys: %s", strings.Join(dups, ", "))
	}
	for _, p := range bencode.UnsortedKeys(root) {
		m.Warnings = append(m.Warnings, "dictionary keys out of order at "+describePath(p))
	}

	info := root.Get("info")
	if info == nil || info.Kind != bencode.Dict {
		return nil, errors.New("torrent: missing or invalid info dictionary")
	}
	m.InfoHash = sha1.Sum(data[info.Start:info.End])

	name := info.Get("name")
	if name == nil || name.Kind != bencode.String || len(name.Str) == 0 {
		return nil, errors.New("torrent: info.name missing or not a non-empty string")
	}
	m.Name = string(name.Str)

	pl := info.Get("piece length")
	if pl == nil || pl.Kind != bencode.Int || pl.Int <= 0 {
		return nil, errors.New("torrent: info.piece length missing or not a positive integer")
	}
	m.PieceLength = pl.Int

	pieces := info.Get("pieces")
	if pieces == nil || pieces.Kind != bencode.String {
		return nil, errors.New("torrent: info.pieces missing or not a string")
	}
	if len(pieces.Str)%20 != 0 {
		return nil, fmt.Errorf("torrent: info.pieces length %d is not a multiple of 20", len(pieces.Str))
	}
	for i := 0; i < len(pieces.Str); i += 20 {
		var h [20]byte
		copy(h[:], pieces.Str[i:i+20])
		m.Pieces = append(m.Pieces, h)
	}

	if files := info.Get("files"); files != nil {
		if err := m.parseMulti(files); err != nil {
			return nil, err
		}
	} else {
		ln := info.Get("length")
		if ln == nil || ln.Kind != bencode.Int || ln.Int < 0 {
			return nil, errors.New("torrent: info.length missing or negative")
		}
		m.Files = []File{{Path: []string{m.Name}, Length: ln.Int}}
		m.TotalSize = ln.Int
	}

	want := m.TotalSize / m.PieceLength
	if m.TotalSize%m.PieceLength != 0 {
		want++
	}
	if int64(len(m.Pieces)) != want {
		return nil, fmt.Errorf("torrent: %d piece hashes but content requires %d pieces", len(m.Pieces), want)
	}
	return m, nil
}

func (m *Meta) parseMulti(files *bencode.Value) error {
	if files.Kind != bencode.List || len(files.List) == 0 {
		return errors.New("torrent: info.files missing or empty")
	}
	var off int64
	for i, fv := range files.List {
		if fv.Kind != bencode.Dict {
			return fmt.Errorf("torrent: files[%d] is not a dictionary", i)
		}
		ln := fv.Get("length")
		if ln == nil || ln.Kind != bencode.Int || ln.Int < 0 {
			return fmt.Errorf("torrent: files[%d].length missing or negative", i)
		}
		pl := fv.Get("path")
		if pl == nil || pl.Kind != bencode.List || len(pl.List) == 0 {
			return fmt.Errorf("torrent: files[%d].path missing or empty", i)
		}
		comps := make([]string, 0, len(pl.List)+1)
		comps = append(comps, m.Name)
		for j, cv := range pl.List {
			if cv.Kind != bencode.String || len(cv.Str) == 0 {
				return fmt.Errorf("torrent: files[%d].path[%d] missing or empty", i, j)
			}
			comps = append(comps, string(cv.Str))
		}
		m.Files = append(m.Files, File{Path: comps, Length: ln.Int, Offset: off})
		off += ln.Int
	}
	m.TotalSize = off
	return nil
}

func describePath(p string) string {
	if p == "" {
		return "<root>"
	}
	return p
}
