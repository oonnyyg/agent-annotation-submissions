package hall

import (
	"path/filepath"
	"reflect"
	"testing"
)

func mustHall(t *testing.T, rows, cols int) *Hall {
	t.Helper()
	h, err := NewHall(rows, cols)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func seats(s ...Seat) []Seat { return s }

// Greedy per-order allocation would give the first order the best run and
// push the second order far away; the joint optimum must do better overall.
func TestBatchBeatsGreedy(t *testing.T) {
	h := mustHall(t, 1, 5)
	// Greedy per-order: A takes its best run (2,3) at cost 1, leaving B
	// only (4,5) at cost 7, total 8. Joint optimum: A at (3,4) cost 1,
	// B at (1,2) cost 1, total 2.
	orders, err := h.Book([]OrderRequest{
		{ID: "A", Size: 2, Pref: Seat{1, 3}},
		{ID: "B", Size: 2, Pref: Seat{1, 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	total := orders[0].Cost + orders[1].Cost
	if total != 2 {
		t.Fatalf("total cost = %d, want 2 (orders: %+v)", total, orders)
	}
	if !reflect.DeepEqual(orders[0].Seats, seats(Seat{1, 3}, Seat{1, 4})) {
		t.Fatalf("A seats = %v", orders[0].Seats)
	}
	if !reflect.DeepEqual(orders[1].Seats, seats(Seat{1, 1}, Seat{1, 2})) {
		t.Fatalf("B seats = %v", orders[1].Seats)
	}
}

// Equal-cost solutions must pick the lexicographically smallest concatenated
// seat sequence (orders sorted by ID).
func TestTieBreakLexicographic(t *testing.T) {
	h := mustHall(t, 1, 5)
	// Single order, size 2, pref at (1,3): runs (2,3) and (3,4) both cost 1.
	// Lexicographically smallest is (1,2)? No: (1,2) costs 2+1=3. Min cost
	// is 1: (2,3) or (3,4). Smallest sequence is (2,3).
	orders, err := h.Book([]OrderRequest{{ID: "A", Size: 2, Pref: Seat{1, 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orders[0].Seats, seats(Seat{1, 2}, Seat{1, 3})) {
		t.Fatalf("seats = %v, want [(1,2) (1,3)]", orders[0].Seats)
	}
}

// Batch with an infeasible order must be rejected as a whole.
func TestBatchAtomicReject(t *testing.T) {
	h := mustHall(t, 1, 4)
	_, err := h.Book([]OrderRequest{
		{ID: "A", Size: 2, Pref: Seat{1, 1}},
		{ID: "B", Size: 3, Pref: Seat{1, 3}},
	})
	if err == nil {
		t.Fatal("expected batch rejection")
	}
	if len(h.Orders) != 0 {
		t.Fatalf("partial allocation left behind: %+v", h.Orders)
	}
}

// Duplicate IDs inside a batch or against existing orders reject everything.
func TestDuplicateIDReject(t *testing.T) {
	h := mustHall(t, 1, 4)
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 1, Pref: Seat{1, 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 1, Pref: Seat{1, 2}}}); err == nil {
		t.Fatal("expected duplicate id rejection")
	}
	if _, err := h.Book([]OrderRequest{
		{ID: "B", Size: 1, Pref: Seat{1, 2}},
		{ID: "B", Size: 1, Pref: Seat{1, 3}},
	}); err == nil {
		t.Fatal("expected in-batch duplicate rejection")
	}
	if len(h.Orders) != 1 {
		t.Fatalf("orders = %+v", h.Orders)
	}
}

// Disabled seats split runs and must not be crossed.
func TestDisabledSplitsRun(t *testing.T) {
	h := mustHall(t, 1, 5)
	if err := h.SetDisabled(seats(Seat{1, 3}), true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 3, Pref: Seat{1, 3}}}); err == nil {
		t.Fatal("expected no run of 3 across disabled seat")
	}
	orders, err := h.Book([]OrderRequest{{ID: "A", Size: 2, Pref: Seat{1, 3}}})
	if err != nil {
		t.Fatal(err)
	}
	// Min cost run of 2 not crossing (1,3): (1,2) cost 1, or (4,5) cost 2.
	if !reflect.DeepEqual(orders[0].Seats, seats(Seat{1, 1}, Seat{1, 2})) {
		t.Fatalf("seats = %v", orders[0].Seats)
	}
}

// A failed change must keep the original seats, size and preference.
func TestChangeFailureKeepsOriginal(t *testing.T) {
	h := mustHall(t, 1, 4)
	orders, err := h.Book([]OrderRequest{
		{ID: "A", Size: 2, Pref: Seat{1, 1}},
		{ID: "B", Size: 2, Pref: Seat{1, 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := *orders[0]
	// A wants 3 seats but only 2 remain free (B holds the other two).
	if _, err := h.Change("A", 3, Seat{1, 2}); err == nil {
		t.Fatal("expected change to fail")
	}
	got := h.Orders["A"]
	if got.Size != before.Size || got.Pref != before.Pref ||
		!reflect.DeepEqual(got.Seats, before.Seats) || got.Cost != before.Cost {
		t.Fatalf("order mutated on failed change: %+v vs %+v", got, before)
	}
}

// A successful change may reuse its own seats.
func TestChangeReuseOwnSeats(t *testing.T) {
	h := mustHall(t, 1, 5)
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 2, Pref: Seat{1, 1}}}); err != nil {
		t.Fatal(err)
	}
	o, err := h.Change("A", 3, Seat{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(o.Seats, seats(Seat{1, 1}, Seat{1, 2}, Seat{1, 3})) {
		t.Fatalf("seats = %v", o.Seats)
	}
}

// Released seats must be allocatable again and not reported as occupied.
func TestReleaseFreesSeats(t *testing.T) {
	h := mustHall(t, 1, 4)
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 4, Pref: Seat{1, 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Release("A"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Book([]OrderRequest{{ID: "B", Size: 4, Pref: Seat{1, 1}}}); err != nil {
		t.Fatalf("released seats not reusable: %v", err)
	}
	if _, err := h.Release("A"); err == nil {
		t.Fatal("expected release of unknown order to fail")
	}
}

// State must survive a reload from disk.
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	h := mustHall(t, 2, 3)
	if err := h.SetDisabled(seats(Seat{2, 2}), true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Book([]OrderRequest{{ID: "A", Size: 2, Pref: Seat{1, 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := h.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("no state loaded")
	}
	if !loaded.Disabled["2,2"] {
		t.Fatal("disabled seat lost")
	}
	o := loaded.Orders["A"]
	if o == nil || !reflect.DeepEqual(o.Seats, seats(Seat{1, 1}, Seat{1, 2})) {
		t.Fatalf("order lost: %+v", o)
	}
	if _, err := Load(filepath.Join(dir, "nonexistent")); err != nil {
		t.Fatal(err)
	}
}
