package ledger

import (
	"reflect"
	"testing"
)

func mustGrant(t *testing.T, l *Ledger, date Date, id, emp string, start, end Date, min int) {
	t.Helper()
	if _, err := l.Grant(date, id, emp, start, end, min); err != nil {
		t.Fatalf("grant %s: %v", id, err)
	}
}

func TestCrossBatchRequest(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-02-01", 300)
	mustGrant(t, l, "2026-01-01", "B2", "alice", "2026-01-01", "2026-03-01", 300)
	res, err := l.Request("2026-01-10", "R1", "alice", 500)
	if err != nil {
		t.Fatal(err)
	}
	want := []Deduction{{Batch: "B1", Minutes: 300}, {Batch: "B2", Minutes: 200}}
	if !reflect.DeepEqual(res.Deductions, want) {
		t.Fatalf("deductions = %+v, want %+v", res.Deductions, want)
	}
	rep := l.Report("alice", "2026-01-10")
	if rep.Available != 100 || rep.Used != 500 || !rep.Balanced {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSameExpiryTieBreakByBatchID(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B2", "alice", "2026-01-01", "2026-02-01", 100)
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-02-01", 100)
	res, err := l.Request("2026-01-05", "R1", "alice", 150)
	if err != nil {
		t.Fatal(err)
	}
	want := []Deduction{{Batch: "B1", Minutes: 100}, {Batch: "B2", Minutes: 50}}
	if !reflect.DeepEqual(res.Deductions, want) {
		t.Fatalf("deductions = %+v, want %+v", res.Deductions, want)
	}
}

func TestInsufficientBalanceRejectsWholeRequest(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-02-01", 100)
	if _, err := l.Request("2026-01-05", "R1", "alice", 200); err == nil {
		t.Fatal("expected rejection")
	}
	if got := l.Batches["B1"].Remaining; got != 100 {
		t.Fatalf("partial deduction left remaining=%d", got)
	}
}

func TestCancelAfterExpiryReturnsExpired(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-02-01", 300)
	mustGrant(t, l, "2026-01-01", "B2", "alice", "2026-01-01", "2026-06-01", 300)
	if _, err := l.Request("2026-01-10", "R1", "alice", 400); err != nil {
		t.Fatal(err)
	}
	res, err := l.Cancel("2026-03-01", "R1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Expired, []Deduction{{Batch: "B1", Minutes: 300}}) {
		t.Fatalf("expired = %+v", res.Expired)
	}
	if !reflect.DeepEqual(res.Returns, []Deduction{{Batch: "B2", Minutes: 100}}) {
		t.Fatalf("returns = %+v", res.Returns)
	}
	if got := l.Batches["B1"].Remaining; got != 0 {
		t.Fatalf("expired batch regained balance: %d", got)
	}
	rep := l.Report("alice", "2026-03-01")
	if rep.Expired != 300 || rep.Available != 300 || rep.Used != 0 || !rep.Balanced {
		t.Fatalf("report = %+v", rep)
	}
	if _, err := l.Cancel("2026-03-02", "R1"); err == nil {
		t.Fatal("double cancel should fail")
	}
}

func TestDuplicateRequestReplayAndConflict(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-03-01", 500)
	first, err := l.Request("2026-01-10", "R1", "alice", 200)
	if err != nil {
		t.Fatal(err)
	}
	// Identical replay wins over the date check: last date is 2026-01-10,
	// replay uses the same date so it must not double-deduct.
	second, err := l.Request("2026-01-10", "R1", "alice", 200)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || !reflect.DeepEqual(first.Deductions, second.Deductions) {
		t.Fatalf("replay = %+v", second)
	}
	if got := l.Batches["B1"].Remaining; got != 300 {
		t.Fatalf("double deduction, remaining=%d", got)
	}
	// Replay must bypass the date-order check entirely.
	if _, err := l.Request("2026-01-20", "R2", "alice", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Request("2026-01-10", "R1", "alice", 200); err != nil {
		t.Fatalf("replay after later op should still succeed: %v", err)
	}
	if _, err := l.Request("2026-01-10", "R1", "alice", 250); err == nil {
		t.Fatal("conflicting content should fail")
	}
}

func TestDateCannotGoBackwards(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-10", "B1", "alice", "2026-01-01", "2026-03-01", 100)
	if _, err := l.Request("2026-01-05", "R1", "alice", 50); err == nil {
		t.Fatal("expected backwards-date error")
	}
}

func TestHalfOpenRangeAndPending(t *testing.T) {
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-02-01", "2026-03-01", 100)
	if _, err := l.Request("2026-01-15", "R1", "alice", 50); err == nil {
		t.Fatal("batch not yet effective should not be usable")
	}
	rep := l.Report("alice", "2026-01-15")
	if rep.Pending != 100 || !rep.Balanced {
		t.Fatalf("report = %+v", rep)
	}
	if _, err := l.Request("2026-03-01", "R2", "alice", 50); err == nil {
		t.Fatal("end date is exclusive")
	}
	rep = l.Report("alice", "2026-03-01")
	if rep.Lapsed != 100 || !rep.Balanced {
		t.Fatalf("report = %+v", rep)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := New()
	mustGrant(t, l, "2026-01-01", "B1", "alice", "2026-01-01", "2026-03-01", 500)
	if _, err := l.Request("2026-01-10", "R1", "alice", 200); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(dir); err != nil {
		t.Fatal(err)
	}
	l2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, err := l2.Usage("R1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Minutes != 200 || l2.Batches["B1"].Remaining != 300 || l2.LastDate != "2026-01-10" {
		t.Fatalf("reloaded state wrong: %+v %+v", u, l2.Batches["B1"])
	}
}
