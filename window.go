package main

import (
	"fmt"
	"sort"
)

// Interval is one adjacent-snapshot delta for a single instance.
type Interval struct {
	Instance   string
	Start, End int64
	Counts     []uint64 // per-bucket request counts in this interval
	Sum        float64  // total latency in this interval
	Restart    bool     // boot id changed; old process tail is unobserved
}

func sameBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findSnapshot(snaps []*Snapshot, ts int64) *Snapshot {
	i := sort.Search(len(snaps), func(i int) bool { return snaps[i].TS >= ts })
	if i < len(snaps) && snaps[i].TS == ts {
		return snaps[i]
	}
	return nil
}

// InstanceIntervals computes all adjacent-snapshot intervals for one
// instance within [start, end]. Both endpoints must have exact snapshots
// and bucket bounds must agree across the window.
func InstanceIntervals(inst string, snaps []*Snapshot, start, end int64) ([]*Interval, error) {
	first := findSnapshot(snaps, start)
	if first == nil {
		return nil, fmt.Errorf("instance %q: no snapshot at start ts %d", inst, start)
	}
	if findSnapshot(snaps, end) == nil {
		return nil, fmt.Errorf("instance %q: no snapshot at end ts %d", inst, end)
	}
	var inWindow []*Snapshot
	for _, s := range snaps {
		if s.TS >= start && s.TS <= end {
			inWindow = append(inWindow, s)
		}
	}
	for _, s := range inWindow {
		if !sameBounds(s.Bounds, first.Bounds) {
			return nil, fmt.Errorf("instance %q: bucket bounds at ts %d differ from ts %d", inst, s.TS, start)
		}
	}
	var out []*Interval
	for i := 1; i < len(inWindow); i++ {
		prev, cur := inWindow[i-1], inWindow[i]
		iv := &Interval{Instance: inst, Start: prev.TS, End: cur.TS, Counts: make([]uint64, len(cur.Counts))}
		if cur.Boot == prev.Boot {
			for b := range cur.Counts {
				if cur.Counts[b] < prev.Counts[b] {
					return nil, fmt.Errorf("instance %q: cumulative count decreased between ts %d and %d without boot change", inst, prev.TS, cur.TS)
				}
				iv.Counts[b] = cur.Counts[b] - prev.Counts[b]
			}
			if cur.Sum < prev.Sum {
				return nil, fmt.Errorf("instance %q: cumulative sum decreased between ts %d and %d without boot change", inst, prev.TS, cur.TS)
			}
			iv.Sum = cur.Sum - prev.Sum
		} else {
			// Restart: only the new process's cumulative values are
			// observable; requests after the old process's last sample
			// are unknown and flagged via Restart.
			copy(iv.Counts, cur.Counts)
			iv.Sum = cur.Sum
			iv.Restart = true
		}
		out = append(out, iv)
	}
	return out, nil
}
