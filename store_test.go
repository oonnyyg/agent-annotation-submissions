package main

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := LoadStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func appendN(t *testing.T, s *Store, mailbox string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		r := Execute(s, &Command{Cmd: "append", Mailbox: mailbox, Body: "msg"})
		if !r.OK {
			t.Fatalf("append: %s", r.Error)
		}
	}
}

func TestUIDsIncreaseAndNeverReuse(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateMailbox("inbox"); err != nil {
		t.Fatal(err)
	}
	appendN(t, s, "inbox", 3)
	mb, _ := s.MailboxByName("inbox")
	mb.Messages[1].Deleted = true
	removed := mb.Expunge()
	if len(removed) != 1 || removed[0].UID != 2 || removed[0].Seq != 2 {
		t.Fatalf("unexpected removed: %+v", removed)
	}
	// Remaining messages keep UIDs 1 and 3, renumbered to seq 1..2.
	if mb.Messages[0].UID != 1 || mb.Messages[1].UID != 3 {
		t.Fatalf("UIDs changed: %+v", mb.Messages)
	}
	// Next append gets UID 4, not the freed UID 2.
	if uid := mb.Append("x", false); uid != 4 {
		t.Fatalf("UID reused or wrong: got %d want 4", uid)
	}
}

func TestSelectionSeqAndUID(t *testing.T) {
	s := newTestStore(t)
	s.CreateMailbox("inbox")
	appendN(t, s, "inbox", 5)
	mb, _ := s.MailboxByName("inbox")

	// Overlapping ranges dedupe; missing numbers are ignored.
	sel := &Selection{By: "seq", Ranges: []NumRange{{1, 3}, {3, 4}, {9, 10}}}
	idx, err := sel.resolve(mb)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 4 || idx[0] != 0 || idx[3] != 3 {
		t.Fatalf("bad indexes: %v", idx)
	}

	// UID selection after expunge tracks the same messages.
	mb.Messages[1].Deleted = true // UID 2
	mb.Expunge()                  // UIDs now 1,3,4,5
	sel = &Selection{By: "uid", Ranges: []NumRange{{3, 4}}}
	idx, err = sel.resolve(mb)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 || mb.Messages[idx[0]].UID != 3 || mb.Messages[idx[1]].UID != 4 {
		t.Fatalf("uid selection wrong: %v", idx)
	}
}

