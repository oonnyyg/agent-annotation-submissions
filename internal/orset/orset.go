
// Package orset implements an observed-remove set (OR-Set) of string tags
// that converges across replicas through snapshot exchange.
package orset

import (
	"errors"
	"fmt"
	"sort"
)

// Dot uniquely identifies one add operation: the replica that issued it and
// the per-replica sequence number at the time.
type Dot struct {
	Replica string `json:"replica"`
	Seq     uint64 `json:"seq"`
}

func (d Dot) String() string { return fmt.Sprintf("%s#%d", d.Replica, d.Seq) }

// State is the full replica state: identity, local sequence counter, the set
// of observed add dots per tag, and the set of removed dots (tombstones).
type State struct {
	Replica string `json:"replica"`
	Seq     uint64 `json:"seq"`
	// Adds maps a tag to the add dots currently known for it.
	Adds map[string][]Dot `json:"adds"`
	// Removed lists add dots that have been observed and then removed.
	Removed []Dot `json:"removed"`
}

// New creates an empty state for the given replica identity.
func New(replica string) (*State, error) {
	if replica == "" {
		return nil, errors.New("orset: replica identity must not be empty")
	}
	return &State{
		Replica: replica,
		Adds:    map[string][]Dot{},
		Removed: []Dot{},
	}, nil
}

// Add records a new add dot for tag and returns it. Re-adding an existing
// tag still produces a fresh, unique dot.
func (s *State) Add(tag string) (Dot, error) {
	if tag == "" {
		return Dot{}, errors.New("orset: tag must not be empty")
	}
	s.Seq++
	d := Dot{Replica: s.Replica, Seq: s.Seq}
	s.Adds[tag] = append(s.Adds[tag], d)
	return d, nil
}

// Remove deletes every add dot for tag that this replica has already
// observed. It returns the dots that were removed. Adds not yet observed
// (concurrent adds on other replicas) are unaffected.
func (s *State) Remove(tag string) []Dot {
	dots := s.Adds[tag]
	if len(dots) == 0 {
		return nil
	}
	removed := make([]Dot, 0, len(dots))
	for _, d := range dots {
		if !containsDot(s.Removed, d) {
			s.Removed = append(s.Removed, d)
			removed = append(removed, d)
		}
	}
	return removed
}

func containsDot(dots []Dot, d Dot) bool {
	for _, x := range dots {
		if x == d {
			return true
		}
	}
	return false
}

// Visible reports whether tag is currently visible: it has at least one add
// dot that has not been removed.
func (s *State) Visible(tag string) bool {
	for _, d := range s.Adds[tag] {
		if !containsDot(s.Removed, d) {
			return true
		}
	}
	return false
}

// Tags returns the sorted list of visible tags, each appearing once.
func (s *State) Tags() []string {
	var out []string
	for tag := range s.Adds {
		if s.Visible(tag) {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

// Evidence describes why a tag is or is not visible.
type Evidence struct {
	Tag     string   `json:"tag"`
	Visible bool     `json:"visible"`
	Live    []string `json:"live"`
	Removed []string `json:"removed"`
}

// Explain returns the add-dot evidence for tag, split into live and removed
// dots, so conflicts can be inspected.
func (s *State) Explain(tag string) Evidence {
	ev := Evidence{Tag: tag}
	for _, d := range s.Adds[tag] {
		if containsDot(s.Removed, d) {
			ev.Removed = append(ev.Removed, d.String())
		} else {
			ev.Live = append(ev.Live, d.String())
		}
	}
	sort.Strings(ev.Live)
	sort.Strings(ev.Removed)
	ev.Visible = len(ev.Live) > 0
	return ev
}
