package ledger

import "testing"

func TestDuplicateRequest(t *testing.T) {
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-03-01", 300, "2026-01-01")
	first, err := l.Apply("R1", "alice", "2026-01-10", 100)
	if err != nil || first.Status != "approved" {
		t.Fatalf("first apply: %v %+v", err, first)
	}
	// Identical resubmission: returns the original result, does not
	// advance the date cursor, does not deduct again.
	l.st.LastDate = "2026-01-20"
	dup, err := l.Apply("R1", "alice", "2026-01-10", 100)
	if err != nil || dup.Status != "duplicate" {
		t.Fatalf("duplicate: %v %+v", err, dup)
	}
	if l.st.LastDate != "2026-01-20" {
		t.Fatalf("date cursor moved to %s", l.st.LastDate)
	}
	if got := l.st.Batches["B1"].Used; got != 100 {
		t.Fatalf("double deduction: used=%d", got)
	}
	// Same ID, different content: conflict.
	if _, err := l.Apply("R1", "alice", "2026-01-11", 100); err == nil {
		t.Fatal("conflicting resubmission should fail")
	}
}

func TestDateRegression(t *testing.T) {
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-03-01", 300, "2026-01-05")
	if _, err := l.Apply("R1", "alice", "2026-01-04", 10); err == nil {
		t.Fatal("regressing request date should fail")
	}
	if _, err := l.Balance("alice", "2026-01-01"); err == nil {
		t.Fatal("regressing query date should fail")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-03-01", 300, "2026-01-01")
	if _, err := l.Apply("R1", "alice", "2026-01-10", 120); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, l.st); err != nil {
		t.Fatal(err)
	}
	st, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	l2 := New(st)
	rep, err := l2.Balance("alice", "2026-01-11")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Available != 180 || rep.Used != 120 || rep.Total != 300 {
		t.Fatalf("report after reload = %+v", rep)
	}
	// Duplicate detection survives the restart.
	dup, err := l2.Apply("R1", "alice", "2026-01-10", 120)
	if err != nil || dup.Status != "duplicate" {
		t.Fatalf("duplicate after reload: %v %+v", err, dup)
	}
}

func TestConservationAcrossLifecycle(t *testing.T) {
	l := New(NewState())
	grant(t, l, "B1", "2026-01-01", "2026-02-01", 100, "2026-01-01")
	grant(t, l, "B2", "2026-02-01", "2026-03-01", 50, "2026-01-01")
	if _, err := l.Apply("R1", "alice", "2026-01-15", 60); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Cancel("R1", "2026-02-05"); err != nil {
		t.Fatal(err)
	}
	rep, err := l.Balance("alice", "2026-02-05")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Available+rep.Used+rep.Pending+rep.ExpiredUnused != rep.Total {
		t.Fatalf("conservation broken: %+v", rep)
	}
	if rep.Total != 150 || rep.ExpiredUnused != 100 || rep.Available != 50 || rep.Pending != 0 {
		t.Fatalf("report = %+v", rep)
	}
}
