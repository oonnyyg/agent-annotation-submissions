package ledger

import (
	"errors"
	"fmt"
	"sort"
)

// Ledger mutates a State in memory; persistence is handled by the caller.
type Ledger struct {
	st *State
}

func New(st *State) *Ledger { return &Ledger{st: st} }

// ErrDateRegression is returned when an operation date moves backwards.
var ErrDateRegression = errors.New("operation date must be non-decreasing")

// advance enforces the monotonic operation-date rule.
func (l *Ledger) advance(d Date) error {
	if l.st.LastDate != "" && d < l.st.LastDate {
		return fmt.Errorf("%w: %s before %s", ErrDateRegression, d, l.st.LastDate)
	}
	l.st.LastDate = d
	return nil
}

// Grant adds a new quota batch. Batch IDs are unique per ledger.
func (l *Ledger) Grant(b Batch, opDate Date) error {
	if b.ID == "" || b.Employee == "" {
		return errors.New("grant requires batch id and employee")
	}
	if b.Minutes <= 0 {
		return errors.New("grant minutes must be positive")
	}
	if !(b.Start < b.End) {
		return fmt.Errorf("batch %s: start %s must be before end %s", b.ID, b.Start, b.End)
	}
	if _, ok := l.st.Batches[b.ID]; ok {
		return fmt.Errorf("batch %s already exists", b.ID)
	}
	if err := l.advance(opDate); err != nil {
		return err
	}
	cp := b
	l.st.Batches[b.ID] = &cp
	return nil
}

// RequestResult is the outcome of an apply operation.
type RequestResult struct {
	ID     string       `json:"id"`
	Status string       `json:"status"` // "approved", "duplicate", "rejected"
	Allocs []Allocation `json:"allocs,omitempty"`
	Reason string       `json:"reason,omitempty"`
}

// Apply approves a leave request for the given operation date.
// Re-submitting the same ID with identical content returns the stored
// result without touching the date cursor or deducting again.
func (l *Ledger) Apply(id, employee string, date Date, minutes int) (RequestResult, error) {
	if id == "" || employee == "" {
		return RequestResult{}, errors.New("request requires id and employee")
	}
	if minutes <= 0 {
		return RequestResult{}, errors.New("request minutes must be positive")
	}
	if prev, ok := l.st.Requests[id]; ok {
		if prev.Employee == employee && prev.Date == date && prev.Minutes == minutes && !prev.Canceled {
			return RequestResult{ID: id, Status: "duplicate", Allocs: prev.Allocs}, nil
		}
		return RequestResult{}, fmt.Errorf("request %s conflicts with existing record", id)
	}
	if err := l.advance(date); err != nil {
		return RequestResult{}, err
	}

	batches := l.effectiveBatches(employee, date)
	total := 0
	for _, b := range batches {
		total += b.Remaining()
	}
	if total < minutes {
		return RequestResult{
			ID:     id,
			Status: "rejected",
			Reason: fmt.Sprintf("insufficient balance: need %d, have %d", minutes, total),
		}, nil
	}

	// Earliest expiry first, then batch ID lexicographic order.
	left := minutes
	var allocs []Allocation
	for _, b := range batches {
		if left == 0 {
			break
		}
		take := b.Remaining()
		if take > left {
			take = left
		}
		if take <= 0 {
			continue
		}
		b.Used += take
		left -= take
		allocs = append(allocs, Allocation{BatchID: b.ID, Minutes: take})
	}
	l.st.Requests[id] = &Request{
		ID: id, Employee: employee, Date: date, Minutes: minutes, Allocs: allocs,
	}
	return RequestResult{ID: id, Status: "approved", Allocs: allocs}, nil
}

// effectiveBatches returns the employee's batches usable on date d,
// ordered by (End, ID).
func (l *Ledger) effectiveBatches(employee string, d Date) []*Batch {
	var out []*Batch
	for _, b := range l.st.Batches {
		if b.Employee == employee && b.Effective(d) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].End != out[j].End {
			return out[i].End < out[j].End
		}
		return out[i].ID < out[j].ID
	})
	return out
}
