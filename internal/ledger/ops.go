package ledger

import "fmt"

// GrantResult acknowledges a grant operation.
type GrantResult struct {
	Status string `json:"status"`
	Batch  string `json:"batch"`
	Total  int    `json:"total"`
}

// Grant adds a new batch. The operation date only advances the ledger
// clock; the batch's own [start, end) range governs usability.
func (l *Ledger) Grant(date Date, id, employee string, start, end Date, minutes int) (GrantResult, error) {
	if minutes <= 0 {
		return GrantResult{}, fmt.Errorf("grant minutes must be positive")
	}
	if !(start < end) {
		return GrantResult{}, fmt.Errorf("batch %s: start %s must be before end %s", id, start, end)
	}
	if _, ok := l.Batches[id]; ok {
		return GrantResult{}, fmt.Errorf("batch %s already exists", id)
	}
	if err := l.advance(date); err != nil {
		return GrantResult{}, err
	}
	l.Batches[id] = &Batch{ID: id, Employee: employee, Start: start, End: end, Total: minutes, Remaining: minutes}
	return GrantResult{Status: "granted", Batch: id, Total: minutes}, nil
}

// RequestResult reports an approved leave request and its frozen
// per-batch deduction breakdown.
type RequestResult struct {
	Status     string      `json:"status"`
	ID         string      `json:"id"`
	Minutes    int         `json:"minutes"`
	Deductions []Deduction `json:"deductions"`
	Replayed   bool        `json:"replayed,omitempty"`
}

// Request approves leave on the given date, deducting only from batches
// valid on that day, earliest-expiry first. Insufficient balance rejects
// the whole request with no partial deduction. A repeated ID with
// identical content replays the original result before any date check;
// identical ID with different content is a conflict.
func (l *Ledger) Request(date Date, id, employee string, minutes int) (RequestResult, error) {
	if prev, ok := l.Requests[id]; ok {
		if prev.Employee == employee && prev.Date == date && prev.Minutes == minutes && !prev.Cancelled {
			return RequestResult{Status: "approved", ID: id, Minutes: prev.Minutes, Deductions: prev.Deductions, Replayed: true}, nil
		}
		return RequestResult{}, fmt.Errorf("request %s conflicts with existing record", id)
	}
	if minutes <= 0 {
		return RequestResult{}, fmt.Errorf("request minutes must be positive")
	}
	if err := l.advance(date); err != nil {
		return RequestResult{}, err
	}
	batches := l.usableBatches(employee, date)
	total := 0
	for _, b := range batches {
		total += b.Remaining
	}
	if total < minutes {
		return RequestResult{}, fmt.Errorf("insufficient balance: need %d, have %d", minutes, total)
	}
	left := minutes
	var ded []Deduction
	for _, b := range batches {
		if left == 0 {
			break
		}
		take := b.Remaining
		if take > left {
			take = left
		}
		b.Remaining -= take
		left -= take
		ded = append(ded, Deduction{Batch: b.ID, Minutes: take})
	}
	l.Requests[id] = &Request{ID: id, Employee: employee, Date: date, Minutes: minutes, Deductions: ded}
	return RequestResult{Status: "approved", ID: id, Minutes: minutes, Deductions: ded}, nil
}

// CancelResult reports how a cancellation was returned per batch.
type CancelResult struct {
	Status  string      `json:"status"`
	ID      string      `json:"id"`
	Returns []Deduction `json:"returns"`
	// Expired lists minutes returned to batches already expired on the
	// cancellation date; they are recorded but never usable again.
	Expired []Deduction `json:"expired,omitempty"`
}

// Cancel returns a request's minutes to the exact original batches, at
// most once. Portions whose batch has expired by the cancellation date
// become expired-returned and stay out of the usable balance.
func (l *Ledger) Cancel(date Date, id string) (CancelResult, error) {
	req, ok := l.Requests[id]
	if !ok {
		return CancelResult{}, fmt.Errorf("request %s not found", id)
	}
	if req.Cancelled {
		return CancelResult{}, fmt.Errorf("request %s already cancelled", id)
	}
	if err := l.advance(date); err != nil {
		return CancelResult{}, err
	}
	res := CancelResult{Status: "cancelled", ID: id}
	for _, d := range req.Deductions {
		b := l.Batches[d.Batch]
		if date < b.End {
			b.Remaining += d.Minutes
			res.Returns = append(res.Returns, d)
		} else {
			b.ExpiredReturned += d.Minutes
			res.Expired = append(res.Expired, d)
		}
	}
	req.Cancelled = true
	return res, nil
}
