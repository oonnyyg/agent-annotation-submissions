package dispense

import (
	"errors"
	"fmt"
	"sort"
)

// Request status values.
const (
	StatusReserved  = "RESERVED"
	StatusConfirmed = "CONFIRMED"
	StatusCancelled = "CANCELLED"
)

var (
	// ErrInsufficientBalance: total available balance is below the amount.
	ErrInsufficientBalance = errors.New("insufficient total balance")
	// ErrNoCombination: balance suffices but no exact combination exists.
	ErrNoCombination = errors.New("amount cannot be composed with available denominations")
	// ErrRequestConflict: request ID reused with a different amount.
	ErrRequestConflict = errors.New("request id already exists with a different amount")
	// ErrRequestNotFound: unknown request id.
	ErrRequestNotFound = errors.New("request not found")
	// ErrAlreadyConfirmed: confirm/cancel on a confirmed request where not allowed.
	ErrAlreadyConfirmed = errors.New("request already confirmed")
)

// Request is a withdrawal request with its reserved plan.
type Request struct {
	ID     string           `json:"id"`
	Amount int64            `json:"amount"`
	Status string           `json:"status"`
	Plan   map[string]int64 `json:"plan"` // denom (string) -> note count
}

// ATM holds physical inventory and reserved requests.
type ATM struct {
	// Physical notes currently in the cassettes (includes reserved ones
	// until they are confirmed/dispensed).
	Physical map[int64]int64 `json:"physical"`
	// Reserved notes per denomination across all RESERVED requests.
	Reserved map[int64]int64            `json:"reserved"`
	Requests map[string]*Request        `json:"requests"`
}

// NewATM creates an empty ATM.
func NewATM() *ATM {
	return &ATM{
		Physical: map[int64]int64{},
		Reserved: map[int64]int64{},
		Requests: map[string]*Request{},
	}
}

// LoadCassette imports cassette inventory. Duplicate denominations and
// negative counts are rejected.
func (a *ATM) LoadCassette(denoms []int64, counts []int64) error {
	if len(denoms) != len(counts) {
		return fmt.Errorf("denominations and counts length mismatch")
	}
	seen := map[int64]bool{}
	for i, d := range denoms {
		if d <= 0 {
			return fmt.Errorf("invalid denomination %d", d)
		}
		if seen[d] {
			return fmt.Errorf("duplicate denomination %d", d)
		}
		seen[d] = true
		if counts[i] < 0 {
			return fmt.Errorf("negative count for denomination %d", d)
		}
	}
	for i, d := range denoms {
		a.Physical[d] += counts[i]
	}
	return nil
}

// Replenish adds notes to an existing denomination. Existing reservations
// are untouched. Negative counts are rejected.
func (a *ATM) Replenish(denom, count int64) error {
	if count < 0 {
		return fmt.Errorf("negative replenish count")
	}
	if _, ok := a.Physical[denom]; !ok {
		return fmt.Errorf("unknown denomination %d", denom)
	}
	a.Physical[denom] += count
	return nil
}

// denomsDesc returns denominations in descending order.
func (a *ATM) denomsDesc() []int64 {
	ds := make([]int64, 0, len(a.Physical))
	for d := range a.Physical {
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] > ds[j] })
	return ds
}

// available returns physical - reserved per denomination (desc order).
func (a *ATM) available() ([]int64, []int64) {
	ds := a.denomsDesc()
	av := make([]int64, len(ds))
	for i, d := range ds {
		av[i] = a.Physical[d] - a.Reserved[d]
	}
	return ds, av
}

// CreateRequest reserves notes for a withdrawal. Idempotent: same id and
// same amount returns the existing request; same id with a different amount
// is rejected. Failed requests leave no reservation behind.
func (a *ATM) CreateRequest(id string, amount int64) (*Request, error) {
	if id == "" {
		return nil, fmt.Errorf("empty request id")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if r, ok := a.Requests[id]; ok {
		if r.Amount != amount {
			return nil, ErrRequestConflict
		}
		return r, nil
	}
	ds, av := a.available()
	var balance int64
	for i, d := range ds {
		balance += d * av[i]
	}
	if balance < amount {
		return nil, ErrInsufficientBalance
	}
	plan := Solve(amount, ds, av)
	if plan == nil {
		return nil, ErrNoCombination
	}
	r := &Request{ID: id, Amount: amount, Status: StatusReserved, Plan: map[string]int64{}}
	for i, d := range ds {
		if plan[i] > 0 {
			r.Plan[fmt.Sprint(d)] = plan[i]
			a.Reserved[d] += plan[i]
		}
	}
	a.Requests[id] = r
	return r, nil
}

// Confirm dispenses a reserved request: physical stock is deducted.
// Confirming an already-confirmed request is a no-op returning the request.
func (a *ATM) Confirm(id string) (*Request, error) {
	r, ok := a.Requests[id]
	if !ok {
		return nil, ErrRequestNotFound
	}
	if r.Status == StatusConfirmed {
		return r, nil
	}
	if r.Status == StatusCancelled {
		return nil, fmt.Errorf("request already cancelled")
	}
	for ds, n := range r.Plan {
		var d int64
		fmt.Sscan(ds, &d)
		a.Physical[d] -= n
		a.Reserved[d] -= n
	}
	r.Status = StatusConfirmed
	return r, nil
}

// Cancel releases the reservation of a non-confirmed request. Cancelling an
// already-cancelled request is a no-op; confirmed requests cannot cancel.
func (a *ATM) Cancel(id string) (*Request, error) {
	r, ok := a.Requests[id]
	if !ok {
		return nil, ErrRequestNotFound
	}
	if r.Status == StatusConfirmed {
		return nil, ErrAlreadyConfirmed
	}
	if r.Status == StatusCancelled {
		return r, nil
	}
	for ds, n := range r.Plan {
		var d int64
		fmt.Sscan(ds, &d)
		a.Reserved[d] -= n
	}
	r.Status = StatusCancelled
	return r, nil
}

// DenomReport is the per-denomination reconciliation view.
type DenomReport struct {
	Denom     int64 `json:"denom"`
	Physical  int64 `json:"physical"`
	Reserved  int64 `json:"reserved"`
	Available int64 `json:"available"`
}

// Report returns per-denomination physical/reserved/available counts.
func (a *ATM) Report() []DenomReport {
	ds := a.denomsDesc()
	out := make([]DenomReport, len(ds))
	for i, d := range ds {
		out[i] = DenomReport{Denom: d, Physical: a.Physical[d], Reserved: a.Reserved[d], Available: a.Physical[d] - a.Reserved[d]}
	}
	return out
}
