// Package audit streams content files and verifies piece hashes.
package audit

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"example.com/torrent-piece-auditor/internal/torrent"
)

// Piece status values.
const (
	PieceOK           = "ok"
	PieceCorrupt      = "corrupt"
	PieceUnverifiable = "unverifiable"
)

// File status values.
const (
	FileOK             = "ok"
	FileMissing        = "missing"
	FileLengthMismatch = "length_mismatch"
)

// FileRange is the interval of a file covered by one piece.
type FileRange struct {
	Path       string `json:"path"`
	FileOffset int64  `json:"file_offset"`
	Length     int64  `json:"length"`
}

// PieceReport describes one piece and the file ranges it spans.
type PieceReport struct {
	Index  int64       `json:"index"`
	Offset int64       `json:"offset"`
	Length int64       `json:"length"`
	Status string      `json:"status"`
	SHA1   string      `json:"sha1,omitempty"`
	Files  []FileRange `json:"files"`
}

// FileReport describes one declared content file.
type FileReport struct {
	Path           string `json:"path"`
	DeclaredLength int64  `json:"declared_length"`
	ActualLength   *int64 `json:"actual_length"`
	Status         string `json:"status"`
}

// Summary aggregates the audit outcome.
type Summary struct {
	PiecesOK           int `json:"pieces_ok"`
	PiecesCorrupt      int `json:"pieces_corrupt"`
	PiecesUnverifiable int `json:"pieces_unverifiable"`
	FilesOK            int `json:"files_ok"`
	FilesMissing       int `json:"files_missing"`
	FilesLengthBad     int `json:"files_length_mismatch"`
}

// Report is the full audit result.
type Report struct {
	InfoHash    string        `json:"info_hash"`
	Name        string        `json:"name"`
	PieceLength int64         `json:"piece_length"`
	PieceCount  int64         `json:"piece_count"`
	TotalSize   int64         `json:"total_size"`
	Warnings    []string      `json:"warnings"`
	Pieces      []PieceReport `json:"pieces"`
	Files       []FileReport  `json:"files"`
	Summary     Summary       `json:"summary"`
}

const readBufSize = 1 << 20

