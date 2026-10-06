package tzif

import (
	"fmt"
	"sort"
	"time"
)

// Interval is a constant-offset span within the served window.
type Interval struct {
	Start  int64 // inclusive UTC second
	End    int64 // exclusive UTC second
	Offset int64
	Abbr   string
}

// Intervals returns the constant-offset intervals covering [first, last).
// The type installed by the final transition is never served.
func (f *File) Intervals() []Interval {
	var out []Interval
	for i := 0; i+1 < len(f.Transitions); i++ {
		t := f.Types[f.TransTypes[i]]
		out = append(out, Interval{
			Start:  f.Transitions[i],
			End:    f.Transitions[i+1],
			Offset: t.Offset,
			Abbr:   t.Abbr,
		})
	}
	return out
}

// ErrOutsideWindow is returned for queries outside [first, last).
type ErrOutsideWindow struct {
	Query      int64
	First      int64
	Last       int64
	LocalQuery bool
}

func (e *ErrOutsideWindow) Error() string {
	return fmt.Sprintf("query is outside the served window [%d, %d)", e.First, e.Last)
}

// At converts an absolute time (UTC seconds) to local wall time.
// The transition instant itself uses the newly installed type.
func (f *File) At(unix int64) (local int64, typ Type, err error) {
	first, last := f.Window()
	if unix < first || unix >= last {
		return 0, Type{}, &ErrOutsideWindow{Query: unix, First: first, Last: last}
	}
	i := sort.Search(len(f.Transitions), func(i int) bool {
		return f.Transitions[i] > unix
	}) - 1
	typ = f.Types[f.TransTypes[i]]
	return unix + typ.Offset, typ, nil
}

// LocalResult maps one local wall-clock query to its absolute candidates.
type LocalResult struct {
	Times []int64  // all matching UTC seconds, sorted
	Gap   *[2]int64 // if no match due to a forward jump: [gapStart, gapEnd) local
}

// FromLocal resolves a local wall-clock second to all absolute times in the
// window that display it. Forward jumps yield a gap range; backward jumps
// yield multiple candidates, all preserved.
func (f *File) FromLocal(local int64) (*LocalResult, error) {
	first, last := f.Window()
	res := &LocalResult{}
	ivs := f.Intervals()
	for _, iv := range ivs {
		t := local - iv.Offset
		if t >= iv.Start && t < iv.End {
			res.Times = append(res.Times, t)
		}
	}
	sort.Slice(res.Times, func(i, j int) bool { return res.Times[i] < res.Times[j] })
	if len(res.Times) > 0 {
		return res, nil
	}
	// No match: check whether the local time falls in a forward-jump gap
	// inside the window. At boundary b, offset change old->new with
	// new > old makes local times [b+old, b+new) nonexistent.
	for i := 1; i < len(f.Transitions); i++ {
		b := f.Transitions[i]
		if b >= last {
			break
		}
		oldOff := f.Types[f.TransTypes[i-1]].Offset
		newOff := f.Types[f.TransTypes[i]].Offset
		if newOff > oldOff {
			lo, hi := b+oldOff, b+newOff
			if local >= lo && local < hi {
				res.Gap = &[2]int64{lo, hi}
				return res, nil
			}
		}
	}
	// Outside any interval's local coverage: report window bounds in local terms.
	return nil, &ErrOutsideWindow{Query: local, First: first, Last: last, LocalQuery: true}
}

// FormatCivil renders unix seconds as a civil time string (no zone suffix).
func FormatCivil(unix int64) string {
	return time.Unix(unix, 0).UTC().Format("2006-01-02T15:04:05")
}

// ParseCivil parses "YYYY-MM-DDTHH:MM:SS" as a zone-less wall-clock time.
func ParseCivil(s string) (int64, error) {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return 0, fmt.Errorf("invalid local time %q: use YYYY-MM-DDTHH:MM:SS", s)
	}
	return t.Unix(), nil
}
