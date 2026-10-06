package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func parseErr(t *testing.T, data string) error {
	t.Helper()
	_, err := Parse([]byte(data))
	if err == nil {
		t.Fatalf("Parse(%q) succeeded, want error", data)
	}
	return err
}

func singleFileTorrent(infoInner string) string {
	return "d4:infod" + infoInner + "ee"
}

func TestInfoHashUsesRawBytes(t *testing.T) {
	// Keys intentionally out of order: re-encoding sorted would change the hash.
	raw := "6:pieces20:aaaaaaaaaaaaaaaaaaaa4:name1:x12:piece lengthi4e6:lengthi4e"
	m, err := Parse([]byte(singleFileTorrent(raw)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := sha1.Sum([]byte("d" + raw + "e"))
	if m.InfoHash != want {
		t.Fatalf("info hash = %s, want %s",
			hex.EncodeToString(m.InfoHash[:]), hex.EncodeToString(want[:]))
	}
	if len(m.Warnings) == 0 || !strings.Contains(m.Warnings[0], "out of order") {
		t.Fatalf("expected unsorted-keys warning, got %v", m.Warnings)
	}
}

func TestMetaValidation(t *testing.T) {
	good := "6:lengthi4e4:name1:x12:piece lengthi4e6:pieces20:aaaaaaaaaaaaaaaaaaaa"
	if _, err := Parse([]byte(singleFileTorrent(good))); err != nil {
		t.Fatalf("valid torrent rejected: %v", err)
	}
	parseErr(t, singleFileTorrent("4:name1:x12:piece lengthi4e6:pieces20:aaaaaaaaaaaaaaaaaaaa"))            // no length
	parseErr(t, singleFileTorrent("6:lengthi4e4:name1:x12:piece lengthi0e6:pieces20:aaaaaaaaaaaaaaaaaaaa")) // zero piece len
	parseErr(t, singleFileTorrent("6:lengthi4e4:name1:x12:piece lengthi4e6:pieces19:aaaaaaaaaaaaaaaaaaa"))  // bad pieces len
	parseErr(t, singleFileTorrent("6:lengthi9e4:name1:x12:piece lengthi4e6:pieces20:aaaaaaaaaaaaaaaaaaaa")) // piece count mismatch
	parseErr(t, "li1ee")                                                                                    // top-level not a dict
}

func metaWithPaths(paths ...[]string) *Meta {
	m := &Meta{}
	for _, p := range paths {
		m.Files = append(m.Files, FileEntry{Path: p})
	}
	return m
}

func TestValidatePathsRejectsUnsafe(t *testing.T) {
	bad := [][]string{
		{"..", "x"},
		{"a", "..", "b"},
		{"."},
		{""},
		{"a/b"},
		{`a\b`},
		{"C:", "Windows"},
		{"\x00"},
	}
	for _, p := range bad {
		if err := ValidatePaths(metaWithPaths(p)); err == nil {
			t.Errorf("path %v accepted, want rejection", p)
		}
	}
	if err := ValidatePaths(metaWithPaths([]string{"dir", "file.txt"})); err != nil {
		t.Errorf("valid path rejected: %v", err)
	}
}

func TestValidatePathsCaseConflict(t *testing.T) {
	m := metaWithPaths([]string{"Sub", "a.txt"}, []string{"sub", "a.txt"})
	err := ValidatePaths(m)
	if runtime.GOOS == "windows" {
		if err == nil {
			t.Fatal("case-insensitive conflict not rejected on Windows")
		}
	} else if err != nil {
		t.Fatalf("unexpected rejection on non-Windows: %v", err)
	}
}

func TestSafeJoinBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := SafeJoin(root, []string{"link", "secret.txt"}); err == nil {
		t.Fatal("symlink escape not blocked")
	}
	if _, err := SafeJoin(root, []string{"inner", "file.txt"}); err != nil {
		t.Fatalf("ordinary path blocked: %v", err)
	}
}
