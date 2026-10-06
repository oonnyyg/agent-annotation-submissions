package ledger

import "testing"

func grant(t *testing.T, l *Ledger, id, start, end string, minutes int, date string) {
	t.Helper()
	err := l.Grant(Batch{ID: id, Employee: "alice", Start: start, End: end, Minutes: minutes}, date)
	if err != nil {
		t.Fatalf("grant %s: %v", id, err)
	}
}

func TestCrossBatchApply(t *testing.T) {
	l := New(NewState())
	// B2 expires earlier but has lexicographically larger ID; B1 added
	// later must not change the frozen plan of earlier requests.
	grant(t, l, "B2", "2026-01-01", "2026-02-01", 100, "2026-01-01")
	grant(t, l, "B1", "2026-01-01", "2026-03-01", 200, "2026-01-01")

	res, err := l.Apply("R1", "alice", "2026-01-10", 150)
	if err != nil || res.Status != "approved" {
		t.Fatalf("apply: %v %+v", err, res)
	}
	want := []Allocation{{BatchID: "B2", Minutes: 100}, {BatchID: "B1", Minutes: 50}}
	if len(res.Allocs) != 2 || res.Allocs[0] != want[0] || res.Allocs[1] != want[1] {
		t.Fatalf("allocs = %+v, want %+v", res.Allocs, want)
	}

	rep, err := l.Balance("alice", "2026-01-10")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Available != 150 || rep.Used != 150 || rep.Total != 300 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestInsufficientBalanceRejectsWhole(t *testing.T) {
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-02-01", 100, "2026-01-01")
	res, err := l.Apply("R1", "alice", "2026-01-05", 101)
	if err != nil || res.Status != "rejected" {
		t.Fatalf("apply: %v %+v", err, res)
	}
	if got := l.st.Batches["B1"].Used; got != 0 {
		t.Fatalf("partial deduction left behind: used=%d", got)
	}
}

func TestCancelAfterExpiry(t *testing.T) {
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-02-01", 100, "2026-01-01")
	grant(t, l, "B2", "2026-01-01", "2026-04-01", 100, "2026-01-01")
	if _, err := l.Apply("R1", "alice", "2026-01-10", 150); err != nil {
		t.Fatal(err)
	}
	// Cancel after B1 expired: its 100 minutes become expired returns.
	res, err := l.Cancel("R1", "2026-02-10")
	if err != nil {
		t.Fatal(err)
	}
	if res.ExpiredMinutes != 100 || res.ActiveMinutes != 50 {
		t.Fatalf("cancel = %+v", res)
	}
	rep, err := l.Balance("alice", "2026-02-10")
	if err != nil {
		t.Fatal(err)
	}
	// B2 refunded 50 back to available; B1's 100 is expired-unused.
	if rep.Available != 100 || rep.ExpiredUnused != 100 || rep.Used != 0 || rep.Total != 200 {
		t.Fatalf("report = %+v", rep)
	}
	// Re-cancel must fail: refund happens exactly once.
	if _, err := l.Cancel("R1", "2026-02-11"); err == nil {
		t.Fatal("second cancel should fail")
	}
	// Refunded B2 minutes participate in later requests.
	res2, err := l.Apply("R2", "alice", "2026-02-12", 100)
	if err != nil || res2.Status != "approved" {
		t.Fatalf("re-apply: %v %+v", err, res2)
	}
	if len(res2.Allocs) != 1 || res2.Allocs[0].BatchID != "B2" || res2.Allocs[0].Minutes != 100 {
		t.Fatalf("allocs = %+v", res2.Allocs)
	}
}
