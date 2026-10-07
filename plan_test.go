package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestMakePlanNonGreedy(t *testing.T) {
	// Greedy would take 4+1+1 (3 notes); optimal is 3+3.
	avail := Plan{4: 1, 3: 2, 1: 5}
	got := MakePlan(6, avail)
	want := Plan{3: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMakePlanTieBreakPrefersLarger(t *testing.T) {
	avail := Plan{10: 2, 5: 2}
	got := MakePlan(10, avail)
	want := Plan{10: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMakePlanRespectsCounts(t *testing.T) {
	// Only one 10; 20 must be 10+5+5.
	avail := Plan{10: 1, 5: 3}
	got := MakePlan(20, avail)
	want := Plan{10: 1, 5: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMakePlanImpossible(t *testing.T) {
	if got := MakePlan(7, Plan{4: 2, 2: 1}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := MakePlan(100, Plan{1: 3}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestWithdrawDistinguishesFailures(t *testing.T) {
	s := NewStore()
	if err := s.SetInventory(Plan{20: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Withdraw("a", 30); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
	if _, err := s.Withdraw("b", 15); !errors.Is(err, ErrCannotCompose) {
		t.Fatalf("want ErrCannotCompose, got %v", err)
	}
	// Failed attempts must not leave reservations behind.
	if len(s.Requests) != 0 {
		t.Fatalf("failed withdraws leaked requests: %v", s.Requests)
	}
	if got := s.Available()[20]; got != 1 {
		t.Fatalf("available changed after failures: %d", got)
	}
}

func TestInterleavedRequestsShareStock(t *testing.T) {
	s := NewStore()
	if err := s.SetInventory(Plan{10: 2, 5: 2}); err != nil {
		t.Fatal(err)
	}
	r1, err := s.Withdraw("r1", 20) // takes 10+10
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1.Plan, Plan{10: 2}) {
		t.Fatalf("r1 plan %v", r1.Plan)
	}
	// Only 5+5 left; 10 must now use fives.
	r2, err := s.Withdraw("r2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r2.Plan, Plan{5: 2}) {
		t.Fatalf("r2 plan %v", r2.Plan)
	}
	// Nothing left.
	if _, err := s.Withdraw("r3", 5); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("want insufficient, got %v", err)
	}
	// Cancel r1 frees its notes; r3 can now be served (with 10s only).
	if _, err := s.Cancel("r1"); err != nil {
		t.Fatal(err)
	}
	r3, err := s.Withdraw("r3", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r3.Plan, Plan{10: 1}) {
		t.Fatalf("r3 plan %v", r3.Plan)
	}
}

func TestWithdrawIdempotency(t *testing.T) {
	s := NewStore()
	s.SetInventory(Plan{10: 5})
	r1, err := s.Withdraw("x", 20)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Withdraw("x", 20)
	if err != nil {
		t.Fatal(err)
	}
	if r1 != r2 {
		t.Fatal("same id+amount should return original request")
	}
	if _, err := s.Withdraw("x", 30); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("want ErrAmountMismatch, got %v", err)
	}
}

func TestConfirmAndCancelSemantics(t *testing.T) {
	s := NewStore()
	s.SetInventory(Plan{10: 3})
	if _, err := s.Withdraw("c1", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Confirm("c1"); err != nil {
		t.Fatal(err)
	}
	if got := s.Inventory[10]; got != 2 {
		t.Fatalf("inventory after confirm: %d", got)
	}
	// Double confirm must not deduct again.
	if _, err := s.Confirm("c1"); err != nil {
		t.Fatal(err)
	}
	if got := s.Inventory[10]; got != 2 {
		t.Fatalf("double confirm deducted: %d", got)
	}
	// Confirmed request cannot be cancelled.
	if _, err := s.Cancel("c1"); err == nil {
		t.Fatal("cancel of confirmed request should fail")
	}
	// Cancel then confirm must fail; double cancel is a no-op.
	if _, err := s.Withdraw("c2", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel("c2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel("c2"); err != nil {
		t.Fatal(err)
	}
	if got := s.Available()[10]; got != 2 {
		t.Fatalf("double cancel should not duplicate release: %d", got)
	}
	if _, err := s.Confirm("c2"); err == nil {
		t.Fatal("confirm of cancelled request should fail")
	}
}

func TestReconcile(t *testing.T) {
	s := NewStore()
	s.SetInventory(Plan{50: 2, 20: 3, 10: 4})
	s.Withdraw("a", 60)
	s.Withdraw("b", 30)
	s.Confirm("a")
	s.Withdraw("c", 20)
	s.Cancel("b")
	avail := s.Available()
	res := s.Reserved()
	for _, d := range s.SortedDenoms() {
		if s.Inventory[d]-res[d] != avail[d] {
			t.Fatalf("reconcile failed for denom %d", d)
		}
		if avail[d] < 0 {
			t.Fatalf("negative availability for denom %d", d)
		}
	}
}
