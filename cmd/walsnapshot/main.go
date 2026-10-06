// Command walsnapshot verifies a SQLite WAL file offline against a baseline
// main database and optionally writes a rebuilt snapshot image.
//
// Usage:
//
//	go run ./cmd/walsnapshot -db main.db -wal main.db-wal [-out snapshot.db]
//
// It never invokes the SQLite engine and never modifies the input files.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"example.com/sqlite-wal-snapshot/wal"
)

type frameReport struct {
	Index      int    `json:"index"`
	Offset     int64  `json:"offset"`
	PageNumber uint32 `json:"pageNumber"`
	DBSize     uint32 `json:"dbSize"`
	Commit     bool   `json:"commit"`
}

type report struct {
	DBPath        string          `json:"dbPath"`
	WALPath       string          `json:"walPath"`
	PageSize      uint32          `json:"pageSize,omitempty"`
	ChecksumOrder string          `json:"checksumOrder,omitempty"`
	Frames        []frameReport   `json:"frames"`
	ValidFrames   int             `json:"validFrames"`
	LastCommit    int             `json:"lastCommit"`
	FirstError    string          `json:"firstError,omitempty"`
	ErrorOffset   int64           `json:"errorOffset,omitempty"`
	IgnoredTail   *tailRange      `json:"ignoredTail,omitempty"`
	Snapshot      *snapshotReport `json:"snapshot,omitempty"`
	WALHeaderErr  string          `json:"walHeaderError,omitempty"`
}

type tailRange struct {
	From  int64 `json:"fromOffset"`
	Bytes int64 `json:"bytes"`
}

type snapshotReport struct {
	Path     string           `json:"path"`
	Bytes    int64            `json:"bytes"`
	NoCommit bool             `json:"noCommit,omitempty"`
	Note     string           `json:"note"`
	PageSrc  []wal.PageSource `json:"pages,omitempty"`
}

func main() {
	dbPath := flag.String("db", "", "path to baseline main database file")
	walPath := flag.String("wal", "", "path to WAL file")
	outPath := flag.String("out", "", "optional path for rebuilt snapshot (must not exist)")
	flag.Parse()
	if *dbPath == "" || *walPath == "" {
		fmt.Fprintln(os.Stderr, "usage: walsnapshot -db <db> -wal <wal> [-out <snapshot>]")
		os.Exit(2)
	}
	if err := run(*dbPath, *walPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "walsnapshot:", err)
		os.Exit(1)
	}
}

func run(dbPath, walPath, outPath string) error {
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("read database: %w", err)
	}
	walData, err := os.ReadFile(walPath)
	if err != nil {
		return fmt.Errorf("read WAL: %w", err)
	}

	rep := &report{DBPath: dbPath, WALPath: walPath, LastCommit: -1}

	res, err := wal.Verify(walData)
	if err != nil {
		// Invalid WAL header: report only, never build a snapshot.
		rep.WALHeaderErr = err.Error()
		return emit(rep)
	}
	rep.PageSize = res.Header.PageSize
	if res.Header.BigEndian {
		rep.ChecksumOrder = "big-endian"
	} else {
		rep.ChecksumOrder = "little-endian"
	}
	for _, f := range res.Frames {
		rep.Frames = append(rep.Frames, frameReport{
			Index: f.Index, Offset: f.Offset, PageNumber: f.PageNumber,
			DBSize: f.DBSize, Commit: f.Commit,
		})
	}
	rep.ValidFrames = len(res.Frames)
	rep.LastCommit = res.LastCommit
	if res.FirstError != "" {
		rep.FirstError = res.FirstError
		rep.ErrorOffset = res.ErrorOffset
		rep.IgnoredTail = &tailRange{From: res.IgnoredFrom, Bytes: int64(len(walData)) - res.IgnoredFrom}
	}

	snap, err := wal.BuildSnapshot(dbData, res)
	if err != nil {
		rep.FirstError = joinErr(rep.FirstError, err.Error())
		return emit(rep)
	}
	sr := &snapshotReport{Bytes: int64(len(snap.Data)), NoCommit: snap.NoCommit,
		Note: snap.Note, PageSrc: snap.Pages}
	rep.Snapshot = sr

	if outPath != "" {
		if err := writeSnapshot(outPath, dbPath, walPath, snap.Data); err != nil {
			return err
		}
		sr.Path = outPath
	}
	return emit(rep)
}

func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func writeSnapshot(outPath, dbPath, walPath string, data []byte) error {
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			return p
		}
		return a
	}
	out := abs(outPath)
	if out == abs(dbPath) || out == abs(walPath) {
		return fmt.Errorf("refusing to overwrite source file: %s", outPath)
	}
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create snapshot (file must not already exist): %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}

func emit(rep *report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
