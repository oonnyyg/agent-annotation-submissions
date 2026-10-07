
package orset

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Snapshot is the exchange format between replicas. It carries both add and
// remove evidence so merges converge regardless of order or duplication.
type Snapshot struct {
	// Adds maps a tag to the add dots known for it.
	Adds map[string][]Dot `json:"adds"`
	// Removed lists add dots that have been removed.
	Removed []Dot `json:"removed"`
}

// Export returns a snapshot of the current add/remove evidence.
func (s *State) Export() Snapshot {
	adds := make(map[string][]Dot, len(s.Adds))
	for tag, dots := range s.Adds {
		adds[tag] = append([]Dot(nil), dots...)
	}
	return Snapshot{
		Adds:    adds,
		Removed: append([]Dot(nil), s.Removed...),
	}
}

// ExportJSON serializes Export to JSON.
func (s *State) ExportJSON() ([]byte, error) {
	return json.Marshal(s.Export())
}

// MergeJSON validates and merges a JSON snapshot. The whole merge is
// rejected (state untouched) if the payload is malformed or maps the same
// dot to two different tags.
func (s *State) MergeJSON(data []byte) error {
	var snap Snapshot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snap); err != nil {
		return fmt.Errorf("orset: invalid snapshot: %w", err)
	}
	return s.Merge(snap)
}

// Merge merges a snapshot into the local state. Merging is idempotent and
// order-independent: adds and removals are unioned, and a removed dot stays
// removed even if its add arrives later. The merge is rejected atomically if
// the same dot is bound to different tags.
func (s *State) Merge(snap Snapshot) error {
	if err := snap.Validate(); err != nil {
		return err
	}
	// Cross-check against local state before mutating anything.
	owner := s.dotOwners()
	for tag, dots := range snap.Adds {
		for _, d := range dots {
			if other, ok := owner[d]; ok && other != tag {
				return fmt.Errorf("orset: dot %s bound to both %q and %q", d, other, tag)
			}
		}
	}
	for tag, dots := range snap.Adds {
		for _, d := range dots {
			if !containsDot(s.Adds[tag], d) {
				s.Adds[tag] = append(s.Adds[tag], d)
			}
		}
	}
	for _, d := range snap.Removed {
		if !containsDot(s.Removed, d) {
			s.Removed = append(s.Removed, d)
		}
	}
	return nil
}

// Validate checks snapshot internal consistency: no dot may appear under two
// different tags, and no tag may be empty.
func (snap Snapshot) Validate() error {
	owner := map[Dot]string{}
	for tag, dots := range snap.Adds {
		if tag == "" {
			return fmt.Errorf("orset: snapshot contains empty tag")
		}
		for _, d := range dots {
			if d.Replica == "" || d.Seq == 0 {
				return fmt.Errorf("orset: snapshot contains invalid dot %+v", d)
			}
			if other, ok := owner[d]; ok && other != tag {
				return fmt.Errorf("orset: dot %s bound to both %q and %q", d, other, tag)
			}
			owner[d] = tag
		}
	}
	return nil
}

func (s *State) dotOwners() map[Dot]string {
	owner := map[Dot]string{}
	for tag, dots := range s.Adds {
		for _, d := range dots {
			owner[d] = tag
		}
	}
	return owner
}
