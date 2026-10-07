package main

import (
	"errors"
	"fmt"
	"sort"
)

// Request status values.
const (
	StatusReserved  = "RESERVED"  // notes reserved, not yet dispensed
	StatusConfirmed = "CONFIRMED" // dispensed, physical stock deducted
	StatusCancelled = "CANCELLED" // reservation released
)

// Request is a withdrawal request with its reserved plan.
type Request struct {
	ID     string `json:"id"`
	Amount int    `json:"amount"`
	Plan   Plan   `json:"plan"`
	Status string `json:"status"`
}

// Store holds physical inventory and all requests.
type Store struct {
	Inventory Plan              `json:"inventory"` // physical notes on hand
	Requests  map[string]*Request `json:"requests"`
}

func NewStore() *Store {
	return &Store{Inventory: Plan{}, Requests: map[string]*Request{}}
}

// SetInventory imports initial cassette configuration. Rejects duplicate
// denominations (checked by caller parsing) and negative counts.
func (s *Store) SetInventory(counts Plan) error {
	for d, n := range counts {
		if d <= 0 {
			return fmt.Errorf("invalid denomination %d", d)
		}
		if n < 0 {
			return fmt.Errorf("negative stock for denomination %d", d)
		}
	}
	s.Inventory = counts
	return nil
}

// AddCash replenishes cassettes; does not touch existing reservations.
func (s *Store) AddCash(counts Plan) error {
	for d, n := range counts {
		if d <= 0 || n < 0 {
			return fmt.Errorf("invalid refill %d:%d", d, n)
		}
		s.Inventory[d] += n
	}
	return nil
}

// Reserved returns total notes reserved by unconfirmed, uncancelled requests.
func (s *Store) Reserved() Plan {
	res := Plan{}
	for _, r := range s.Requests {
		if r.Status != StatusReserved {
			continue
		}
		for d, n := range r.Plan {
			res[d] += n
		}
	}
	return res
}

// Available returns physical minus reserved per denomination.
func (s *Store) Available() Plan {
	res := s.Reserved()
	avail := Plan{}
	for d, n := range s.Inventory {
		avail[d] = n - res[d]
	}
	return avail
}

var (
	ErrInsufficientBalance = errors.New("insufficient total balance")
	ErrCannotCompose       = errors.New("balance sufficient but amount cannot be composed")
	ErrAmountMismatch      = errors.New("request id exists with different amount")
)

// Withdraw creates a request and reserves notes. Idempotent on same id+amount.
func (s *Store) Withdraw(id string, amount int) (*Request, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if r, ok := s.Requests[id]; ok {
		if r.Amount != amount {
			return nil, ErrAmountMismatch
		}
		return r, nil
	}
	avail := s.Available()
	total := 0
	for d, n := range avail {
		total += d * n
	}
	if total < amount {
		return nil, ErrInsufficientBalance
	}
	plan := MakePlan(amount, avail)
	if plan == nil {
		return nil, ErrCannotCompose
	}
	r := &Request{ID: id, Amount: amount, Plan: plan, Status: StatusReserved}
	s.Requests[id] = r
	return r, nil
}

// Confirm dispenses a reserved request, deducting physical stock.
// Re-confirming a confirmed request is a no-op returning the request.
func (s *Store) Confirm(id string) (*Request, error) {
	r, ok := s.Requests[id]
	if !ok {
		return nil, fmt.Errorf("unknown request %q", id)
	}
	switch r.Status {
	case StatusConfirmed:
		return r, nil
	case StatusCancelled:
		return nil, fmt.Errorf("request %q is cancelled", id)
	}
	for d, n := range r.Plan {
		if s.Inventory[d] < n {
			return nil, fmt.Errorf("physical stock inconsistent for %d", d)
		}
	}
	for d, n := range r.Plan {
		s.Inventory[d] -= n
	}
	r.Status = StatusConfirmed
	return r, nil
}

// Cancel releases a reservation. Only reserved (unconfirmed) requests can
// be cancelled; cancelling a cancelled request is a no-op.
func (s *Store) Cancel(id string) (*Request, error) {
	r, ok := s.Requests[id]
	if !ok {
		return nil, fmt.Errorf("unknown request %q", id)
	}
	switch r.Status {
	case StatusConfirmed:
		return nil, fmt.Errorf("request %q already confirmed, cannot cancel", id)
	case StatusCancelled:
		return r, nil
	}
	r.Status = StatusCancelled
	return r, nil
}

// SortedDenoms lists all known denominations descending.
func (s *Store) SortedDenoms() []int {
	return denomsDesc(s.Inventory)
}

// SortedRequests returns requests ordered by id for stable output.
func (s *Store) SortedRequests() []*Request {
	rs := make([]*Request, 0, len(s.Requests))
	for _, r := range s.Requests {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	return rs
}
