package main

import (
	"fmt"
	"sort"
)

// Report is the merged window result across instances.
type Report struct {
	Start, End int64
	Bounds     []float64
	// BucketCounts[i] is the cumulative count of requests with latency
	// <= Bounds[i] observed in the window (last entry is the total,
	// including the +Inf bucket).
	BucketCounts []uint64
	TotalCount   uint64
	TotalSum     float64
	RestartGaps  []RestartGap
}

// RestartGap marks an interval where a boot id changed and traffic
// between the old process's last sample and its exit is unobserved.
type RestartGap struct {
	Instance string `json:"instance"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
}

// MergeWindow computes per-instance intervals first, then merges bucket
// deltas and sums. Bounds must be identical across all instances.
func MergeWindow(byInstance map[string][]*Snapshot, start, end int64) (*Report, error) {
	if end <= start {
		return nil, fmt.Errorf("end ts %d must be greater than start ts %d", end, start)
	}
	if len(byInstance) == 0 {
		return nil, fmt.Errorf("no instances in input")
	}
	names := make([]string, 0, len(byInstance))
	for n := range byInstance {
		names = append(names, n)
	}
	sort.Strings(names)
	rep := &Report{Start: start, End: end}
	for _, name := range names {
		ivs, err := InstanceIntervals(name, byInstance[name], start, end)
		if err != nil {
			return nil, err
		}
		snaps := byInstance[name]
		if rep.Bounds == nil {
			rep.Bounds = findSnapshot(snaps, start).Bounds
			rep.BucketCounts = make([]uint64, len(rep.Bounds)+1)
		} else if !sameBounds(findSnapshot(snaps, start).Bounds, rep.Bounds) {
			return nil, fmt.Errorf("instance %q: bucket bounds differ from other instances", name)
		}
		for _, iv := range ivs {
			for b := range iv.Counts {
				rep.BucketCounts[b] += iv.Counts[b]
			}
			rep.TotalSum += iv.Sum
			if iv.Restart {
				rep.RestartGaps = append(rep.RestartGaps, RestartGap{Instance: name, Start: iv.Start, End: iv.End})
			}
		}
	}
	rep.TotalCount = rep.BucketCounts[len(rep.BucketCounts)-1]
	return rep, nil
}

// BucketSegments returns non-cumulative per-bucket counts.
func (r *Report) BucketSegments() []uint64 {
	segs := make([]uint64, len(r.BucketCounts))
	for i, c := range r.BucketCounts {
		if i == 0 {
			segs[i] = c
		} else {
			segs[i] = c - r.BucketCounts[i-1]
		}
	}
	return segs
}

// AvgLatency returns the mean latency and whether it is defined
// (undefined when the window observed zero requests).
func (r *Report) AvgLatency() (float64, bool) {
	if r.TotalCount == 0 {
		return 0, false
	}
	return r.TotalSum / float64(r.TotalCount), true
}

// RequestsPerSecond returns observed requests divided by window length.
func (r *Report) RequestsPerSecond() float64 {
	return float64(r.TotalCount) / float64(r.End-r.Start)
}

// Quantile estimates a latency quantile in [0,1] from the merged
// histogram. It linearly interpolates within finite buckets (first
// bucket lower bound is 0); a quantile landing in the +Inf bucket
// returns the last finite upper bound. Returns ok=false when the
// window observed zero requests.
func (r *Report) Quantile(q float64) (float64, bool) {
	if q < 0 || q > 1 {
		return 0, false
	}
	if r.TotalCount == 0 {
		return 0, false
	}
	if q == 0 {
		return 0, true
	}
	rank := q * float64(r.TotalCount)
	prev := uint64(0)
	for i, cum := range r.BucketCounts {
		if float64(cum) < rank {
			prev = cum
			continue
		}
		lower := 0.0
		if i > 0 {
			lower = r.Bounds[i-1]
		}
		if i == len(r.Bounds) {
			// +Inf bucket: no upper bound to interpolate toward.
			return r.Bounds[len(r.Bounds)-1], true
		}
		upper := r.Bounds[i]
		c := cum - prev
		if c == 0 {
			return upper, true
		}
		frac := (rank - float64(prev)) / float64(c)
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		return lower + frac*(upper-lower), true
	}
	return r.Bounds[len(r.Bounds)-1], true
}
