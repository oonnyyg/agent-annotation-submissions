// Package torrent parses v1 metainfo files and validates their structure.
package torrent

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"

	"example.com/torrent-piece-auditor/internal/bencode"
)

// FileEntry describes one content file in declaration order.
type FileEntry struct {
	Path   []string // path components relative to the content root
	Length int64
	Offset int64 // offset of this file's data in the concatenated stream
}

// Meta is a validated v1 torrent metainfo.
type Meta struct {
	InfoHash    [sha1.Size]byte
	PieceLength int64
	NumPieces   int
	TotalLength int64
	Name        string
	Files       []FileEntry
	Warnings    []string

	piecesRaw []byte
}

// Parse decodes and validates a v1 torrent file. The info hash is computed
// over the raw bencode bytes of the info dictionary, never a re-encoding.
func Parse(data []byte) (*Meta, error) {
	root, err := bencode.Decode(data)
	if err != nil {
		return nil, err
	}
	if root.Kind != bencode.Dict {
		return nil, errors.New("torrent: top-level value must be a dictionary")
	}
	m := &Meta{}
	collectOrderWarnings(root, "", &m.Warnings)

	info := root.Get("info")
	if info == nil || info.Kind != bencode.Dict {
		return nil, errors.New("torrent: missing or invalid \"info\" dictionary")
	}
	m.InfoHash = sha1.Sum(data[info.Start:info.End])

	name := info.Get("name")
	if name == nil || name.Kind != bencode.String || len(name.Str) == 0 {
		return nil, errors.New("torrent: info.name must be a non-empty string")
	}
	m.Name = string(name.Str)

	pl := info.Get("piece length")
	if pl == nil || pl.Kind != bencode.Int || pl.Int <= 0 {
		return nil, errors.New("torrent: info.piece length must be a positive integer")
	}
	m.PieceLength = pl.Int

	pieces := info.Get("pieces")
	if pieces == nil || pieces.Kind != bencode.String {
		return nil, errors.New("torrent: info.pieces must be a string")
	}
	if len(pieces.Str) == 0 || len(pieces.Str)%sha1.Size != 0 {
		return nil, fmt.Errorf("torrent: info.pieces length %d is not a positive multiple of 20", len(pieces.Str))
	}
	m.NumPieces = len(pieces.Str) / sha1.Size
	m.piecesRaw = pieces.Str

	if err := m.buildFiles(info); err != nil {
		return nil, err
	}
	wantPieces := 0
	if m.TotalLength > 0 {
		wantPieces = int((m.TotalLength + m.PieceLength - 1) / m.PieceLength)
	}
	if wantPieces != m.NumPieces {
		return nil, fmt.Errorf("torrent: %d piece hashes but total length %d implies %d pieces",
			m.NumPieces, m.TotalLength, wantPieces)
	}
	return m, nil
}

// PieceHash returns the expected SHA-1 for piece i.
func (m *Meta) PieceHash(i int) []byte {
	return m.piecesRaw[i*sha1.Size : (i+1)*sha1.Size]
}

// PieceRange returns the [start, end) stream offsets covered by piece i.
func (m *Meta) PieceRange(i int) (int64, int64) {
	start := int64(i) * m.PieceLength
	end := start + m.PieceLength
	if end > m.TotalLength {
		end = m.TotalLength
	}
	return start, end
}

func (m *Meta) buildFiles(info *bencode.Value) error {
	files := info.Get("files")
	if files == nil {
		length := info.Get("length")
		if length == nil || length.Kind != bencode.Int || length.Int < 0 {
			return errors.New("torrent: single-file info requires a non-negative \"length\"")
		}
		m.Files = []FileEntry{{Path: []string{m.Name}, Length: length.Int}}
		m.TotalLength = length.Int
		return nil
	}
	if files.Kind != bencode.List || len(files.List) == 0 {
		return errors.New("torrent: info.files must be a non-empty list")
	}
	var off int64
	for i, fv := range files.List {
		if fv.Kind != bencode.Dict {
			return fmt.Errorf("torrent: files[%d] must be a dictionary", i)
		}
		length := fv.Get("length")
		if length == nil || length.Kind != bencode.Int || length.Int < 0 {
			return fmt.Errorf("torrent: files[%d].length must be a non-negative integer", i)
		}
		pathVal := fv.Get("path")
		if pathVal == nil || pathVal.Kind != bencode.List || len(pathVal.List) == 0 {
			return fmt.Errorf("torrent: files[%d].path must be a non-empty list", i)
		}
		comps := make([]string, 0, len(pathVal.List))
		for _, cv := range pathVal.List {
			if cv.Kind != bencode.String {
				return fmt.Errorf("torrent: files[%d].path components must be strings", i)
			}
			comps = append(comps, string(cv.Str))
		}
		m.Files = append(m.Files, FileEntry{Path: comps, Length: length.Int, Offset: off})
		off += length.Int
	}
	m.TotalLength = off
	return nil
}

func collectOrderWarnings(v *bencode.Value, path string, warnings *[]string) {
	switch v.Kind {
	case bencode.Dict:
		if v.UnsortedKeys() {
			where := path
			if where == "" {
				where = "root"
			}
			*warnings = append(*warnings, "dictionary keys out of order at "+where)
		}
		for _, p := range v.Dict {
			collectOrderWarnings(p.Value, path+"."+string(p.Key), warnings)
		}
	case bencode.List:
		for i, item := range v.List {
			collectOrderWarnings(item, fmt.Sprintf("%s[%d]", path, i), warnings)
		}
	}
}

// DisplayPath joins components for report output.
func DisplayPath(comps []string) string {
	return strings.Join(comps, "/")
}
