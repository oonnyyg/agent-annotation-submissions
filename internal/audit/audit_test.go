package audit

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/torrent-piece-auditor/internal/torrent"
)

const testPieceLen = 16

// buildTorrent constructs v1 metainfo bytes for the given files and content.
func buildTorrent(t *testing.T, name string, files []torrent.File, content []byte) []byte {
	t.Helper()
	var pieces []byte
	for off := 0; off < len(content); off += testPieceLen {
		end := off + testPieceLen
		if end > len(content) {
			end = len(content)
		}
		h := sha1.Sum(content[off:end])
		pieces = append(pieces, h[:]...)
	}
	var info strings.Builder
	info.WriteString("d")
	if len(files) == 1 && len(files[0].Path) == 1 {
		fmt.Fprintf(&info, "6:lengthi%de", files[0].Length)
	} else {
		info.WriteString("5:filesl")
		for _, f := range files {
			fmt.Fprintf(&info, "d6:lengthi%de4:pathl", f.Length)
			for _, c := range f.Path[1:] {
				fmt.Fprintf(&info, "%d:%s", len(c), c)
			}
			info.WriteString("ee")
		}
		info.WriteString("e")
	}
	fmt.Fprintf(&info, "4:name%d:%s12:piece lengthi%de6:pieces%d:", len(name), name, testPieceLen, len(pieces))
	info.Write(pieces)
	info.WriteString("e")
	return []byte("d4:info" + info.String() + "e")
}

func writeFiles(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for rel, data := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mkFiles(name string, parts ...struct {
	rel string
	n   int
}) ([]torrent.File, map[string][]byte, []byte) {
	var files []torrent.File
	disk := map[string][]byte{}
	var content []byte
	var off int64
	for i, p := range parts {
		data := make([]byte, p.n)
		for j := range data {
			data[j] = byte(i*7 + j)
		}
		files = append(files, torrent.File{
			Path:   append([]string{name}, strings.Split(p.rel, "/")...),
			Length: int64(p.n),
			Offset: off,
		})
		disk[name+"/"+p.rel] = data
		content = append(content, data...)
		off += int64(p.n)
	}
	return files, disk, content
}

func parseAndRun(t *testing.T, metaBytes []byte, root string) *Report {
	t.Helper()
	m, err := torrent.Parse(metaBytes)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(m, root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAllOKMultiFile(t *testing.T) {
	files, disk, content := mkFiles("root",
		struct {
			rel string
			n   int
		}{"a.bin", 20},
		struct {
			rel string
			n   int
		}{"sub/b.bin", 30},
	)
	root := t.TempDir()
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "root", files, content), root)
	if r.Summary.PiecesCorrupt != 0 || r.Summary.PiecesUnverifiable != 0 {
		t.Fatalf("summary: %+v", r.Summary)
	}
	// 50 bytes / 16 = 4 pieces (last short: 2 bytes)
	if len(r.Pieces) != 4 || r.Pieces[3].Length != 2 {
		t.Fatalf("pieces: %+v", r.Pieces)
	}
	// Piece 1 spans a.bin tail and b.bin head.
	p1 := r.Pieces[1]
	if len(p1.Files) != 2 || p1.Files[0].Path != "root/a.bin" || p1.Files[0].FileOffset != 16 ||
		p1.Files[1].Path != "root/sub/b.bin" || p1.Files[1].FileOffset != 0 || p1.Files[1].Length != 12 {
		t.Fatalf("piece1 files: %+v", p1.Files)
	}
}

func TestEmptyFilesInterleaved(t *testing.T) {
	files, disk, content := mkFiles("root",
		struct {
			rel string
			n   int
		}{"a.bin", 10},
		struct {
			rel string
			n   int
		}{"empty1.bin", 0},
		struct {
			rel string
			n   int
		}{"b.bin", 6},
		struct {
			rel string
			n   int
		}{"empty2.bin", 0},
	)
	root := t.TempDir()
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "root", files, content), root)
	if r.Summary.PiecesOK != 1 || len(r.Pieces) != 1 {
		t.Fatalf("summary: %+v pieces %d", r.Summary, len(r.Pieces))
	}
	// The single piece must cover only the two non-empty files.
	if len(r.Pieces[0].Files) != 2 {
		t.Fatalf("piece0 files: %+v", r.Pieces[0].Files)
	}
}