// Run verifies the content under root against m. It never writes to root.
func Run(m *torrent.Meta, root string) (*Report, error) {
	if err := torrent.ValidatePaths(m); err != nil {
		return nil, err
	}
	r := &Report{
		InfoHash:    hex.EncodeToString(m.InfoHash[:]),
		Name:        m.Name,
		PieceLength: m.PieceLength,
		PieceCount:  int64(len(m.Pieces)),
		TotalSize:   m.TotalSize,
		Warnings:    m.Warnings,
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	a := &auditor{meta: m, report: r, buf: make([]byte, readBufSize)}
	for _, f := range m.Files {
		fr := a.auditFile(root, f)
		r.Files = append(r.Files, fr)
		switch fr.Status {
		case FileOK:
			r.Summary.FilesOK++
		case FileMissing:
			r.Summary.FilesMissing++
		case FileLengthMismatch:
			r.Summary.FilesLengthBad++
		}
	}
	if err := a.flush(); err != nil {
		return nil, err
	}
	if a.pieceIndex != int64(len(m.Pieces)) {
		return nil, fmt.Errorf("audit: consumed %d pieces, expected %d", a.pieceIndex, len(m.Pieces))
	}
	return r, nil
}

type auditor struct {
	meta   *torrent.Meta
	report *Report
	buf    []byte

	pieceIndex  int64
	pieceOffset int64 // absolute offset of current piece start
	pieceFilled int64 // bytes of current piece already fed to the hasher
	hasher      hash.Hash
	ranges      []FileRange
	broken      bool // current piece covers missing/unreadable data
}

func (a *auditor) auditFile(root string, f torrent.File) FileReport {
	path := strings.Join(f.Path, "/")
	fr := FileReport{Path: path, DeclaredLength: f.Length, Status: FileOK}
	abs, err := torrent.SafeJoin(root, f)
	if err != nil {
		// Paths were validated up front; treat a resolution failure as missing.
		fr.Status = FileMissing
		a.skip(f.Length, path)
		return fr
	}
	fh, err := os.Open(abs)
	if err != nil {
		fr.Status = FileMissing
		a.skip(f.Length, path)
		return fr
	}
	defer fh.Close()
	fi, err := fh.Stat()
	if err != nil || fi.IsDir() {
		fr.Status = FileMissing
		a.skip(f.Length, path)
		return fr
	}
	actual := fi.Size()
	fr.ActualLength = &actual
	readable := actual
	if readable > f.Length {
		readable = f.Length
	}
	if actual != f.Length {
		fr.Status = FileLengthMismatch
	}
	if err := a.consume(fh, readable, path); err != nil {
		fr.Status = FileMissing
		a.skip(f.Length-readable, path)
		return fr
	}
	if readable < f.Length {
		a.skip(f.Length-readable, path)
	}
	return fr
}

// consume streams n bytes from r into the piece hasher.
func (a *auditor) consume(r io.Reader, n int64, path string) error {
	fileOff := int64(0)
	for n > 0 {
		chunk := int64(len(a.buf))
		if pieceRemain := a.meta.PieceLength - a.pieceFilled; pieceRemain < chunk {
			chunk = pieceRemain
		}
		if n < chunk {
			chunk = n
		}
		if _, err := io.ReadFull(r, a.buf[:chunk]); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		a.feed(a.buf[:chunk], path, fileOff)
		fileOff += chunk
		n -= chunk
	}
	return nil
}

// skip advances the stream over n bytes that cannot be read, marking any
// piece they touch unverifiable while keeping piece boundaries aligned.
func (a *auditor) skip(n int64, path string) {
	fileOff := int64(0)
	for n > 0 {
		chunk := a.meta.PieceLength - a.pieceFilled
		if n < chunk {
			chunk = n
		}
		a.broken = true
		a.ranges = append(a.ranges, FileRange{Path: path, FileOffset: fileOff, Length: chunk})
		a.advance(chunk)
		fileOff += chunk
		n -= chunk
	}
}

func (a *auditor) feed(b []byte, path string, fileOff int64) {
	if a.hasher == nil {
		a.hasher = sha1.New()
	}
	a.hasher.Write(b)
	a.ranges = append(a.ranges, FileRange{Path: path, FileOffset: fileOff, Length: int64(len(b))})
	a.advance(int64(len(b)))
}

func (a *auditor) advance(n int64) {
	a.pieceFilled += n
	if a.pieceFilled == a.meta.PieceLength {
		if err := a.finishPiece(); err != nil {
			// finishPiece only errors on internal inconsistency; record it.
			a.broken = true
		}
	}
}

func (a *auditor) finishPiece() error {
	if a.pieceIndex >= int64(len(a.meta.Pieces)) {
		return errors.New("more content than declared pieces")
	}
	pr := PieceReport{
		Index:  a.pieceIndex,
		Offset: a.pieceOffset,
		Length: a.pieceFilled,
		Files:  a.ranges,
	}
	want := a.meta.Pieces[a.pieceIndex]
	switch {
	case a.broken:
		pr.Status = PieceUnverifiable
		a.report.Summary.PiecesUnverifiable++
	default:
		sum := a.hasher.Sum(nil)
		pr.SHA1 = hex.EncodeToString(sum)
		if string(sum) == string(want[:]) {
			pr.Status = PieceOK
			a.report.Summary.PiecesOK++
		} else {
			pr.Status = PieceCorrupt
			a.report.Summary.PiecesCorrupt++
		}
	}
	a.report.Pieces = append(a.report.Pieces, pr)
	a.pieceIndex++
	a.pieceOffset += a.pieceFilled
	a.pieceFilled = 0
	a.hasher = nil
	a.ranges = nil
	a.broken = false
	return nil
}

// flush completes the final, possibly short, piece.
func (a *auditor) flush() error {
	if a.pieceFilled > 0 {
		return a.finishPiece()
	}
	return nil
}
