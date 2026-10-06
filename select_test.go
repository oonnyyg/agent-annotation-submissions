package main

import "testing"

func mailboxWith(n int) *Mailbox {
	mb := &Mailbox{Name: "t", NextUID: 1}
	for i := 0; i < n; i++ {
		mb.Append("body")
	}
	return mb
}

func TestSelectionDedupAndOverlap(t *testing.T) {
	mb := mailboxWith(5)
	idx, err := resolveSelection(mb, &Selection{By: "seq", Items: []string{"1:3", "2:4", "5"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(idx) != 5 {
		t.Fatalf("expected 5 deduplicated indexes, got %v", idx)
	}
}

func TestSelectionIgnoresMissingNumbers(t *testing.T) {
	mb := mailboxWith(3)
	idx, err := resolveSelection(mb, &Selection{By: "seq", Items: []string{"2", "99"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(idx) != 1 || idx[0] != 1 {
		t.Fatalf("expected only seq 2, got %v", idx)
	}
	idx, err = resolveSelection(mb, &Selection{By: "uid", Items: []string{"1", "42"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(idx) != 1 || idx[0] != 0 {
		t.Fatalf("expected only uid 1, got %v", idx)
	}
}

func TestSelectionRejectsInvalidInput(t *testing.T) {
	mb := mailboxWith(3)
	cases := []Selection{
		{By: "seq", Items: []string{"3:1"}}, // inverted range
		{By: "seq", Items: []string{"0"}},   // zero
		{By: "seq", Items: []string{"-2"}},  // negative
		{By: "seq", Items: []string{"abc"}}, // not a number
		{By: "seq", Items: nil},             // empty selection
		{By: "bogus", Items: []string{"1"}}, // unknown mode
	}
	for _, sel := range cases {
		if _, err := resolveSelection(mb, &sel); err == nil {
			t.Errorf("selection %+v: expected error, got nil", sel)
		}
	}
}

func TestUIDSelectionTracksMessagesAcrossSeqShift(t *testing.T) {
	mb := mailboxWith(4) // uids 1..4
	mb.Messages[0].Deleted = true
	mb.Expunge() // remaining uids 2,3,4 now at seq 1,2,3
	idx, err := resolveSelection(mb, &Selection{By: "uid", Items: []string{"4"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(idx) != 1 || mb.Messages[idx[0]].UID != 4 {
		t.Fatalf("uid 4 not found after renumbering: %v", idx)
	}
}
