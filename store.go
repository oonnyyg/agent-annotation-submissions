// Package main implements an observed-remove tag set (OR-Set, add-wins)
// for offline replicas that converge by exchanging JSON snapshots.
package main

import (
	"errors"
	"fmt"
	"sort"
)

// Mark uniquely identifies one "add tag" event. It is never reused.
type Mark struct {
	Replica string `json:"replica"`
	Seq     uint64 `json:"seq"`
}

func (m Mark) key() string { return fmt.Sprintf("%s#%d", m.Replica, m.Seq) }

// AddEvidence records that Mark added Tag.
type AddEvidence struct {
	Mark Mark   `json:"mark"`
	Tag  string `json:"tag"`
}

// Snapshot carries all evidence needed to merge two replicas.
type Snapshot struct {
	Replica string        `json:"replica"`
	Adds    []AddEvidence `json:"adds"`
	Removes []Mark        `json:"removes"`
}

// Store is the local replica state.
type Store struct {
	Replica string
	NextSeq uint64
	adds    map[string]AddEvidence // mark key -> evidence
	removes map[string]Mark        // mark key -> tombstone
}

func NewStore(replica string) *Store {
	return &Store{
		Replica: replica,
		NextSeq: 1,
		adds:    map[string]AddEvidence{},
		removes: map[string]Mark{},
	}
}

// Add creates a fresh unique mark for tag, even if tag is already visible.
func (s *Store) Add(tag string) AddEvidence {
	m := Mark{Replica: s.Replica, Seq: s.NextSeq}
	s.NextSeq++
	ev := AddEvidence{Mark: m, Tag: tag}
	s.adds[m.key()] = ev
	return ev
}

// Remove tombstones every add mark for tag this replica has observed.
// Adds not yet observed (concurrent on other replicas) survive.
func (s *Store) Remove(tag string) []Mark {
	var out []Mark
	for _, ev := range s.adds {
		if ev.Tag == tag {
			if _, gone := s.removes[ev.Mark.key()]; !gone {
				s.removes[ev.Mark.key()] = ev.Mark
				out = append(out, ev.Mark)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// VisibleTags returns the deduplicated set of currently visible tags.
func (s *Store) VisibleTags() []string {
	set := map[string]bool{}
	for k, ev := range s.adds {
		if _, gone := s.removes[k]; !gone {
			set[ev.Tag] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// TagEvidence explains why a tag is visible or not.
type TagEvidence struct {
	Tag         string `json:"tag"`
	Visible     bool   `json:"visible"`
	LiveAdds    []Mark `json:"live_adds"`
	RemovedAdds []Mark `json:"removed_adds"`
}

// Explain returns evidence for every known tag (or a single tag if non-empty).
func (s *Store) Explain(only string) []TagEvidence {
	byTag := map[string]*TagEvidence{}
	order := []string{}
	get := func(t string) *TagEvidence {
		if e, ok := byTag[t]; ok {
			return e
		}
		e := &TagEvidence{Tag: t}
		byTag[t] = e
		order = append(order, t)
		return e
	}
	for k, ev := range s.adds {
		e := get(ev.Tag)
		if _, gone := s.removes[k]; gone {
			e.RemovedAdds = append(e.RemovedAdds, ev.Mark)
		} else {
			e.LiveAdds = append(e.LiveAdds, ev.Mark)
		}
	}
	sort.Strings(order)
	out := []TagEvidence{}
	for _, t := range order {
		if only != "" && t != only {
			continue
		}
		e := byTag[t]
		e.Visible = len(e.LiveAdds) > 0
		sort.Slice(e.LiveAdds, func(i, j int) bool { return e.LiveAdds[i].key() < e.LiveAdds[j].key() })
		sort.Slice(e.RemovedAdds, func(i, j int) bool { return e.RemovedAdds[i].key() < e.RemovedAdds[j].key() })
		out = append(out, *e)
	}
	return out
}

// ExportSnapshot returns the full local evidence as a mergeable snapshot.
func (s *Store) ExportSnapshot() Snapshot {
	snap := Snapshot{Replica: s.Replica, Adds: []AddEvidence{}, Removes: []Mark{}}
	for _, ev := range s.adds {
		snap.Adds = append(snap.Adds, ev)
	}
	for _, m := range s.removes {
		snap.Removes = append(snap.Removes, m)
	}
	sort.Slice(snap.Adds, func(i, j int) bool { return snap.Adds[i].Mark.key() < snap.Adds[j].Mark.key() })
	sort.Slice(snap.Removes, func(i, j int) bool { return snap.Removes[i].key() < snap.Removes[j].key() })
	return snap
}

// MergeSnapshot folds snap into local state. It is idempotent and
// order-independent. The whole merge is rejected if the same mark maps
// to two different tags. Local progress (NextSeq) never regresses.
func (s *Store) MergeSnapshot(snap Snapshot) error {
	for _, ev := range snap.Adds {
		if ev.Mark.Replica == "" || ev.Mark.Seq == 0 {
			return errors.New("invalid add evidence: empty mark")
		}
		if old, ok := s.adds[ev.Mark.key()]; ok && old.Tag != ev.Tag {
			return fmt.Errorf("conflicting tag for mark %s: %q vs %q", ev.Mark.key(), old.Tag, ev.Tag)
		}
	}
	seen := map[string]string{}
	for _, ev := range snap.Adds {
		if t, ok := seen[ev.Mark.key()]; ok && t != ev.Tag {
			return fmt.Errorf("conflicting tag for mark %s within snapshot", ev.Mark.key())
		}
		seen[ev.Mark.key()] = ev.Tag
	}
	for _, ev := range snap.Adds {
		s.adds[ev.Mark.key()] = ev
		if ev.Mark.Replica == s.Replica && ev.Mark.Seq >= s.NextSeq {
			s.NextSeq = ev.Mark.Seq + 1
		}
	}
	for _, m := range snap.Removes {
		s.removes[m.key()] = m
	}
	return nil
}
