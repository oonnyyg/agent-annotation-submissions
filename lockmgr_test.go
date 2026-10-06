package main

import "testing"

func mustBegin(t *testing.T, m *LockManager) int {
	t.Helper()
	return m.Begin()
}

func mustLock(t *testing.T, m *LockManager, tx int, res string, mode LockMode) *Grant {
	t.Helper()
	g, err := m.RequestLock(tx, res, mode)
	if err != nil {
		t.Fatalf("lock tx=%d res=%s mode=%s: %v", tx, res, mode, err)
	}
	return g
}

func TestSharedCoexistAndExclusiveConflicts(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	if g := mustLock(t, m, t1, "a", Shared); g == nil {
		t.Fatal("t1 S(a) should be granted")
	}
	if g := mustLock(t, m, t2, "a", Shared); g == nil {
		t.Fatal("t2 S(a) should coexist")
	}
	if g := mustLock(t, m, t1, "a", Exclusive); g != nil {
		t.Fatal("t1 X(a) upgrade must wait behind t2 S")
	}
	if st := m.txState[t1]; st != TxWaiting {
		t.Fatalf("t1 should be waiting, got %s", st)
	}
}

func TestUpgradeDeadlockVictimIsYoungest(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Shared)
	mustLock(t, m, t2, "a", Shared)
	mustLock(t, m, t1, "a", Exclusive) // waits
	mustLock(t, m, t2, "a", Exclusive) // waits -> cycle
	reports := m.ResolveDeadlocks()
	if len(reports) != 1 {
		t.Fatalf("expected 1 deadlock report, got %d", len(reports))
	}
	rep := reports[0]
	if rep.Victim != t2 {
		t.Fatalf("victim should be youngest tx %d, got %d", t2, rep.Victim)
	}
	if len(rep.EdgesBefore) == 0 {
		t.Fatal("report must keep wait-for edges before resolution")
	}
	for _, e := range rep.EdgesBefore {
		if e.From == e.To {
			t.Fatalf("self edge %v must not appear", e)
		}
	}
	// t1's upgrade is granted after the victim aborts.
	found := false
	for _, g := range rep.Granted {
		if g.Tx == t1 && g.Resource == "a" && g.Mode == string(Exclusive) {
			found = true
		}
	}
	if !found {
		t.Fatal("t1 upgrade should be granted after victim abort")
	}
	snap := m.Snapshot()
	if _, ok := snap.Transactions["2"]; ok {
		t.Fatal("finished victim must not appear in snapshot")
	}
	if len(snap.WaitFor) != 0 {
		t.Fatalf("no wait edges should remain, got %v", snap.WaitFor)
	}
}

func TestQueueFairnessNoJumping(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	t3 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Exclusive)
	mustLock(t, m, t2, "a", Exclusive) // waits (head)
	mustLock(t, m, t3, "a", Shared)    // waits behind t2, cannot jump
	granted, err := m.Finish(t1)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 1 || granted[0].Tx != t2 {
		t.Fatalf("only t2 should be granted, got %+v", granted)
	}
	if st := m.txState[t3]; st != TxWaiting {
		t.Fatalf("t3 must keep waiting behind t2's X, got %s", st)
	}
	granted, _ = m.Finish(t2)
	if len(granted) != 1 || granted[0].Tx != t3 {
		t.Fatalf("t3 should be granted after t2 finishes, got %+v", granted)
	}
}

func TestConsecutiveSharedGrantedTogether(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	t3 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Exclusive)
	mustLock(t, m, t2, "a", Shared)
	mustLock(t, m, t3, "a", Shared)
	granted, _ := m.Finish(t1)
	if len(granted) != 2 {
		t.Fatalf("both shared waiters should be granted together, got %+v", granted)
	}
}

func TestMultiResourceWake(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	t3 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Exclusive)
	mustLock(t, m, t1, "b", Exclusive)
	mustLock(t, m, t2, "a", Exclusive) // waits on a
	mustLock(t, m, t3, "b", Exclusive) // waits on b
	granted, _ := m.Finish(t1)
	got := map[int]bool{}
	for _, g := range granted {
		got[g.Tx] = true
	}
	if !got[t2] || !got[t3] {
		t.Fatalf("both t2 and t3 should wake, got %+v", granted)
	}
}

func TestRepeatAndWeakerRequestsSucceed(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Exclusive)
	g := mustLock(t, m, t1, "a", Exclusive)
	if g == nil || g.Reason != "already-held" {
		t.Fatalf("repeat X should succeed, got %+v", g)
	}
	g = mustLock(t, m, t1, "a", Shared)
	if g == nil || g.Reason != "already-held" {
		t.Fatalf("weaker S on held X should succeed, got %+v", g)
	}
}

func TestWaitingTxCanOnlyQueryOrRollback(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	t2 := mustBegin(t, m)
	mustLock(t, m, t1, "a", Exclusive)
	mustLock(t, m, t2, "a", Exclusive) // waits
	if _, err := m.RequestLock(t2, "b", Shared); err == nil {
		t.Fatal("waiting tx must not request new locks")
	}
	if _, err := m.Finish(t2); err != nil {
		t.Fatalf("waiting tx must be allowed to roll back: %v", err)
	}
	if len(m.resources["a"].queue) != 0 {
		t.Fatal("rollback must remove queued request")
	}
}

func TestIllegalOpsLeaveNoTrace(t *testing.T) {
	m := NewLockManager()
	if _, err := m.RequestLock(99, "a", Exclusive); err == nil {
		t.Fatal("lock for unknown tx must fail")
	}
	if _, ok := m.resources["a"]; ok && len(m.resources["a"].queue) != 0 {
		t.Fatal("failed request must not enqueue")
	}
	t1 := mustBegin(t, m)
	if _, err := m.RequestLock(t1, "a", LockMode("Q")); err == nil {
		t.Fatal("invalid mode must fail")
	}
	r := m.resources["a"]
	if r != nil && (len(r.holders) != 0 || len(r.queue) != 0) {
		t.Fatal("invalid mode must leave no lock or queue record")
	}
	if _, err := m.Finish(99); err == nil {
		t.Fatal("finish of unknown tx must fail")
	}
}

func TestTxIDsNotReused(t *testing.T) {
	m := NewLockManager()
	t1 := mustBegin(t, m)
	if _, err := m.Finish(t1); err != nil {
		t.Fatal(err)
	}
	t2 := mustBegin(t, m)
	if t2 <= t1 {
		t.Fatalf("ids must increase, got %d after %d", t2, t1)
	}
	if _, err := m.Finish(t1); err == nil {
		t.Fatal("finished tx must not be reusable")
	}
}