func TestMissingFileKeepsAlignment(t *testing.T) {
	parts := []struct {
		rel string
		n   int
	}{
		{"a.bin", 16}, // piece 0
		{"gone.bin", 20},
		{"c.bin", 16},
	}
	files, disk, content := mkFiles("root", parts...)
	root := t.TempDir()
	delete(disk, "root/gone.bin")
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "root", files, content), root)
	// 52 bytes -> 4 pieces. Piece 0 ok; pieces 1,2 touch missing data;
	// piece 3 (c.bin bytes 36..52) must still verify at the right offset.
	if r.Pieces[0].Status != PieceOK {
		t.Fatalf("piece0: %+v", r.Pieces[0])
	}
	if r.Pieces[1].Status != PieceUnverifiable || r.Pieces[2].Status != PieceUnverifiable {
		t.Fatalf("pieces 1-2: %s %s", r.Pieces[1].Status, r.Pieces[2].Status)
	}
	if r.Pieces[3].Status != PieceOK {
		t.Fatalf("piece3 misaligned: %+v", r.Pieces[3])
	}
	if r.Summary.FilesMissing != 1 {
		t.Fatalf("summary: %+v", r.Summary)
	}
}

func TestCorruptPieceLocated(t *testing.T) {
	files, disk, content := mkFiles("root",
		struct {
			rel string
			n   int
		}{"a.bin", 16},
		struct {
			rel string
			n   int
		}{"b.bin", 16},
	)
	disk["root/b.bin"][5] ^= 0xff
	root := t.TempDir()
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "root", files, content), root)
	if r.Pieces[0].Status != PieceOK || r.Pieces[1].Status != PieceCorrupt {
		t.Fatalf("statuses: %s %s", r.Pieces[0].Status, r.Pieces[1].Status)
	}
	if r.Pieces[1].Files[0].Path != "root/b.bin" {
		t.Fatalf("corrupt piece files: %+v", r.Pieces[1].Files)
	}
}

func TestLengthMismatch(t *testing.T) {
	files, disk, content := mkFiles("root",
		struct {
			rel string
			n   int
		}{"a.bin", 20},
		struct {
			rel string
			n   int
		}{"b.bin", 16},
	)
	disk["root/b.bin"] = disk["root/b.bin"][:8] // truncated
	root := t.TempDir()
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "root", files, content), root)
	if r.Files[1].Status != FileLengthMismatch {
		t.Fatalf("file status: %+v", r.Files[1])
	}
	// Piece 1 spans a.bin tail (ok) + b.bin 8 real bytes + 8 missing.
	if r.Pieces[1].Status != PieceUnverifiable {
		t.Fatalf("piece1: %+v", r.Pieces[1])
	}
	if r.Pieces[2].Status != PieceUnverifiable {
		t.Fatalf("piece2: %+v", r.Pieces[2])
	}
}

func TestSingleFile(t *testing.T) {
	files, disk, content := mkFiles("solo.bin", struct {
		rel string
		n   int
	}{"", 40})
	// Single-file torrents address the file by the torrent name itself.
	files[0].Path = []string{"solo.bin"}
	disk = map[string][]byte{"solo.bin": disk["solo.bin/"]}
	root := t.TempDir()
	writeFiles(t, root, disk)
	r := parseAndRun(t, buildTorrent(t, "solo.bin", files, content), root)
	if r.Summary.PiecesOK != 3 {
		t.Fatalf("summary: %+v", r.Summary)
	}
}

func TestRejectsUnsafePaths(t *testing.T) {
	mk := func(comps ...string) *torrent.Meta {
		return &torrent.Meta{Files: []torrent.File{{Path: comps, Length: 1}}}
	}
	for _, m := range []*torrent.Meta{
		mk("..", "evil"),
		mk("ok", "..", "evil"),
		mk("a", "", "b"),
		mk("C:", "evil"),
	} {
		if err := torrent.ValidatePaths(m); err == nil {
			t.Errorf("expected rejection of %v", m.Files[0].Path)
		}
	}
	// Case-insensitive collision.
	m := &torrent.Meta{Files: []torrent.File{
		{Path: []string{"r", "A.txt"}, Length: 1},
		{Path: []string{"r", "a.TXT"}, Length: 1},
	}}
	if err := torrent.ValidatePaths(m); err == nil {
		t.Error("expected case collision rejection")
	}
}
