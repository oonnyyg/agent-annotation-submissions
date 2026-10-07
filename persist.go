package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// diskState is the persisted form of a Store.
type diskState struct {
	Replica string        `json:"replica"`
	NextSeq uint64        `json:"next_seq"`
	Adds    []AddEvidence `json:"adds"`
	Removes []Mark        `json:"removes"`
}

// LoadStore reads state from dir/state.json, or creates a fresh store
// with the given replica identity when the file does not exist.
func LoadStore(dir, replica string) (*Store, error) {
	path := filepath.Join(dir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if replica == "" {
				return nil, fmt.Errorf("no state in %s; run init first", dir)
			}
			return NewStore(replica), nil
		}
		return nil, err
	}
	var ds diskState
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", path, err)
	}
	if ds.Replica == "" || ds.NextSeq == 0 {
		return nil, fmt.Errorf("invalid state file %s", path)
	}
	if replica != "" && replica != ds.Replica {
		return nil, fmt.Errorf("state belongs to replica %q, not %q", ds.Replica, replica)
	}
	s := NewStore(ds.Replica)
	s.NextSeq = ds.NextSeq
	for _, ev := range ds.Adds {
		if old, ok := s.adds[ev.Mark.key()]; ok && old.Tag != ev.Tag {
			return nil, fmt.Errorf("corrupt state: mark %s maps to two tags", ev.Mark.key())
		}
		s.adds[ev.Mark.key()] = ev
	}
	for _, m := range ds.Removes {
		s.removes[m.key()] = m
	}
	return s, nil
}

// SaveStore writes state atomically (temp file + rename).
func SaveStore(dir string, s *Store) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	snap := s.ExportSnapshot()
	ds := diskState{Replica: s.Replica, NextSeq: s.NextSeq, Adds: snap.Adds, Removes: snap.Removes}
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}
