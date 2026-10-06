package main

import "testing"

func boolp(b bool) *bool { return &b }

func setup(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	mustCreate(t, s, "inbox")
	mustCreate(t, s, "archive")
	mb, _ := s.Mailbox("inbox")
	mb.Append("one")
	mb.Append("two")
	mb.Append("three")
	return s
}

func TestAppendReturnsBothNumbers(t *testing.T) {
	s := NewStore()
	mustCreate(t, s, "inbox")
	resp, changed := Execute(s, &Command{Cmd: "append", Mailbox: "inbox", Body: "hi"})
	if !resp.OK || !changed || resp.UID != 1 || resp.Seq != 1 {
		t.Fatalf("append response = %+v, changed=%v", resp, changed)
	}
}

func TestStoreFlagsAndFetchKeepsUnseen(t *testing.T) {
	s := setup(t)
	_, changed := Execute(s, &Command{
		Cmd: "store", Mailbox: "inbox",
		Select: &Selection{By: "uid", Items: []string{"1:2"}},
		Seen:   boolp(true),
	})
	if !changed {
		t.Fatal("store should report a change")
	}
	resp, _ := Execute(s, &Command{Cmd: "fetch", Mailbox: "inbox",
		Select: &Selection{By: "seq", Items: []string{"1:3"}}})
	if !resp.OK || len(resp.Messages) != 3 {
		t.Fatalf("fetch = %+v", resp)
	}
	if !resp.Messages[0].Seen || !resp.Messages[1].Seen || resp.Messages[2].Seen {
		t.Fatalf("seen flags wrong: %+v", resp.Messages)
	}
	if resp.Messages[0].Body != "one" {
		t.Fatalf("fetch should include bodies: %+v", resp.Messages[0])
	}
	// Fetching must not mark anything seen.
	stats, _ := Execute(s, &Command{Cmd: "stats", Mailbox: "inbox"})
	if stats.Stats.Unseen != 1 {
		t.Fatalf("fetch changed unseen count: %+v", stats.Stats)
	}
}

func TestExpungeReturnsOldSeqAndRenumbers(t *testing.T) {
	s := setup(t)
	Execute(s, &Command{Cmd: "store", Mailbox: "inbox",
		Select: &Selection{By: "seq", Items: []string{"1", "3"}}, Deleted: boolp(true)})
	resp, _ := Execute(s, &Command{Cmd: "expunge", Mailbox: "inbox"})
	if !resp.OK || len(resp.Removed) != 2 {
		t.Fatalf("expunge = %+v", resp)
	}
	if resp.Removed[0].OldSeq != 1 || resp.Removed[0].UID != 1 ||
		resp.Removed[1].OldSeq != 3 || resp.Removed[1].UID != 3 {
		t.Fatalf("removed info wrong: %+v", resp.Removed)
	}
	list, _ := Execute(s, &Command{Cmd: "list", Mailbox: "inbox"})
	if len(list.Messages) != 1 || list.Messages[0].Seq != 1 || list.Messages[0].UID != 2 {
		t.Fatalf("survivor not renumbered to seq 1: %+v", list.Messages)
	}
}

func TestCopyPreservesFlagsAndAssignsNewUIDs(t *testing.T) {
	s := setup(t)
	Execute(s, &Command{Cmd: "store", Mailbox: "inbox",
		Select: &Selection{By: "uid", Items: []string{"2"}}, Seen: boolp(true)})
	resp, changed := Execute(s, &Command{Cmd: "copy", Mailbox: "inbox", To: "archive",
		Select: &Selection{By: "seq", Items: []string{"2", "1"}}}) // dedup keeps source order
	if !resp.OK || !changed || len(resp.Copied) != 2 {
		t.Fatalf("copy = %+v changed=%v", resp, changed)
	}
	if resp.Copied[0].SrcUID != 1 || resp.Copied[1].SrcUID != 2 {
		t.Fatalf("copies must follow source sequence order: %+v", resp.Copied)
	}
	if resp.Copied[0].DstUID != 1 || resp.Copied[1].DstUID != 2 {
		t.Fatalf("destination must assign fresh uids: %+v", resp.Copied)
	}
	list, _ := Execute(s, &Command{Cmd: "fetch", Mailbox: "archive"})
	if list.Messages[0].Body != "one" || list.Messages[0].Seen ||
		list.Messages[1].Body != "two" || !list.Messages[1].Seen {
		t.Fatalf("bodies/flags not preserved: %+v", list.Messages)
	}
	src, _ := Execute(s, &Command{Cmd: "stats", Mailbox: "inbox"})
	if src.Stats.Messages != 3 {
		t.Fatalf("source must be unchanged, got %+v", src.Stats)
	}
}

func TestFailedCommandsLeaveNoPartialChanges(t *testing.T) {
	s := setup(t)
	before := *s.Mailboxes["inbox"]

	cases := []*Command{
		{Cmd: "copy", Mailbox: "inbox", To: "missing",
			Select: &Selection{By: "seq", Items: []string{"1"}}},
		{Cmd: "store", Mailbox: "inbox",
			Select: &Selection{By: "seq", Items: []string{"9:2"}}, Seen: boolp(true)},
		{Cmd: "append", Mailbox: "missing", Body: "x"},
		{Cmd: "expunge", Mailbox: "missing"},
		{Cmd: "nonsense"},
	}
	for _, c := range cases {
		resp, changed := Execute(s, c)
		if resp.OK || changed {
			t.Errorf("cmd %+v: expected failure without change, got %+v changed=%v", c, resp, changed)
		}
	}
	after := *s.Mailboxes["inbox"]
	if len(after.Messages) != len(before.Messages) || after.NextUID != before.NextUID {
		t.Fatalf("failed commands mutated inbox: %+v", after)
	}
	if len(s.Mailboxes["archive"].Messages) != 0 {
		t.Fatal("failed copy left messages in archive")
	}
}

func TestStatsAndMailboxes(t *testing.T) {
	s := setup(t)
	Execute(s, &Command{Cmd: "store", Mailbox: "inbox",
		Select: &Selection{By: "seq", Items: []string{"1"}}, Seen: boolp(true), Deleted: boolp(true)})
	resp, _ := Execute(s, &Command{Cmd: "stats", Mailbox: "inbox"})
	st := resp.Stats
	if st.Messages != 3 || st.Unseen != 2 || st.Deleted != 1 || st.NextUID != 4 {
		t.Fatalf("stats = %+v", st)
	}
	names, _ := Execute(s, &Command{Cmd: "mailboxes"})
	if len(names.Mailboxes) != 2 || names.Mailboxes[0] != "archive" {
		t.Fatalf("mailboxes = %+v", names.Mailboxes)
	}
}
