package tzif

import (
	"sort"
	"time"
)

// Match is one absolute instant corresponding to a local wall-clock time.
type Match struct {
	Unix   int64
	Offset int64
	Abbr   string
}

// Gap describes a local-time interval that never occurs because of a
// forward jump at a transition.
type Gap struct {
	StartLocal time.Time // first missing local time (inclusive)
	EndLocal   time.Time // first valid local time after the gap (exclusive)
	AtUnix     int64     // transition instant causing the gap
}

// LookupLocal finds every absolute instant in the service window whose local
// wall-clock time equals local (interpreted as a naive UTC clock reading).
// If none exist and the local time falls inside a forward-jump gap, the gap
// is reported.
func (z *Zone) LookupLocal(local time.Time) (matches []Match, gap *Gap) {
	target := local.UTC()
	first, last := z.Window()
	// Walk constant-offset intervals; interval i covers
	// [Transitions[i], Transitions[i+1]) with type Types[i].
	for i := 0; i < len(z.Transitions); i++ {
		start := z.Transitions[i]
		end := last
		if i+1 < len(z.Transitions) {
			end = z.Transitions[i+1]
		}
		lt := z.Locals[z.Types[i]]
		// unix = local - offset
		u := target.Unix() - lt.Offset
		if u >= start && u < end && u >= first && u < last {
			matches = append(matches, Match{Unix: u, Offset: lt.Offset, Abbr: lt.Abbr})
		}
	}
	sort.Slice(matches, func(a, b int) bool { return matches[a].Unix < matches[b].Unix })
	if len(matches) > 0 {
		return matches, nil
	}
	// Detect a forward-jump gap containing the target.
	for i := 1; i < len(z.Transitions); i++ {
		prev := z.Locals[z.Types[i-1]]
		cur := z.Locals[z.Types[i]]
		if cur.Offset > prev.Offset {
			t := z.Transitions[i]
			gapStart := time.Unix(t, 0).UTC().Add(time.Duration(prev.Offset) * time.Second)
			gapEnd := time.Unix(t, 0).UTC().Add(time.Duration(cur.Offset) * time.Second)
			if !target.Before(gapStart) && target.Before(gapEnd) {
				return nil, &Gap{StartLocal: gapStart, EndLocal: gapEnd, AtUnix: t}
			}
		}
	}
	return nil, nil
}

// Interval is a maximal constant-offset span inside the service window.
type Interval struct {
	StartUnix int64
	EndUnix   int64 // exclusive; equals the window end for the last interval
	Offset    int64
	Abbr      string
	IsDST     bool
}

// Intervals lists the constant-offset intervals covering the service window.
func (z *Zone) Intervals() []Interval {
	_, last := z.Window()
	out := make([]Interval, 0, len(z.Transitions))
	for i := 0; i < len(z.Transitions); i++ {
		end := last
		if i+1 < len(z.Transitions) {
			end = z.Transitions[i+1]
		}
		if end <= z.Transitions[i] {
			continue
		}
		lt := z.Locals[z.Types[i]]
		out = append(out, Interval{
			StartUnix: z.Transitions[i],
			EndUnix:   end,
			Offset:    lt.Offset,
			Abbr:      lt.Abbr,
			IsDST:     lt.IsDST,
		})
	}
	return out
}