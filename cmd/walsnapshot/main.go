// Command walsnapshot verifies a SQLite WAL file offline against a baseline
// database image and optionally rebuilds the last committed snapshot.
//
// Usage:
//
//	go run ./cmd/walsnapshot -db main.db -wal main.db-wal [-out snapshot.db]
//
// It never modifies the input files and never uses the SQLite engine.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"example.com/sqlite-wal-snapshot/internal/wal"
)

type frameReport struct {
	Index      int    `json:"index"`
	Offset     int64  `json:"offset"`
	PageNumber uint32 `json:"pageNumber"`
	DBSize     uint32 `json:"dbSize"`
	Commit     bool   `json:"commit"`
}

type report struct {
	DBPath            string          `json:"dbPath"`
	WALPath           string          `json:"walPath"`
	PageSize          uint32          `json:"pageSize"`
	ChecksumByteOrder string          `json:"checksumByteOrder"`
	Salt1             uint32          `json:"salt1"`
	Salt2             uint32          `json:"salt2"`
	Frames            []frameReport   `json:"frames"`
	ValidFrames       int             `json:"validFrames"`
	FirstError        *wal.ScanError  `json:"firstError,omitempty"`
	IgnoredTail       *tailRange      `json:"ignoredTail,omitempty"`
	LastCommit        int             `json:"lastCommitFrame"`
	Snapshot          *snapshotReport `json:"snapshot,omitempty"`
}

type tailRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type snapshotReport struct {
	Path     string           `json:"path"`
	DBSize   uint32           `json:"dbSize"`
	NoCommit bool             `json:"noCommit"`
	Note     string           `json:"note,omitempty"`
	Pages    []wal.PageSource `json:"pages"`
}

func main() {
	dbPath := flag.String("db", "", "主库（基线）文件路径")
	walPath := flag.String("wal", "", "WAL 文件路径")
	outPath := flag.String("out", "", "可选：快照输出路径（不得已存在，不得与源文件相同）")
	flag.Parse()
	if *dbPath == "" || *walPath == "" {
		fmt.Fprintln(os.Stderr, "用法: walsnapshot -db <主库> -wal <WAL 文件> [-out <快照文件>]")
		os.Exit(2)
	}
	if err := run(*dbPath, *walPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(dbPath, walPath, outPath string) error {
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("读取主库失败: %w", err)
	}
	walData, err := os.ReadFile(walPath)
	if err != nil {
		return fmt.Errorf("读取 WAL 失败: %w", err)
	}

	dbHdr, err := wal.ParseDBHeader(dbData)
	if err != nil {
		return fmt.Errorf("主库头校验失败: %w", err)
	}

	rep := &report{DBPath: dbPath, WALPath: walPath, PageSize: dbHdr.PageSize, LastCommit: -1}

	res, err := wal.Scan(walData)
	if err != nil {
		// WAL 头非法：输出报告但不生成快照。
		rep.FirstError = &wal.ScanError{Offset: 0, Reason: "WAL 头非法: " + err.Error()}
		return writeReport(rep)
	}
	if res.Header.PageSize != dbHdr.PageSize {
		return fmt.Errorf("页大小不一致: 主库 %d, WAL %d", dbHdr.PageSize, res.Header.PageSize)
	}

	rep.PageSize = res.Header.PageSize
	rep.ChecksumByteOrder = res.Header.Order.String()
	rep.Salt1 = res.Header.Salt1
	rep.Salt2 = res.Header.Salt2
	rep.ValidFrames = len(res.Frames)
	rep.LastCommit = res.LastCommit()
	for _, f := range res.Frames {
		rep.Frames = append(rep.Frames, frameReport{
			Index: f.Index, Offset: f.Offset, PageNumber: f.PageNumber,
			DBSize: f.DBSize, Commit: f.Commit(),
		})
	}
	if res.Truncated {
		rep.FirstError = res.Stop
		rep.IgnoredTail = &tailRange{Start: res.TailStart, End: res.TailEnd}
	}

	if outPath == "" {
		return writeReport(rep)
	}

	snap, err := wal.BuildSnapshot(res, dbData)
	if err != nil {
		return fmt.Errorf("构建快照失败: %w", err)
	}
	if err := writeSnapshot(outPath, dbPath, walPath, snap.Image); err != nil {
		return err
	}
	sr := &snapshotReport{
		Path: outPath, DBSize: snap.DBSize, NoCommit: snap.NoCommit, Pages: snap.Pages,
	}
	if snap.NoCommit {
		sr.Note = "WAL 有效前缀中没有提交帧，快照为主库基线的原样副本"
	}
	rep.Snapshot = sr
	return writeReport(rep)
}

// writeSnapshot writes the image, refusing to overwrite anything or to
// clobber a source path.
func writeSnapshot(out, dbPath, walPath string, image []byte) error {
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	for _, src := range []string{dbPath, walPath} {
		absSrc, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		if absOut == absSrc {
			return fmt.Errorf("拒绝写入: 输出路径 %q 与源文件相同", out)
		}
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("拒绝覆盖已有文件 %q", out)
		}
		return fmt.Errorf("创建快照文件失败: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(image); err != nil {
		return fmt.Errorf("写入快照失败: %w", err)
	}
	return nil
}

func writeReport(rep *report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
