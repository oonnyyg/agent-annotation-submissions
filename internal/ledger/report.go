package ledger

import "sort"

// BatchReport breaks one batch down into minute categories on date d.
type BatchReport struct {
	Batch   string `json:"batch"`
	Start   Date   `json:"start"`
	End     Date   `json:"end"`
	Total   int    `json:"total"`
	Used    int    `json:"used"`
	Expired int    `json:"expiredReturned"`
	// Remaining is split by the batch's lifecycle state on d:
	// not-yet-effective, available, or expired-unused.
	Pending   int `json:"pending,omitempty"`
	Available int `json:"available,omitempty"`
	Lapsed    int `json:"lapsed,omitempty"`
}

// Report summarizes an employee's balances on date d and proves
// conservation: total == available+used+pending+lapsed+expired.
type Report struct {
	Employee  string        `json:"employee"`
	Date      Date          `json:"date"`
	Batches   []BatchReport `json:"batches"`
	Total     int           `json:"total"`
	Available int           `json:"available"`
	Used      int           `json:"used"`
	Pending   int           `json:"pending"`
	Lapsed    int           `json:"lapsed"`
	Expired   int           `json:"expiredReturned"`
	Balanced  bool          `json:"balanced"`
}

// Report computes the balance sheet for employee on date d. Used counts
// minutes frozen in non-cancelled requests, attributed to their batches.
func (l *Ledger) Report(employee string, d Date) Report {
	used := map[string]int{}
	for _, r := range l.Requests {
		if r.Employee != employee || r.Cancelled {
			continue
		}
		for _, ded := range r.Deductions {
			used[ded.Batch] += ded.Minutes
		}
	}
	rep := Report{Employee: employee, Date: d, Balanced: true}
	for _, b := range l.Batches {
		if b.Employee != employee {
			continue
		}
		br := BatchReport{Batch: b.ID, Start: b.Start, End: b.End, Total: b.Total, Used: used[b.ID], Expired: b.ExpiredReturned}
		switch {
		case d < b.Start:
			br.Pending = b.Remaining
		case d < b.End:
			br.Available = b.Remaining
		default:
			br.Lapsed = b.Remaining
		}
		rep.Batches = append(rep.Batches, br)
		rep.Total += b.Total
		rep.Available += br.Available
		rep.Used += br.Used
		rep.Pending += br.Pending
		rep.Lapsed += br.Lapsed
		rep.Expired += br.Expired
	}
	sort.Slice(rep.Batches, func(i, j int) bool { return rep.Batches[i].Batch < rep.Batches[j].Batch })
	rep.Balanced = rep.Total == rep.Available+rep.Used+rep.Pending+rep.Lapsed+rep.Expired
	return rep
}

// Usage returns the frozen deduction breakdown of a request.
func (l *Ledger) Usage(id string) (RequestResult, error) {
	req, ok := l.Requests[id]
	if !ok {
		return RequestResult{}, errNotFound(id)
	}
	status := "approved"
	if req.Cancelled {
		status = "cancelled"
	}
	return RequestResult{Status: status, ID: id, Minutes: req.Minutes, Deductions: req.Deductions}, nil
}

type errNotFound string

func (e errNotFound) Error() string { return "request " + string(e) + " not found" }
