package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseCounts(t *testing.T) {
	p, err := ParseCounts("100:5, 50:10")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, Plan{100: 5, 50: 10}) {
		t.Fatalf("got %v", p)
	}
	if _, err := ParseCounts("100:5,100:3"); err == nil {
		t.Fatal("duplicate denomination accepted")
	}
	if _, err := ParseCounts("100:-1"); err == nil {
		t.Fatal("negative count accepted")
	}
	if _, err := ParseCounts("0:5"); err == nil {
		t.Fatal("zero denomination accepted")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewStore()
	s.SetInventory(Plan{50: 2, 10: 5})
	if _, err := s.Withdraw("keep", 60); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Withdraw("done", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Confirm("done"); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}

	// Simulate restart.
	s2, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s2.Inventory, s.Inventory) {
		t.Fatalf("inventory mismatch: %v vs %v", s2.Inventory, s.Inventory)
	}
	r := s2.Requests["keep"]
	if r == nil || r.Status != StatusReserved || !reflect.DeepEqual(r.Plan, Plan{50: 1, 10: 1}) {
		t.Fatalf("reserved request not restored: %+v", r)
	}
	// Unfinished request can still be confirmed after restart.
	if _, err := s2.Confirm("keep"); err != nil {
		t.Fatal(err)
	}
	if got := s2.Inventory[50]; got != 1 {
		t.Fatalf("inventory after confirm: %d", got)
	}
	if got := s2.Inventory[10]; got != 3 {
		t.Fatalf("inventory after confirm: %d", got)
	}
}

func TestAddCashKeepsReservations(t *testing.T) {
	s := NewStore()
	s.SetInventory(Plan{10: 1})
	r, err := s.Withdraw("w", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddCash(Plan{10: 4, 20: 1}); err != nil {
		t.Fatal(err)
	}
	// Existing reservation plan unchanged.
	if !reflect.DeepEqual(r.Plan, Plan{10: 1}) {
		t.Fatalf("plan changed after refill: %v", r.Plan)
	}
	if got := s.Available()[10]; got != 4 {
		t.Fatalf("available after refill: %d", got)
	}
	if err := s.AddCash(Plan{10: -1}); err == nil {
		t.Fatal("negative refill accepted")
	}
}
