package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// stateFile is JSON: {"inventory": {"100": 5}, "requests": {...}}.
// Plan keys are ints; encode via a string-keyed map.

type stateJSON struct {
	Inventory map[string]int `json:"inventory"`
	Requests  map[string]*RequestJSON `json:"requests"`
}

type RequestJSON struct {
	ID     string         `json:"id"`
	Amount int            `json:"amount"`
	Plan   map[string]int `json:"plan"`
	Status string         `json:"status"`
}

func planToStrings(p Plan) map[string]int {
	m := map[string]int{}
	for d, n := range p {
		m[strconv.Itoa(d)] = n
	}
	return m
}

func planFromStrings(m map[string]int) (Plan, error) {
	p := Plan{}
	for k, v := range m {
		d, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("bad denomination key %q", k)
		}
		p[d] = v
	}
	return p, nil
}

// SaveState atomically writes the store to path.
func SaveState(path string, s *Store) error {
	sj := stateJSON{
		Inventory: planToStrings(s.Inventory),
		Requests:  map[string]*RequestJSON{},
	}
	for id, r := range s.Requests {
		sj.Requests[id] = &RequestJSON{
			ID: r.ID, Amount: r.Amount,
			Plan: planToStrings(r.Plan), Status: r.Status,
		}
	}
	data, err := json.MarshalIndent(sj, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadState reads the store from path; missing file yields an empty store.
func LoadState(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewStore(), nil
	}
	if err != nil {
		return nil, err
	}
	var sj stateJSON
	if err := json.Unmarshal(data, &sj); err != nil {
		return nil, fmt.Errorf("corrupt state file: %w", err)
	}
	s := NewStore()
	inv, err := planFromStrings(sj.Inventory)
	if err != nil {
		return nil, err
	}
	s.Inventory = inv
	for id, rj := range sj.Requests {
		plan, err := planFromStrings(rj.Plan)
		if err != nil {
			return nil, err
		}
		s.Requests[id] = &Request{ID: rj.ID, Amount: rj.Amount, Plan: plan, Status: rj.Status}
	}
	return s, nil
}

// ParseCounts parses "100:5,50:10" into a Plan, rejecting duplicates
// and negative counts.
func ParseCounts(spec string) (Plan, error) {
	p := Plan{}
	if strings.TrimSpace(spec) == "" {
		return nil, fmt.Errorf("empty counts spec")
	}
	for _, part := range strings.Split(spec, ",") {
		kv := strings.Split(strings.TrimSpace(part), ":")
		if len(kv) != 2 {
			return nil, fmt.Errorf("bad entry %q, want denom:count", part)
		}
		d, err := strconv.Atoi(strings.TrimSpace(kv[0]))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("bad denomination %q", kv[0])
		}
		n, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			return nil, fmt.Errorf("bad count %q", kv[1])
		}
		if n < 0 {
			return nil, fmt.Errorf("negative count for denomination %d", d)
		}
		if _, dup := p[d]; dup {
			return nil, fmt.Errorf("duplicate denomination %d", d)
		}
		p[d] = n
	}
	return p, nil
}