func TestSelectionRejectsInvalid(t *testing.T) {
	mb := &Mailbox{Name: "m", NextUID: 1}
	mb.Append("a", false)
	cases := []Selection{
		{By: "seq", Ranges: []NumRange{{3, 1}}},   // inverted
		{By: "seq", Ranges: []NumRange{{0, 1}}},   // zero
		{By: "bogus", Ranges: []NumRange{{1, 1}}}, // bad mode
		{By: "seq", Ranges: nil},                  // empty
	}
	for i, sel := range cases {
		if _, err := sel.resolve(mb); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestFetchDoesNotMarkSeen(t *testing.T) {
	s := newTestStore(t)
	s.CreateMailbox("inbox")
	appendN(t, s, "inbox", 2)
	r := Execute(s, &Command{Cmd: "fetch", Mailbox: "inbox",
		Set: &Selection{By: "seq", Ranges: []NumRange{{1, 2}}}})
	if !r.OK || len(r.Messages) != 2 {
		t.Fatalf("fetch: %+v", r)
	}
	mb, _ := s.MailboxByName("inbox")
	if mb.Messages[0].Seen || mb.Messages[1].Seen {
		t.Fatal("fetch must not change seen state")
	}
	if r.Messages[0].Seq != 1 || r.Messages[0].UID != 1 {
		t.Fatalf("view missing numbering: %+v", r.Messages[0])
	}
}

func TestStoreFlagsAndExpungeRenumbers(t *testing.T) {
	s := newTestStore(t)
	s.CreateMailbox("inbox")
	appendN(t, s, "inbox", 4)
	tr, fa := true, false
	r := Execute(s, &Command{Cmd: "store", Mailbox: "inbox", Deleted: &tr,
		Set: &Selection{By: "uid", Ranges: []NumRange{{2, 3}}}})
	if !r.OK {
		t.Fatal(r.Error)
	}
	// Deleted flag does not remove messages yet.
	st := Execute(s, &Command{Cmd: "stats", Mailbox: "inbox"})
	if st.Stats.Messages != 4 || st.Stats.Deleted != 2 {
		t.Fatalf("stats before expunge: %+v", st.Stats)
	}
	r = Execute(s, &Command{Cmd: "store", Mailbox: "inbox", Seen: &fa, Deleted: &fa,
		Set: &Selection{By: "seq", Ranges: []NumRange{{1, 1}}}})
	if !r.OK {
		t.Fatal(r.Error)
	}
	ex := Execute(s, &Command{Cmd: "expunge", Mailbox: "inbox"})
	if !ex.OK || len(ex.Removed) != 2 || ex.Removed[0].Seq != 2 || ex.Removed[0].UID != 2 {
		t.Fatalf("expunge: %+v", ex)
	}
	mb, _ := s.MailboxByName("inbox")
	if len(mb.Messages) != 2 || mb.Messages[0].UID != 1 || mb.Messages[1].UID != 4 {
		t.Fatalf("renumber wrong: %+v", mb.Messages)
	}
}

func TestCopyPreservesAndAssignsNewUIDs(t *testing.T) {
	s := newTestStore(t)
	s.CreateMailbox("src")
	s.CreateMailbox("dst")
	tr := true
	Execute(s, &Command{Cmd: "append", Mailbox: "src", Body: "a"})
	Execute(s, &Command{Cmd: "append", Mailbox: "src", Body: "b", Seen: &tr})
	Execute(s, &Command{Cmd: "append", Mailbox: "src", Body: "c"})
	Execute(s, &Command{Cmd: "store", Mailbox: "src", Deleted: &tr,
		Set: &Selection{By: "seq", Ranges: []NumRange{{3, 3}}}})

	r := Execute(s, &Command{Cmd: "copy", Mailbox: "src", To: "dst",
		Set: &Selection{By: "seq", Ranges: []NumRange{{1, 3}}}})
	if !r.OK || len(r.Copied) != 3 {
		t.Fatalf("copy: %+v", r)
	}
	for i, m := range r.Copied {
		if m.SrcUID != uint64(i+1) || m.DstUID != uint64(i+1) || m.DstSeq != i+1 {
			t.Fatalf("mapping %d: %+v", i, m)
		}
	}
	dst, _ := s.MailboxByName("dst")
	if dst.Messages[0].Body != "a" || !dst.Messages[1].Seen || !dst.Messages[2].Deleted {
		t.Fatalf("flags/body not preserved: %+v", dst.Messages)
	}
	src, _ := s.MailboxByName("src")
	if len(src.Messages) != 3 || src.Messages[2].UID != 3 {
		t.Fatal("source mutated by copy")
	}
}

func TestCopyToMissingMailboxIsAtomic(t *testing.T) {
	s := newTestStore(t)
	s.CreateMailbox("src")
	appendN(t, s, "src", 2)
	r := Execute(s, &Command{Cmd: "copy", Mailbox: "src", To: "nope",
		Set: &Selection{By: "seq", Ranges: []NumRange{{1, 2}}}})
	if r.OK {
		t.Fatal("expected failure")
	}
	if len(s.Mailboxes) != 1 {
		t.Fatal("failed copy left partial changes")
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	s, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s.CreateMailbox("inbox")
	appendN(t, s, "inbox", 3)
	mb, _ := s.MailboxByName("inbox")
	mb.Messages[0].Deleted = true
	mb.Expunge()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	mb2, _ := s2.MailboxByName("inbox")
	if len(mb2.Messages) != 2 || mb2.Messages[0].UID != 2 || mb2.NextUID != 4 {
		t.Fatalf("state not continued: %+v next=%d", mb2.Messages, mb2.NextUID)
	}
	if uid := mb2.Append("x", false); uid != 4 {
		t.Fatalf("UID not continued after restart: %d", uid)
	}
}

func TestUnknownAndInvalidCommands(t *testing.T) {
	s := newTestStore(t)
	if r := Execute(s, &Command{Cmd: "nope"}); r.OK {
		t.Fatal("unknown cmd accepted")
	}
	if r := Execute(s, &Command{Cmd: "fetch", Mailbox: "ghost",
		Set: &Selection{By: "seq", Ranges: []NumRange{{1, 1}}}}); r.OK {
		t.Fatal("fetch on missing mailbox accepted")
	}
	if _, err := ParseCommand([]byte("{not json")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
