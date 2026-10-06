// Package ledger implements an expiring leave-balance bank.
//
// All quantities are integer minutes. Balances live in batches with a
// half-open validity range [Start, End). The ledger never reads the
// machine clock: expiry is judged purely against operation/query dates,
// and operation dates may only advance monotonically.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Date is a calendar day in YYYY-MM-DD form. String comparison is valid
// because the format is fixed-width and zero-padded.
type Date string

// Batch is a grant of leave minutes valid on [Start, End).
type Batch struct {
	ID        string `json:"id"`
	Employee  string `json:"employee"`
	Start     Date   `json:"start"`
	End       Date   `json:"end"`
	Total     int    `json:"total"`
	Remaining int    `json:"remaining"`
	// ExpiredReturned accumulates minutes returned by cancellations that
	// landed after the batch expired. They never become usable again.
	ExpiredReturned int `json:"expiredReturned"`
}

// Deduction records how many minutes one batch contributed to a request.
type Deduction struct {
	Batch   string `json:"batch"`
	Minutes int    `json:"minutes"`
}

// Request is a leave application. Its deduction breakdown is frozen at
// approval time so later grants or cancellations cannot rewrite history.
type Request struct {
	ID         string      `json:"id"`
	Employee   string      `json:"employee"`
	Date       Date        `json:"date"`
	Minutes    int         `json:"minutes"`
	Deductions []Deduction `json:"deductions"`
	Cancelled  bool        `json:"cancelled"`
}

// Ledger is the in-memory state, persisted as one JSON document.
type Ledger struct {
	Batches  map[string]*Batch   `json:"batches"`
	Requests map[string]*Request `json:"requests"`
	LastDate Date                `json:"lastDate"`
}

// New returns an empty ledger.
func New() *Ledger {
	return &Ledger{
		Batches:  map[string]*Batch{},
		Requests: map[string]*Request{},
	}
}

// Load reads state from dir/state.json, or returns a fresh ledger when
// the file does not exist yet.
func Load(dir string) (*Ledger, error) {
	path := filepath.Join(dir, "state.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	l := New()
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", path, err)
	}
	return l, nil
}

// Save writes the ledger atomically (temp file + rename) into dir.
func (l *Ledger) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}

// advance enforces the non-decreasing operation-date rule.
func (l *Ledger) advance(d Date) error {
	if l.LastDate != "" && d < l.LastDate {
		return fmt.Errorf("operation date %s goes backwards (last: %s)", d, l.LastDate)
	}
	l.LastDate = d
	return nil
}

// active reports whether a batch is usable on date d: [Start, End).
func active(b *Batch, d Date) bool {
	return b.Start <= d && d < b.End
}

// usableBatches lists the employee's batches usable on d, ordered by
// earliest expiry, ties broken by batch ID (lexicographic).
func (l *Ledger) usableBatches(employee string, d Date) []*Batch {
	var out []*Batch
	for _, b := range l.Batches {
		if b.Employee == employee && active(b, d) {
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
