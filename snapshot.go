package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
)

// Snapshot is one cumulative histogram observation for an instance.
type Snapshot struct {
	Instance string    `json:"instance"`
	TS       int64     `json:"ts"`
	Boot     string    `json:"boot"`
	Bounds   []float64 `json:"bounds"`
	Counts   []uint64  `json:"counts"`
	Sum      float64   `json:"sum"`
}

func (s *Snapshot) validate() error {
	if s.Instance == "" {
		return fmt.Errorf("snapshot missing instance")
	}
	if s.Boot == "" {
		return fmt.Errorf("instance %s ts %d: missing boot id", s.Instance, s.TS)
	}
	if len(s.Bounds) < 1 {
		return fmt.Errorf("instance %s ts %d: at least one finite bucket bound required", s.Instance, s.TS)
	}
	for i, b := range s.Bounds {
		if math.IsNaN(b) || math.IsInf(b, 0) || b <= 0 {
			return fmt.Errorf("instance %s ts %d: bound %d must be finite and positive, got %v", s.Instance, s.TS, i, b)
		}
		if i > 0 && b <= s.Bounds[i-1] {
			return fmt.Errorf("instance %s ts %d: bounds must be strictly increasing", s.Instance, s.TS)
		}
	}
	if len(s.Counts) != len(s.Bounds)+1 {
		return fmt.Errorf("instance %s ts %d: counts length %d, want %d (bounds+1 for +Inf bucket)", s.Instance, s.TS, len(s.Counts), len(s.Bounds)+1)
	}
	for i := 1; i < len(s.Counts); i++ {
		if s.Counts[i] < s.Counts[i-1] {
			return fmt.Errorf("instance %s ts %d: counts must be non-decreasing", s.Instance, s.TS)
		}
	}
	if math.IsNaN(s.Sum) || s.Sum < 0 {
		return fmt.Errorf("instance %s ts %d: sum must be non-negative", s.Instance, s.TS)
	}
	total := s.Counts[len(s.Counts)-1]
	if total == 0 && s.Sum != 0 {
		return fmt.Errorf("instance %s ts %d: sum must be zero when total count is zero", s.Instance, s.TS)
	}
	return nil
}

// LoadSnapshots reads JSONL snapshots, validates them, rejects duplicate
// (instance, ts) pairs, and returns them grouped by instance sorted by ts.
func LoadSnapshots(r io.Reader) (map[string][]*Snapshot, error) {
	seen := map[string]map[int64]bool{}
	byInstance := map[string][]*Snapshot{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Bytes()
		if len(trimSpace(text)) == 0 {
			continue
		}
		var s Snapshot
		if err := json.Unmarshal(text, &s); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", line, err)
		}
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if seen[s.Instance] == nil {
			seen[s.Instance] = map[int64]bool{}
		}
		if seen[s.Instance][s.TS] {
			return nil, fmt.Errorf("line %d: duplicate snapshot for instance %q at ts %d", line, s.Instance, s.TS)
		}
		seen[s.Instance][s.TS] = true
		byInstance[s.Instance] = append(byInstance[s.Instance], &s)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for _, snaps := range byInstance {
		sort.Slice(snaps, func(i, j int) bool { return snaps[i].TS < snaps[j].TS })
	}
	return byInstance, nil
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			b = b[:len(b)-1]
		} else {
			break
		}
	}
	return b
}
