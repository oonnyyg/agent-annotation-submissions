package ledger

import (
	"errors"
	"fmt"
	"sort"
)

// CancelResult describes how a cancellation was refunded.
type CancelResult struct {
	ID             string       `json:"id"`
	Status         string       `json:"status"` // "canceled"
	Refunded       []Allocation `json:"refunded"`
	ExpiredMinutes int          `json:"expiredMinutes"`
	ActiveMinutes  int          `json:"activeMinutes"`
}

// Cancel refunds a request exactly along its frozen allocation plan.
// Portions whose batch had already expired by the operation date are
// recorded as expired returns and never become usable again.
func (l *Ledger) Cancel(id string, opDate Date) (CancelResult, error) {
	req, ok := l.st.Requests[id]
	if !ok {
		return CancelResult{}, fmt.Errorf("request %s not found", id)
	}
	if req.Canceled {
		return CancelResult{}, fmt.Errorf("request %s already canceled", id)
	}
	if err := l.advance(opDate); err != nil {
		return CancelResult{}, err
	}
	res := CancelResult{ID: id, Status: "canceled"}
	for _, a := range req.Allocs {
		b := l.st.Batches[a.BatchID]
		if b == nil {
			return CancelResult{}, fmt.Errorf("batch %s missing for request %s", a.BatchID, id)
		}
		b.Used -= a.Minutes
		if opDate >= b.End {
			b.ExpiredReturned += a.Minutes
			res.ExpiredMinutes += a.Minutes
		} else {
			res.ActiveMinutes += a.Minutes
		}
		res.Refunded = append(res.Refunded, a)
	}
	req.Canceled = true
	return res, nil
}

// Report is a per-employee breakdown of minutes at a query date.
type Report struct {
	Employee string `json:"employee"`
	Date     Date   `json:"date"`
	// Available is usable remaining minutes in effective batches.
	Available int `json:"available"`
	// Used is minutes frozen by approved (not canceled) requests.
	Used int `json:"used"`
	// Pending is remaining minutes in batches not yet effective.
	Pending int `json:"pending"`
	// ExpiredUnused is remaining plus expired-returned minutes in
	// batches whose validity has ended.
	ExpiredUnused int `json:"expiredUnused"`
	// Total is the sum of all granted minutes; the four buckets above
	// always add up to it (conservation check).
	Total int `json:"total"`
}

// Balance reports an employee's minutes as of the query date.
// Queries never read the machine clock; the caller supplies the date.
func (l *Ledger) Balance(employee string, d Date) (Report, error) {
	if err := l.advance(d); err != nil {
		return Report{}, err
	}
	r := Report{Employee: employee, Date: d}
	for _, b := range l.st.Batches {
		if b.Employee != employee {
			continue
		}
		r.Total += b.Minutes
		r.Used += b.Used
		switch {
		case d < b.Start:
			r.Pending += b.Remaining()
		case d >= b.End:
			r.ExpiredUnused += b.Remaining() + b.ExpiredReturned
		default:
			r.Available += b.Remaining()
		}
	}
	if r.Available+r.Used+r.Pending+r.ExpiredUnused != r.Total {
		return Report{}, errors.New("conservation check failed")
	}
	return r, nil
}

// UsageLine is one frozen allocation of a request.
type UsageLine struct {
	RequestID string `json:"requestId"`
	Date      Date   `json:"date"`
	BatchID   string `json:"batchId"`
	Minutes   int    `json:"minutes"`
	Canceled  bool   `json:"canceled"`
}

// Usage lists every request's per-batch allocation for an employee,
// ordered by request date then request ID.
func (l *Ledger) Usage(employee string) []UsageLine {
	reqs := make([]*Request, 0, len(l.st.Requests))
	for _, r := range l.st.Requests {
		if r.Employee == employee {
			reqs = append(reqs, r)
		}
	}
	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Date != reqs[j].Date {
			return reqs[i].Date < reqs[j].Date
		}
		return reqs[i].ID < reqs[j].ID
	})
	var out []UsageLine
	for _, r := range reqs {
		for _, a := range r.Allocs {
			out = append(out, UsageLine{
				RequestID: r.ID, Date: r.Date, BatchID: a.BatchID,
				Minutes: a.Minutes, Canceled: r.Canceled,
			})
		}
	}
	return out
}
