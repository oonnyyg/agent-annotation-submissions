// Package audit streams content files piece by piece and compares SHA-1
// hashes against the torrent metainfo, without ever loading more than a
// small buffer into memory.
package audit

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"example.com/torrent-piece-auditor/internal/torrent"
)

// File status values.
const (
	FileOK             = "ok"
	FileMissing        = "missing"
	FileLengthMismatch = "length_mismatch"
)

// Piece status values.
const (
	PieceOK           = "ok"
	PieceCorrupt      = "corrupt"
	PieceUnverifiable = "unverifiable"
)

// FileReport describes one declared content file.
type FileReport struct {
	Path           string `json:"path"`
	DeclaredLength int64  `json:"declared_length"`
	ActualLength   int64  `json:"actual_length"` // -1 when missing
	Status         string `json:"status"`
}

// Range is the part of a file covered by a piece.
type Range struct {
	Path       string `json:"path"`
	FileOffset int64  `json:"file_offset"`
	Length     int64  `json:"length"`
}

// PieceReport describes one piece.
type PieceReport struct {
	Index        int     `json:"index"`
	Offset       int64   `json:"offset"`
	Length       int64   `json:"length"`
	Status       string  `json:"status"`
	ExpectedSHA1 string  `json:"expected_sha1"`
	ActualSHA1   string  `json:"actual_sha1,omitempty"`
	Ranges       []Range `json:"ranges"`
}

// Summary aggregates the run.
type Summary struct {
	PiecesOK           int  `json:"pieces_ok"`
	PiecesCorrupt      int  `json:"pieces_corrupt"`
	PiecesUnverifiable int  `json:"pieces_unverifiable"`
	FilesOK            int  `json:"files_ok"`
	FilesWithIssues    int  `json:"files_with_issues"`
	Verified           bool `json:"verified"`
}

// Report is the full JSON output document.
type Report struct {
	TorrentFile string        `json:"torrent_file"`
	ContentRoot string        `json:"content_root"`
	Name        string        `json:"name"`
	InfoHash    string        `json:"info_hash"`
	PieceLength int64         `json:"piece_length"`
	NumPieces   int           `json:"num_pieces"`
	TotalLength int64         `json:"total_length"`
	Warnings    []string      `json:"warnings"`
	Files       []FileReport  `json:"files"`
	Pieces      []PieceReport `json:"pieces"`
	Summary     Summary       `json:"summary"`
}

type fileState struct {
	report FileReport
	full   string // validated absolute path
}

// Run verifies content under root against meta and returns the report.
func Run(meta *torrent.Meta, root, torrentPath string) (*Report, error) {
	if err := torrent.ValidatePaths(meta); err != nil {
		return nil, err
	}
	rep := &Report{
		TorrentFile: torrentPath,
		ContentRoot: root,
		Name:        meta.Name,
		InfoHash:    hex.EncodeToString(meta.InfoHash[:]),
		PieceLength: meta.PieceLength,
		NumPieces:   meta.NumPieces,
		TotalLength: meta.TotalLength,
		Warnings:    append([]string{}, meta.Warnings...),
	}

	states := make([]fileState, len(meta.Files))
	for i, f := range meta.Files {
		fr := FileReport{Path: torrent.DisplayPath(f.Path), DeclaredLength: f.Length, ActualLength: -1, Status: FileOK}
		full, err := torrent.SafeJoin(root, f.Path)
		if err != nil {
			return nil, err
		}
		info, statErr := os.Stat(full)
		switch {
		case statErr != nil:
			fr.Status = FileMissing
		case info.IsDir():
			fr.Status = FileMissing
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s is a directory, treated as missing", fr.Path))
		case info.Size() != f.Length:
			fr.Status = FileLengthMismatch
			fr.ActualLength = info.Size()
		default:
			fr.ActualLength = info.Size()
		}
		states[i] = fileState{report: fr, full: full}
		rep.Files = append(rep.Files, fr)
		if fr.Status == FileOK {
			rep.Summary.FilesOK++
		} else {
			rep.Summary.FilesWithIssues++
		}
	}

	if err := verifyPieces(meta, states, rep); err != nil {
		return nil, err
	}
	rep.Summary.Verified = rep.Summary.PiecesCorrupt == 0 &&
		rep.Summary.PiecesUnverifiable == 0 &&
		rep.Summary.FilesWithIssues == 0
	return rep, nil
}

const copyBufSize = 256 * 1024

func verifyPieces(meta *torrent.Meta, states []fileState, rep *Report) error {
	buf := make([]byte, copyBufSize)
	fileIdx := 0 // files are ordered by stream offset
	for p := 0; p < meta.NumPieces; p++ {
		start, end := meta.PieceRange(p)
		pr := PieceReport{
			Index:        p,
			Offset:       start,
			Length:       end - start,
			Status:       PieceOK,
			ExpectedSHA1: hex.EncodeToString(meta.PieceHash(p)),
		}
		h := sha1.New()
		unverifiable := false
		pos := start
		for pos < end {
			for fileIdx < len(meta.Files) &&
				meta.Files[fileIdx].Offset+meta.Files[fileIdx].Length <= pos {
				fileIdx++
			}
			if fileIdx >= len(meta.Files) {
				return fmt.Errorf("audit: internal offset error at piece %d", p)
			}
			f := meta.Files[fileIdx]
			segEnd := f.Offset + f.Length
			if segEnd > end {
				segEnd = end
			}
			segLen := segEnd - pos
			pr.Ranges = append(pr.Ranges, Range{
				Path:       torrent.DisplayPath(f.Path),
				FileOffset: pos - f.Offset,
				Length:     segLen,
			})
			st := states[fileIdx]
			if st.report.Status != FileOK {
				unverifiable = true
			} else if err := hashSegment(h, st.full, pos-f.Offset, segLen, buf); err != nil {
				unverifiable = true
				rep.Warnings = append(rep.Warnings,
					fmt.Sprintf("read %s: %v", st.report.Path, err))
			}
			pos = segEnd
		}
		switch {
		case unverifiable:
			pr.Status = PieceUnverifiable
			rep.Summary.PiecesUnverifiable++
		case bytes.Equal(h.Sum(nil), meta.PieceHash(p)):
			pr.ActualSHA1 = hex.EncodeToString(h.Sum(nil))
			rep.Summary.PiecesOK++
		default:
			pr.Status = PieceCorrupt
			pr.ActualSHA1 = hex.EncodeToString(h.Sum(nil))
			rep.Summary.PiecesCorrupt++
		}
		rep.Pieces = append(rep.Pieces, pr)
	}
	return nil
}

func hashSegment(h io.Writer, path string, off, length int64, buf []byte) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sr := io.NewSectionReader(f, off, length)
	_, err = io.CopyBuffer(h, sr, buf)
	return err
}
