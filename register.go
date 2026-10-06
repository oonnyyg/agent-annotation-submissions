
package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Date is a proleptic Gregorian calendar day (no time-of-day).
type Date struct {
	y, m, d int
}

func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", s)
	}
	return Date{t.Year(), int(t.Month()), t.Day()}, nil
}

func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.y, d.m, d.d) }

func (d Date) Before(o Date) bool {
	if d.y != o.y {
		return d.y < o.y
	}
	if d.m != o.m {
		return d.m < o.m
	}
	return d.d < o.d
}

func (d Date) After(o Date) bool { return o.Before(d) }
func (d Date) Equal(o Date) bool { return d == o }

// Revision assigns device ownership of unit over [From, To) in business
// time. Empty Unit revokes ownership for the span.
type Revision struct {
	Device string `json:"device"`
	Unit   string `json:"unit"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Batch is one JSONL submission: all revisions recorded at RecordedAt
// (integer seconds, supplied by the caller, never the wall clock).
type Batch struct {
	RecordedAt int64      `json:"recorded_at"`
	Revisions  []Revision `json:"revisions"`
}

// Segment is a maximal half-open business span with one owner.
// Unit == "" means "no recorded ownership" (gap).
type Segment struct {
	From Date
	To   Date
	Unit string
}

type appliedRevision struct {
	at       int64
	from, to Date
	unit     string
}

type Register struct {
	lastRecordedAt int64
	hasBatch       bool
	// history[device] holds revisions in application (recorded) order.
	history map[string][]appliedRevision
}

func NewRegister() *Register {
	return &Register{history: map[string][]appliedRevision{}}
}

func (r *Register) LastRecordedAt() (int64, bool) { return r.lastRecordedAt, r.hasBatch }

// ValidateBatch checks a batch without mutating the register.
func (r *Register) ValidateBatch(b *Batch) error {
	if len(b.Revisions) == 0 {
		return fmt.Errorf("batch has no revisions")
	}
	if r.hasBatch && b.RecordedAt <= r.lastRecordedAt {
		return fmt.Errorf("recorded_at %d not strictly greater than last %d", b.RecordedAt, r.lastRecordedAt)
	}
	seen := map[string][][2]Date{}
	for i, rev := range b.Revisions {
		if strings.TrimSpace(rev.Device) == "" {
			return fmt.Errorf("revision %d: empty device", i)
		}
		from, err := ParseDate(rev.From)
		if err != nil {
			return fmt.Errorf("revision %d: %v", i, err)
		}
		to, err := ParseDate(rev.To)
		if err != nil {
			return fmt.Errorf("revision %d: %v", i, err)
		}
		if !from.Before(to) {
			return fmt.Errorf("revision %d: empty or inverted span [%s, %s)", i, rev.From, rev.To)
		}
		for _, sp := range seen[rev.Device] {
			if from.Before(sp[1]) && sp[0].Before(to) {
				return fmt.Errorf("revision %d: overlapping spans for device %q in same batch", i, rev.Device)
			}
		}
		seen[rev.Device] = append(seen[rev.Device], [2]Date{from, to})
	}
	return nil
}

// Apply validates the whole batch first; on any error nothing changes.
func (r *Register) Apply(b *Batch) error {
	if err := r.ValidateBatch(b); err != nil {
		return err
	}
	for _, rev := range b.Revisions {
		from, _ := ParseDate(rev.From)
		to, _ := ParseDate(rev.To)
		r.history[rev.Device] = append(r.history[rev.Device], appliedRevision{b.RecordedAt, from, to, rev.Unit})
	}
	r.lastRecordedAt = b.RecordedAt
	r.hasBatch = true
	return nil
}

// Timeline returns the merged ownership timeline for device as known at or
// before knowledge cutoff (integer seconds). Only spans with a known owner
// are returned; gaps are simply absent.
func (r *Register) Timeline(device string, cutoff int64) []Segment {
	segs := []Segment{} // non-overlapping
	override := func(from, to Date, unit string) {
		var next []Segment
		for _, s := range segs {
			if !s.From.Before(to) || !from.Before(s.To) {
				next = append(next, s) // disjoint
				continue
			}
			if s.From.Before(from) {
				next = append(next, Segment{s.From, from, s.Unit})
			}
			if to.Before(s.To) {
				next = append(next, Segment{to, s.To, s.Unit})
			}
		}
		segs = next
		if unit != "" {
			segs = append(segs, Segment{from, to, unit})
		}
	}
	for _, rev := range r.history[device] {
		if rev.at > cutoff {
			continue
		}
		override(rev.from, rev.to, rev.unit)
	}
	sort.Slice(segs, func(a, b int) bool { return segs[a].From.Before(segs[b].From) })
	// Merge adjacent segments with the same unit.
	var merged []Segment
	for _, s := range segs {
		if len(merged) > 0 && merged[len(merged)-1].Unit == s.Unit && merged[len(merged)-1].To.Equal(s.From) {
			merged[len(merged)-1].To = s.To
			continue
		}
		merged = append(merged, s)
	}
	return merged
}
