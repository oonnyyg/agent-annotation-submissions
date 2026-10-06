package tzif

import (
	"fmt"
	"time"
)

// LocalTime describes the local wall-clock type attached to a transition.
type LocalTime struct {
	Offset int64  // seconds east of UTC
	IsDST  bool   // informational only; never used for offset math
	Abbr   string // time zone abbreviation
}

// Zone is an audited time zone built from the 64-bit TZif data block.
type Zone struct {
	Transitions []int64 // strictly increasing transition instants (unix seconds)
	Types       []int   // local-time type index after each transition
	Locals      []LocalTime
}

// Window returns the half-open service window [first, last) in unix seconds.
func (z *Zone) Window() (first, last int64) {
	return z.Transitions[0], z.Transitions[len(z.Transitions)-1]
}

// ErrOutOfWindow reports a query outside the service window.
type ErrOutOfWindow struct {
	Unix        int64
	First, Last int64
}

func (e *ErrOutOfWindow) Error() string {
	return fmt.Sprintf("unix time %d outside service window [%d, %d)", e.Unix, e.First, e.Last)
}

// indexAfter returns the local-time type index in effect at unix second t.
// At an exact transition instant the new type applies.
func (z *Zone) indexAfter(t int64) int {
	lo, hi := 0, len(z.Transitions)
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if z.Transitions[mid] <= t {
			lo = mid
		} else {
			hi = mid
		}
	}
	return z.Types[lo]
}

// At converts an absolute unix second to local time within the window.
func (z *Zone) At(t int64) (wall time.Time, lt LocalTime, err error) {
	first, last := z.Window()
	if t < first || t >= last {
		return wall, lt, &ErrOutOfWindow{Unix: t, First: first, Last: last}
	}
	lt = z.Locals[z.indexAfter(t)]
	wall = time.Unix(t, 0).UTC().Add(time.Duration(lt.Offset) * time.Second)
	return wall, lt, nil
}