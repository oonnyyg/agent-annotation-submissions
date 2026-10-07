package dispense

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func mustLoad(t *testing.T, a *ATM, denoms, counts []int64) {
	t.Helper()
	if err := a.LoadCassette(denoms, counts); err != nil {
		t.Fatalf("LoadCassette: %v", err)
	}
}

// Greedy would pick 10 then fail on remainder 2; exact solver must find 6+6.
func TestSolveNonGreedy(t *testing.T) {
	got := Solve(12, []int64{10, 6}, []int64{5, 5})
	want := []int64{0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Solve(12) = %v, want %v", got, want)
	}
}

// Min note count wins over greedy larger-denom usage: 10+1+1 (3) vs 6+6 (2).
func TestSolveMinNotes(t *testing.T) {
	got := Solve(12, []int64{10, 6, 1}, []int64{5, 5, 5})
	want := []int64{0, 2, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Solve(12) = %v, want %v", got, want)
	}
}

// Equal note counts: prefer larger denominations (6+4 over 5+5).
func TestSolveTieBreakPrefersLarger(t *testing.T) {
	got := Solve(10, []int64{6, 5, 4}, []int64{5, 5, 5})
	want := []int64{1, 0, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Solve(10) = %v, want %v", got, want)
	}
}

func TestSolveRespectsAvailability(t *testing.T) {
	// Only one 6 available, so 6+6 is impossible; must use 6+4? no, 10 with
	// denoms [6,4] and one 6 -> 6+4 works.
	got := Solve(10, []int64{6, 4}, []int64{1, 5})
	want := []int64{1, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Solve(10) = %v, want %v", got, want)
	}
	if Solve(12, []int64{6, 4}, []int64{1, 1}) != nil {
		t.Fatal("expected nil when combination impossible with limited stock")
	}
}

func TestLoadRejectsDuplicatesAndNegatives(t *testing.T) {
	a := NewATM()
	if err := a.LoadCassette([]int64{100, 100}, []int64{1, 1}); err == nil {
		t.Fatal("expected duplicate denomination error")
	}
	if err := a.LoadCassette([]int64{100}, []int64{-1}); err == nil {
		t.Fatal("expected negative count error")
	}
}

func TestInsufficientBalanceVsNoCombination(t *testing.T) {
	a := NewATM()
	mustLoad(t, a, []int64{10}, []int64{1})
	if _, err := a.CreateRequest("r1", 20); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
	// Balance 14 >= 6, but 6 cannot be composed from {10,4}? 6 cannot.
	b := NewATM()
	mustLoad(t, b, []int64{10, 4}, []int64{1, 1})
	if _, err := b.CreateRequest("r2", 6); !errors.Is(err, ErrNoCombination) {
		t.Fatalf("want ErrNoCombination, got %v", err)
	}
	// Failed requests must not leave reservations.
	for _, row := range b.Report() {
		if row.Reserved != 0 {
			t.Fatalf("reservation leaked after failed request: %+v", row)
		}
	}
}

func TestInterleavedRequestsShareStock(t *testing.T) {
	a := NewATM()
	mustLoad(t, a, []int64{100, 50}, []int64{1, 2})
	r1, err := a.CreateRequest("a", 100)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Plan["100"] != 1 {
		t.Fatalf("r1 plan = %v", r1.Plan)
	}
	// The single 100-note is reserved; r2 must use 50s.
	r2, err := a.CreateRequest("b", 100)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Plan["50"] != 2 {
		t.Fatalf("r2 plan = %v", r2.Plan)
	}
	// No stock left for a third 100 withdrawal.
	if _, err := a.CreateRequest("c", 100); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
	// Cancel r2 frees the 50s; r3 can then reserve them.
	if _, err := a.Cancel("b"); err != nil {
		t.Fatal(err)
	}
	r3, err := a.CreateRequest("c", 100)
	if err != nil {
		t.Fatal(err)
	}
	if r3.Plan["50"] != 2 {
		t.Fatalf("r3 plan = %v", r3.Plan)
	}
}

func TestIdempotentCreateAndConflict(t *testing.T) {
	a := NewATM()
	mustLoad(t, a, []int64{100}, []int64{2})
	r1, err := a.CreateRequest("x", 100)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.CreateRequest("x", 100)
	if err != nil {
		t.Fatal(err)
	}
	if r1 != r2 {
		t.Fatal("same id+amount must return the original request")
	}
	if a.Reserved[100] != 1 {
		t.Fatalf("reserved = %d, want 1 (no double reservation)", a.Reserved[100])
	}
	if _, err := a.CreateRequest("x", 200); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("want ErrRequestConflict, got %v", err)
	}
}

func TestConfirmAndCancelIdempotency(t *testing.T) {
	a := NewATM()
	mustLoad(t, a, []int64{100}, []int64{2})
	if _, err := a.CreateRequest("c1", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateRequest("c2", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Confirm("c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Confirm("c1"); err != nil { // repeat confirm: no double deduct
		t.Fatal(err)
	}
	if a.Physical[100] != 1 || a.Reserved[100] != 1 {
		t.Fatalf("physical=%d reserved=%d, want 1/1", a.Physical[100], a.Reserved[100])
	}
	// Confirmed request cannot be cancelled.
	if _, err := a.Cancel("c1"); !errors.Is(err, ErrAlreadyConfirmed) {
		t.Fatalf("want ErrAlreadyConfirmed, got %v", err)
	}
	// Cancel releases once; repeat cancel is a no-op.
	if _, err := a.Cancel("c2"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Cancel("c2"); err != nil {
		t.Fatal(err)
	}
	if a.Physical[100] != 1 || a.Reserved[100] != 0 {
		t.Fatalf("physical=%d reserved=%d, want 1/0", a.Physical[100], a.Reserved[100])
	}
}

func TestReplenishKeepsExistingReservations(t *testing.T) {
	a := NewATM()
	mustLoad(t, a, []int64{100, 50}, []int64{1, 0})
	r, err := a.CreateRequest("r", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Replenish(50, 4); err != nil {
		t.Fatal(err)
	}
	// Existing plan unchanged even though 50s are now available.
	if r.Plan["100"] != 1 || len(r.Plan) != 1 {
		t.Fatalf("plan changed after replenish: %v", r.Plan)
	}
	if err := a.Replenish(50, -1); err == nil {
		t.Fatal("expected negative replenish error")
	}
	if err := a.Replenish(25, 1); err == nil {
		t.Fatal("expected unknown denomination error")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	a := NewATM()
	mustLoad(t, a, []int64{100, 50}, []int64{2, 3})
	if _, err := a.CreateRequest("keep", 150); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateRequest("done", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Confirm("done"); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(path); err != nil {
		t.Fatal(err)
	}
	b, err := LoadATM(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Report(), b.Report()) {
		t.Fatalf("report mismatch after reload:\n%v\n%v", a.Report(), b.Report())
	}
	// Unfinished request is still reserved and can be confirmed after restart.
	r, err := b.Confirm("keep")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusConfirmed {
		t.Fatalf("status = %s", r.Status)
	}
	if b.Physical[100] != 0 || b.Physical[50] != 2 {
		t.Fatalf("physical after confirm = %v", b.Physical)
	}
}
