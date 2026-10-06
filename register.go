// Package main implements a local bitemporal asset ownership register.
//
// Two time axes are tracked:
//   - Business (valid) time: the [from, to) date range an ownership fact
//     refers to, given as Gregorian calendar dates.
//   - Record (transaction) time: the integer second at which a batch of
//     revisions was recorded by the system.
package main

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Date is a Gregorian calendar date in YYYY-MM-DD form.
type Date struct {
	time.Time
}

// ParseDate validates a YYYY-MM-DD calendar date.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: %w", s, err)
	}
	// Reject non-normalized input such as 2026-02-30.
	if t.Format("2006-01-02") != s {
		return Date{}, fmt.Errorf("invalid calendar date %q", s)
	}
	return Date{t}, nil
}

func (d Date) String() string { return d.Format("2006-01-02") }

// Revision is one item of a submitted batch. An empty Unit revokes any
// ownership recorded for the range.
type Revision struct {
	Device string `json:"device"`
	Unit   string `json:"unit"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Batch is a set of revisions recorded atomically at RecordTime.
type Batch struct {
	RecordTime int64      `json:"record_time"`
	Items      []Revision `json:"items"`
}

// segment is a stored ownership fact over a business-time range.
type segment struct {
	From Date
	To   Date
	Unit string
}

// Span is a query result interval; Unit is "" for a known gap.
type Span struct {
	From string `json:"from"`
	To   string `json:"to"`
	Unit string `json:"unit"`
}

// Change describes one minimal segment where the ownership value differs
// between two record-time cutoffs. Empty string means "no record".
type Change struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Before string `json:"before"`
	After  string `json:"after"`
}

var (
	errEmptyRange = errors.New("range must be non-empty with from < to")
	errOverlap    = errors.New("ranges for the same device overlap within the batch")
)

// validateRevision parses and checks a single revision.
func validateRevision(r Revision) (Date, Date, error) {
	if r.Device == "" {
		return Date{}, Date{}, errors.New("device must not be empty")
	}
	from, err := ParseDate(r.From)
	if err != nil {
		return Date{}, Date{}, err
	}
	to, err := ParseDate(r.To)
	if err != nil {
		return Date{}, Date{}, err
	}
	if !from.Before(to.Time) {
		return Date{}, Date{}, fmt.Errorf("device %q: %w", r.Device, errEmptyRange)
	}
	return from, to, nil
}

// validateBatch checks the whole batch before anything is applied.
func validateBatch(b Batch) error {
	if len(b.Items) == 0 {
		return errors.New("batch must contain at least one revision")
	}
	type rng struct{ from, to Date }
	byDevice := map[string][]rng{}
	for _, it := range b.Items {
		from, to, err := validateRevision(it)
		if err != nil {
			return err
		}
		byDevice[it.Device] = append(byDevice[it.Device], rng{from, to})
	}
	for dev, rs := range byDevice {
		sort.Slice(rs, func(i, j int) bool { return rs[i].from.Before(rs[j].from.Time) })
		for i := 1; i < len(rs); i++ {
			if rs[i].from.Before(rs[i-1].to.Time) {
				return fmt.Errorf("device %q: %w", dev, errOverlap)
			}
		}
	}
	return nil
}

// Register is the bitemporal archive. batches is the append-only revision
// evidence; current holds the latest known business-time segments.
type Register struct {
	batches        []Batch
	current        map[string][]segment
	lastRecordTime int64
	hasBatch       bool
}

// NewRegister returns an empty register.
func NewRegister() *Register {
	return &Register{current: map[string][]segment{}}
}

// LastRecordTime reports the record time of the last applied batch.
func (r *Register) LastRecordTime() (int64, bool) { return r.lastRecordTime, r.hasBatch }

// Submit validates the whole batch and, only if valid, applies it
// atomically. Record times of successful batches must strictly increase.
func (r *Register) Submit(b Batch) error {
	if err := validateBatch(b); err != nil {
		return err
	}
	if r.hasBatch && b.RecordTime <= r.lastRecordTime {
		return fmt.Errorf("record time %d is not after last record time %d", b.RecordTime, r.lastRecordTime)
	}
	for _, it := range b.Items {
		from, to, _ := validateRevision(it)
		r.current[it.Device] = applyRevision(r.current[it.Device], from, to, it.Unit)
	}
	r.batches = append(r.batches, b)
	r.lastRecordTime = b.RecordTime
	r.hasBatch = true
	return nil
}

// applyRevision overwrites the business range [from, to) with unit,
// keeping uncovered parts of existing segments. Empty unit clears.
func applyRevision(segs []segment, from, to Date, unit string) []segment {
	var out []segment
	for _, s := range segs {
		if !s.From.Before(to.Time) || !from.Before(s.To.Time) {
			out = append(out, s) // disjoint
			continue
		}
		if s.From.Before(from.Time) {
			out = append(out, segment{s.From, from, s.Unit})
		}
		if to.Before(s.To.Time) {
			out = append(out, segment{to, s.To, s.Unit})
		}
	}
	if unit != "" {
		out = append(out, segment{from, to, unit})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From.Before(out[j].From.Time) })
	return out
}

// segmentsAt replays revisions recorded no later than cutoff.
func (r *Register) segmentsAt(device string, cutoff int64) []segment {
	var segs []segment
	for _, b := range r.batches {
		if b.RecordTime > cutoff {
			break
		}
		for _, it := range b.Items {
			if it.Device != device {
				continue
			}
			from, to, _ := validateRevision(it)
			segs = applyRevision(segs, from, to, it.Unit)
		}
	}
	return segs
}

// mergeAdjacent merges neighbouring segments with the same unit.
func mergeAdjacent(segs []segment) []segment {
	var out []segment
	for _, s := range segs {
		if n := len(out); n > 0 && out[n-1].Unit == s.Unit && !out[n-1].To.Before(s.From.Time) {
			out[n-1].To = s.To
			continue
		}
		out = append(out, s)
	}
	return out
}

// Timeline returns the merged ownership timeline of a device as known at
// the record-time cutoff. Only revisions recorded no later than cutoff
// are visible.
func (r *Register) Timeline(device string, cutoff int64) []Span {
	segs := mergeAdjacent(r.segmentsAt(device, cutoff))
	spans := make([]Span, 0, len(segs))
	for _, s := range segs {
		spans = append(spans, Span{From: s.From.String(), To: s.To.String(), Unit: s.Unit})
	}
	return spans
}

// OwnerAt returns the unit owning the device on a business date as known
// at the record-time cutoff. found is false when nothing is recorded.
func (r *Register) OwnerAt(device, date string, cutoff int64) (unit string, found bool, err error) {
	d, err := ParseDate(date)
	if err != nil {
		return "", false, err
	}
	for _, s := range r.segmentsAt(device, cutoff) {
		if !d.Before(s.From.Time) && d.Before(s.To.Time) {
			return s.Unit, true, nil
		}
	}
	return "", false, nil
}

// Diff compares the full business timeline of a device as known at two
// record-time cutoffs and returns the minimal segments where the value
// actually changed. Empty Before/After means "no record".
func (r *Register) Diff(device string, cutoffA, cutoffB int64) []Change {
	a := r.segmentsAt(device, cutoffA)
	b := r.segmentsAt(device, cutoffB)
	points := map[Date]bool{}
	for _, s := range a {
		points[s.From], points[s.To] = true, true
	}
	for _, s := range b {
		points[s.From], points[s.To] = true, true
	}
	var ps []Date
	for p := range points {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Before(ps[j].Time) })
	valueAt := func(segs []segment, d Date) string {
		for _, s := range segs {
			if !d.Before(s.From.Time) && d.Before(s.To.Time) {
				return s.Unit
			}
		}
		return ""
	}
	var changes []Change
	for i := 0; i+1 < len(ps); i++ {
		va, vb := valueAt(a, ps[i]), valueAt(b, ps[i])
		if va == vb {
			continue
		}
		c := Change{From: ps[i].String(), To: ps[i+1].String(), Before: va, After: vb}
		if n := len(changes); n > 0 && changes[n-1].Before == c.Before && changes[n-1].After == c.After && changes[n-1].To == c.From {
			changes[n-1].To = c.To
			continue
		}
		changes = append(changes, c)
	}
	return changes
}
