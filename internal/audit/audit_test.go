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

type testFile struct {
	path []string
	data []byte
}

// buildTorrent encodes a v1 multi-file torrent in memory and returns the
// metainfo bytes plus the concatenated content stream.
func buildTorrent(name string, pieceLen int64, files []testFile) []byte {
	var stream []byte
	var filesB strings.Builder
	for _, f := range files {
		stream = append(stream, f.data...)
		var pathB strings.Builder
		for _, c := range f.path {
			fmt.Fprintf(&pathB, "%d:%s", len(c), c)
		}
		fmt.Fprintf(&filesB, "d6:lengthi%de4:pathl%see", len(f.data), pathB.String())
	}
	var pieces []byte
	for off := 0; off < len(stream); off += int(pieceLen) {
		end := off + int(pieceLen)
		if end > len(stream) {
			end = len(stream)
		}
		sum := sha1.Sum(stream[off:end])
		pieces = append(pieces, sum[:]...)
	}
	info := fmt.Sprintf("d5:filesl%se4:name%d:%s12:piece lengthi%de6:pieces%d:%se",
		filesB.String(), len(name), name, pieceLen, len(pieces), pieces)
	return []byte(fmt.Sprintf("d8:announce0:4:info%se", info))
}

func writeContent(t *testing.T, root string, files []testFile) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(append([]string{root}, f.path...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runAudit(t *testing.T, metaBytes []byte, root string) *Report {
	t.Helper()
	meta, err := torrent.Parse(metaBytes)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rep, err := Run(meta, root, "test.torrent")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rep
}

func sampleFiles() []testFile {
	return []testFile{
		{path: []string{"a.bin"}, data: []byte("AAAAA")},         // 5 bytes
		{path: []string{"sub", "empty.bin"}, data: nil},          // 0 bytes, interleaved
		{path: []string{"sub", "b.bin"}, data: []byte("BBBBBB")}, // 6 bytes
	}
}

func TestCleanMultiFileWithEmpty(t *testing.T) {
	files := sampleFiles()
	root := t.TempDir()
	writeContent(t, root, files)
	rep := runAudit(t, buildTorrent("sample", 4, files), root)

	if !rep.Summary.Verified {
		t.Fatalf("expected verified, got %+v", rep.Summary)
	}
	if rep.Summary.PiecesOK != 3 { // ceil(11/4)
		t.Fatalf("pieces ok = %d, want 3", rep.Summary.PiecesOK)
	}
	// Empty file must not appear in any piece range.
	for _, p := range rep.Pieces {
		for _, r := range p.Ranges {
			if r.Path == "sub/empty.bin" {
				t.Fatalf("empty file appears in piece %d ranges", p.Index)
			}
		}
	}
	// Piece 1 spans the a.bin/empty.bin/b.bin boundary.
	p1 := rep.Pieces[1]
	if len(p1.Ranges) != 2 || p1.Ranges[0].Path != "a.bin" || p1.Ranges[1].Path != "sub/b.bin" {
		t.Fatalf("piece 1 ranges = %+v", p1.Ranges)
	}
	if p1.Ranges[0].FileOffset != 4 || p1.Ranges[0].Length != 1 ||
		p1.Ranges[1].FileOffset != 0 || p1.Ranges[1].Length != 3 {
		t.Fatalf("piece 1 range offsets wrong: %+v", p1.Ranges)
	}
	// Last piece is short.
	if got := rep.Pieces[2].Length; got != 3 {
		t.Fatalf("last piece length = %d, want 3", got)
	}
}

func TestMissingFileKeepsLaterPiecesAligned(t *testing.T) {
	files := sampleFiles()
	root := t.TempDir()
	writeContent(t, root, files)
	if err := os.Remove(filepath.Join(root, "sub", "b.bin")); err != nil {
		t.Fatal(err)
	}
	rep := runAudit(t, buildTorrent("sample", 4, files), root)

	if rep.Summary.Verified {
		t.Fatal("expected unverified")
	}
	if rep.Files[2].Status != FileMissing {
		t.Fatalf("b.bin status = %s", rep.Files[2].Status)
	}
	// Piece 0 covers only a.bin and stays verifiable.
	if rep.Pieces[0].Status != PieceOK {
		t.Fatalf("piece 0 status = %s", rep.Pieces[0].Status)
	}
	// Pieces 1 and 2 touch the missing file and must be unverifiable,
	// but their offsets must still follow the declared layout.
	for _, idx := range []int{1, 2} {
		p := rep.Pieces[idx]
		if p.Status != PieceUnverifiable {
			t.Fatalf("piece %d status = %s", idx, p.Status)
		}
		if p.Offset != int64(idx*4) {
			t.Fatalf("piece %d offset = %d, want %d", idx, p.Offset, idx*4)
		}
	}
}

func TestCorruptPiece(t *testing.T) {
	files := sampleFiles()
	root := t.TempDir()
	mutated := sampleFiles()
	mutated[2].data = []byte("BXBBBB") // corrupt b.bin
	writeContent(t, root, mutated)
	rep := runAudit(t, buildTorrent("sample", 4, files), root)

	if rep.Pieces[0].Status != PieceOK {
		t.Fatalf("piece 0 status = %s", rep.Pieces[0].Status)
	}
	if rep.Pieces[1].Status != PieceCorrupt || rep.Pieces[2].Status != PieceOK {
		t.Fatalf("pieces 1,2 statuses = %s,%s", rep.Pieces[1].Status, rep.Pieces[2].Status)
	}
	if rep.Pieces[1].ActualSHA1 == "" || rep.Pieces[1].ActualSHA1 == rep.Pieces[1].ExpectedSHA1 {
		t.Fatalf("corrupt piece actual hash wrong: %+v", rep.Pieces[1])
	}
}

func TestLengthMismatch(t *testing.T) {
	files := sampleFiles()
	root := t.TempDir()
	short := sampleFiles()
	short[0].data = []byte("AAA") // 3 instead of 5
	writeContent(t, root, short)
	rep := runAudit(t, buildTorrent("sample", 4, files), root)

	if rep.Files[0].Status != FileLengthMismatch || rep.Files[0].ActualLength != 3 {
		t.Fatalf("a.bin report = %+v", rep.Files[0])
	}
	if rep.Pieces[0].Status != PieceUnverifiable {
		t.Fatalf("piece 0 status = %s", rep.Pieces[0].Status)
	}
}

func TestSingleFile(t *testing.T) {
	data := []byte("hello world")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(data)
	info := fmt.Sprintf("d6:lengthi%de4:name9:hello.txt12:piece lengthi16e6:pieces20:%se",
		len(data), sum[:])
	metaBytes := []byte("d4:info" + info + "e")
	rep := runAudit(t, metaBytes, root)
	if !rep.Summary.Verified || rep.Summary.PiecesOK != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if rep.Files[0].Path != "hello.txt" {
		t.Fatalf("file path = %q", rep.Files[0].Path)
	}
}
