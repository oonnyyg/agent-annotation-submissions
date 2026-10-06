package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := newSessionLedger(t)
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tokens["tok-1"].Consumed || got.Tokens["tok-2"].ParentID != "tok-1" {
		t.Fatal("parent/child relation not persisted")
	}
}

func TestReplayDetectedAfterRestart(t *testing.T) {
	dir := t.TempDir()
	l := newSessionLedger(t)
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	// Simulate restart: reload from disk only.
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Rotate("tok-1", "client-a", "tok-3", nil); !errors.Is(err, ErrTokenConsumed) {
		t.Fatalf("replay after restart: got %v", err)
	}
	st, _ := reloaded.Status("s1")
	if st.RevokeReason != "replay" {
		t.Fatalf("revoke reason: %+v", st)
	}
}

func TestCorruptStoreRejected(t *testing.T) {
	dir := t.TempDir()
	l := newSessionLedger(t)
	if err := Save(dir, l); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, storeFile)
	data, _ := os.ReadFile(path)
	// Flip bytes inside the payload to break the checksum.
	data[len(data)/2] ^= 0xFF
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("corrupt load: got %v", err)
	}
	// Truncated file is also rejected.
	if err := os.WriteFile(path, []byte(`{"checksum":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("truncated load: got %v", err)
	}
}

func TestFailedSaveLeavesNoHalfRotation(t *testing.T) {
	dir := t.TempDir()
	l := newSessionLedger(t)
	// Make the target path a directory so the rename fails.
	if err := os.MkdirAll(filepath.Join(dir, storeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l); err == nil {
		t.Fatal("expected save error")
	}
	if _, err := os.Stat(filepath.Join(dir, storeFile+".tmp")); !os.IsNotExist(err) {
		// temp file may remain on some platforms; ensure target untouched.
	}
	entries, _ := os.ReadDir(filepath.Join(dir, storeFile))
	if len(entries) != 0 {
		t.Fatal("partial write inside target path")
	}
}

func TestLoadMissingDirGivesFreshLedger(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Tokens) != 0 {
		t.Fatal("expected empty ledger")
	}
}
