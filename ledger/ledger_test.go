package ledger

import "testing"

func newLedgerWithSession(t *testing.T) *Ledger {
	t.Helper()
	l := NewLedger()
	if err := l.Advance(100); err != nil {
		t.Fatal(err)
	}
	if err := l.Create("s1", "client-a", []string{"read", "write"}, "t1", 200); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRotateHappyPath(t *testing.T) {
	l := newLedgerWithSession(t)
	res, err := l.Rotate("t1", "client-a", "t2", []string{"read"})
	if err != nil {
		t.Fatal(err)
	}
	if res.NewToken != "t2" || res.ExpiresAt != 200 {
		t.Fatalf("unexpected result %+v", res)
	}
	old := l.byToken["t1"]
	if old.Status != StatusConsumed || old.Child != "t2" {
		t.Fatalf("old token not consumed: %+v", old)
	}
	next := l.byToken["t2"]
	if next.Parent != "t1" || next.Status != StatusActive {
		t.Fatalf("bad successor: %+v", next)
	}
}

func TestRotateDefaultsToSameScopes(t *testing.T) {
	l := newLedgerWithSession(t)
	res, err := l.Rotate("t1", "client-a", "t2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Scopes) != 2 {
		t.Fatalf("scopes not inherited: %+v", res.Scopes)
	}
}

func TestOrdinaryRejectionsDoNotMutate(t *testing.T) {
	l := newLedgerWithSession(t)
	cases := []error{
		func() error { _, e := l.Rotate("nope", "client-a", "x", nil); return e }(),
		func() error { _, e := l.Rotate("t1", "client-b", "x", nil); return e }(),
		func() error { _, e := l.Rotate("t1", "client-a", "t1", nil); return e }(),
		func() error { _, e := l.Rotate("t1", "client-a", "x", []string{"read", "admin"}); return e }(),
	}
	want := []error{ErrTokenUnknown, ErrClientMismatch, ErrTokenIDTaken, ErrScopeExpansion}
	for i, err := range cases {
		if err != want[i] {
			t.Fatalf("case %d: got %v want %v", i, err, want[i])
		}
	}
	if l.byToken["t1"].Status != StatusActive {
		t.Fatal("rejections must not consume the current token")
	}
	if len(l.Sessions["s1"].History) != 1 {
		t.Fatal("rejections must not append history")
	}
}

func TestExpiryBoundary(t *testing.T) {
	l := newLedgerWithSession(t)
	if err := l.Advance(199); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal("rotation one second before expiry must succeed")
	}
	if err := l.Advance(200); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t2", "client-a", "t3", nil); err != ErrTokenNotActive {
		t.Fatalf("rotation at expiry must fail, got %v", err)
	}
	v, err := l.Query("s1")
	if err != nil {
		t.Fatal(err)
	}
	if v.State != SessionExpired {
		t.Fatalf("state = %s", v.State)
	}
}

func TestClockMonotonic(t *testing.T) {
	l := newLedgerWithSession(t)
	if err := l.Advance(99); err != ErrClockBackwards {
		t.Fatalf("got %v", err)
	}
}

func TestReplayRevokesWholeFamily(t *testing.T) {
	l := newLedgerWithSession(t)
	for _, next := range []string{"t2", "t3", "t4"} {
		prev := l.Sessions["s1"].Head
		if _, err := l.Rotate(prev, "client-a", next, nil); err != nil {
			t.Fatal(err)
		}
	}
	res, err := l.Rotate("t1", "client-a", "t9", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replay || !res.Revoked {
		t.Fatalf("expected replay revocation, got %+v", res)
	}
	for _, id := range []string{"t1", "t2", "t3", "t4"} {
		if l.byToken[id].Status != StatusRevoked {
			t.Fatalf("token %s not revoked", id)
		}
	}
	v, _ := l.Query("s1")
	if v.State != SessionRevoked || v.Revocation.Reason != "replay" || v.Revocation.Token != "t1" {
		t.Fatalf("bad revocation record: %+v", v.Revocation)
	}
	if _, err := l.Rotate("t4", "client-a", "t5", nil); err != ErrTokenNotActive {
		t.Fatalf("revoked family must not refresh, got %v", err)
	}
}

func TestReplayFromWrongClientIsOrdinaryRejection(t *testing.T) {
	l := newLedgerWithSession(t)
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t1", "client-b", "t9", nil); err != ErrClientMismatch {
		t.Fatalf("got %v", err)
	}
	if l.Sessions["s1"].Revoked != nil {
		t.Fatal("client mismatch must not revoke the family")
	}
}

func TestUnrelatedSessionUnaffected(t *testing.T) {
	l := newLedgerWithSession(t)
	if err := l.Create("s2", "client-b", []string{"read"}, "u1", 300); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Rotate("t1", "client-a", "t3", nil); err != nil {
		t.Fatal(err)
	}
	v, _ := l.Query("s2")
	if v.State != SessionActive {
		t.Fatalf("unrelated session state = %s", v.State)
	}
	if _, err := l.Rotate("u1", "client-b", "u2", nil); err != nil {
		t.Fatal("unrelated session must keep rotating")
	}
}

func TestExplicitRevokeCoversSuccessors(t *testing.T) {
	l := newLedgerWithSession(t)
	if _, err := l.Rotate("t1", "client-a", "t2", nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Revoke("t1", "client-a"); err != nil {
		t.Fatal(err)
	}
	if l.byToken["t2"].Status != StatusRevoked {
		t.Fatal("successor must be revoked too")
	}
	v, _ := l.Query("s1")
	if v.Revocation.Reason != "explicit" {
		t.Fatalf("reason = %s", v.Revocation.Reason)
	}
}

func TestCreateValidation(t *testing.T) {
	l := newLedgerWithSession(t)
	if err := l.Create("s1", "c", nil, "z", 500); err != ErrSessionExists {
		t.Fatalf("dup session: %v", err)
	}
	if err := l.Create("s3", "c", nil, "t1", 500); err != ErrTokenIDTaken {
		t.Fatalf("dup token id: %v", err)
	}
	if err := l.Create("s4", "c", nil, "z", 50); err != ErrInvalidExpiry {
		t.Fatalf("past expiry: %v", err)
	}
}
