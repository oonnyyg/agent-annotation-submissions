package main

import (
	"path/filepath"
	"testing"
)

func mustCreate(t *testing.T, s *Store, name string) {
	t.Helper()
	if err := s.CreateMailbox(name); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func TestUIDsArePerMailboxAndNeverReused(t *testing.T) {
	s := NewStore()
	mustCreate(t, s, "a")
	mustCreate(t, s, "b")
	a, _ := s.Mailbox("a")
	b, _ := s.Mailbox("b")
	if got := a.Append("m1"); got != 1 {
		t.Fatalf("first uid in a = %d, want 1", got)
	}
	a.Append("m2")
	a.Messages[0].Deleted = true
	a.Expunge()
	if got := a.Append("m3"); got != 3 {
		t.Fatalf("uid after expunge = %d, want 3 (no reuse)", got)
	}
	if got := b.Append("x"); got != 1 {
		t.Fatalf("first uid in b = %d, want 1 (independent counter)", got)
	}
	if len(a.Messages) != 2 || a.Messages[0].UID != 2 || a.Messages[1].UID != 3 {
		t.Fatalf("unexpected messages after expunge: %+v", a.Messages)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewStore()
	mustCreate(t, s, "inbox")
	mb, _ := s.Mailbox("inbox")
	mb.Append("hello")
	mb.Append("world")
	mb.Messages[1].Seen = true
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	mb2, err := loaded.Mailbox("inbox")
	if err != nil {
		t.Fatalf("mailbox missing after reload: %v", err)
	}
	if len(mb2.Messages) != 2 || mb2.Messages[1].UID != 2 || !mb2.Messages[1].Seen {
		t.Fatalf("state not preserved: %+v", mb2.Messages)
	}
	if got := mb2.Append("again"); got != 3 {
		t.Fatalf("uid after reload = %d, want 3", got)
	}
}

func TestLoadMissingFileGivesEmptyStore(t *testing.T) {
	s, err := LoadStore(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Mailboxes) != 0 {
		t.Fatalf("expected empty store, got %d mailboxes", len(s.Mailboxes))
	}
}
