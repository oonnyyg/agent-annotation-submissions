package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := newLedgerWithSession(t)
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Now != 100 || got.byToken["t2"].Parent != "t1" {
		t.Fatalf("round trip mismatch: %+v", got.byToken["t2"])
	}
}

func TestReplayDetectedAfterReload(t *testing.T) {
	dir := t.TempDir()
	l := newLedgerWithSession(t)
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t2", "client-a", "t3", nil); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := reloaded.Rotate("t1", "client-a", "t9", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replay {
		t.Fatal("replay of early token must be detected after reload")
	}
	if reloaded.byToken["t3"].Status != StatusRevoked {
		t.Fatal("whole family must be revoked after reload")
	}
}

func TestCorruptStoreRejected(t *testing.T) {
	dir := t.TempDir()
	l := newLedgerWithSession(t)
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, storeFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xff
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("expected ErrCorruptStore, got %v", err)
	}
}

func TestLoadMissingDirIsEmpty(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Sessions) != 0 {
		t.Fatal("expected empty ledger")
	}
}

func TestFailedSaveLeavesPreviousArchive(t *testing.T) {
	dir := t.TempDir()
	l := newLedgerWithSession(t)
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	// Force a save failure by pointing at a path that is a file, not a dir.
	file := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(file, "sub"), l); err == nil {
		t.Fatal("expected save error")
	}
	// Original archive still loads fine.
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	// No temp files leaked into the good dir.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != storeFile {
			t.Fatalf("unexpected leftover %s", e.Name())
		}
	}
}
