package auditor

import "sort"

// Manifest is the deterministic final filesystem view.
type Manifest struct {
	// Paths lists every path in sorted order.
	Paths []string `json:"paths"`
	// Entries maps each path to its rebuilt entry.
	Entries map[string]*Entry `json:"entries"`
}

// Manifest returns the final view with paths in fixed sorted order.
func (a *Auditor) Manifest() Manifest {
	m := Manifest{Paths: []string{}, Entries: map[string]*Entry{}}
	for p, e := range a.state {
		m.Paths = append(m.Paths, p)
		m.Entries[p] = e
	}
	sort.Strings(m.Paths)
	return m
}

// History returns the recorded events for a path, or nil if unknown.
func (a *Auditor) History(p string) []Event {
	return a.history[p]
}
