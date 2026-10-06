package ledger

import (
	"errors"
	"testing"
)

func newSessionLedger(t *testing.T) *Ledger {
	t.Helper()
	l := New()
	if err := l.CreateSession("s1", "client-a", "tok-1", []string{"read", "write"}, 100); err != nil {
		t.Fatalf("create: %v", err)
	}
	return l
}

func TestRotateConsumesAndChains(t *testing.T) {
	l := newSessionLedger(t)
	if err := l.Rotate("tok-1", "client-a", "tok-2", []string{"read"}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	old := l.Tokens["tok-1"]
	if !old.Consumed {
		t.Fatal("old token not consumed")
	}
	succ := l.Tokens["tok-2"]
	if succ.ParentID != "tok-1" || succ.ExpiresAt != 100 {
		t.Fatalf("successor mismatch: %+v", succ)
	}
	if len(succ.Scopes) != 1 || succ.Scopes[0] != "read" {
		t.Fatalf("scopes not narrowed: %v", succ.Scopes)
	}
}

func TestDeniedRotationsDoNotMutate(t *testing.T) {
	l := newSessionLedger(t)
	cases := []struct {
		name string
		fn   func() error
		want error
	}{
		{"unknown token", func() error { return l.Rotate("nope", "client-a", "x", nil) }, ErrUnknownToken},
		{"client mismatch", func() error { return l.Rotate("tok-1", "client-b", "x", nil) }, ErrClientMismatch},
		{"scope widening", func() error { return l.Rotate("tok-1", "client-a", "x", []string{"admin"}) }, ErrScopeWidening},
		{"duplicate successor", func() error { return l.Rotate("tok-1", "client-a", "tok-1", nil) }, ErrDuplicateID},
	}
	for _, c := range cases {
		if err := c.fn(); !errors.Is(err, c.want) {
			t.Fatalf("%s: got %v want %v", c.name, err, c.want)
		}
	}
	if l.Tokens["tok-1"].Consumed {
		t.Fatal("denied rotations consumed the token")
	}
	if len(l.Tokens) != 1 || len(l.Events) != 1 {
		t.Fatal("denied rotations mutated the ledger")
	}
}

func TestExpiryAtDeadline(t *testing.T) {
	l := newSessionLedger(t)
	if err := l.Advance(99); err != nil {
		t.Fatal(err)
	}
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatalf("rotate before deadline: %v", err)
	}
	if err := l.Advance(1); err != nil {
		t.Fatal(err)
	}
	if err := l.Rotate("tok-2", "client-a", "tok-3", nil); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("at deadline: got %v", err)
	}
	if l.Tokens["tok-2"].Consumed {
		t.Fatal("expired rotation consumed the token")
	}
}

func TestReplayRevokesWholeFamily(t *testing.T) {
	l := newSessionLedger(t)
	for i, next := range []string{"tok-2", "tok-3", "tok-4"} {
		prev := []string{"tok-1", "tok-2", "tok-3"}[i]
		if err := l.Rotate(prev, "client-a", next, nil); err != nil {
			t.Fatalf("rotate %d: %v", i, err)
		}
	}
	// Replay the oldest consumed token by the bound client.
	if err := l.Rotate("tok-1", "client-a", "tok-5", nil); !errors.Is(err, ErrTokenConsumed) {
		t.Fatalf("replay: got %v", err)
	}
	st, _ := l.Status("s1")
	if !st.Revoked || st.RevokeReason != "replay" {
		t.Fatalf("status after replay: %+v", st)
	}
	for id, tok := range l.Tokens {
		if !tok.Consumed {
			t.Fatalf("family token %s still active", id)
		}
	}
	// No further rotation possible, even of the latest token.
	if err := l.Rotate("tok-4", "client-a", "tok-6", nil); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("post-revoke rotate: got %v", err)
	}
	// Trigger source recorded.
	hist, _ := l.History("s1")
	last := hist[len(hist)-1]
	if last.Kind != "replay_revoke" || last.TokenID != "tok-1" {
		t.Fatalf("replay event: %+v", last)
	}
}

func TestReplayByWrongClientIsPlainDenial(t *testing.T) {
	l := newSessionLedger(t)
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Rotate("tok-1", "client-b", "tok-3", nil); !errors.Is(err, ErrClientMismatch) {
		t.Fatalf("got %v", err)
	}
	st, _ := l.Status("s1")
	if st.Revoked {
		t.Fatal("wrong-client replay revoked the family")
	}
}

func TestUnrelatedSessionUnaffected(t *testing.T) {
	l := newSessionLedger(t)
	if err := l.CreateSession("s2", "client-c", "other-1", []string{"read"}, 100); err != nil {
		t.Fatal(err)
	}
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Rotate("tok-1", "client-a", "tok-3", nil); !errors.Is(err, ErrTokenConsumed) {
		t.Fatal(err)
	}
	if err := l.Rotate("other-1", "client-c", "other-2", nil); err != nil {
		t.Fatalf("unrelated session blocked: %v", err)
	}
}

func TestExplicitRevokeCoversSuccessors(t *testing.T) {
	l := newSessionLedger(t)
	if err := l.Rotate("tok-1", "client-a", "tok-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Revoke("s1"); err != nil {
		t.Fatal(err)
	}
	st, _ := l.Status("s1")
	if !st.Revoked || st.RevokeReason != "explicit" {
		t.Fatalf("status: %+v", st)
	}
	if err := l.Rotate("tok-2", "client-a", "tok-3", nil); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("rotate after revoke: got %v", err)
	}
}
