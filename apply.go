package auditor

import (
	"fmt"
	"sort"
)

// LayerReport summarizes the effect of one applied layer.
type LayerReport struct {
	Layer   int      `json:"layer"`
	File    string   `json:"file"`
	Added   []string `json:"added"`
	Deleted []string `json:"deleted"`
	Changed []string `json:"changed"`
}

// Auditor holds the rebuilt filesystem state and accumulated history.
type Auditor struct {
	state   map[string]*Entry
	history map[string][]Event
	Reports []LayerReport
	// FailedLayer is the index of the layer that failed, or -1.
	FailedLayer int
	Err         string
}

// New returns an empty Auditor.
func New() *Auditor {
	return &Auditor{
		state:       map[string]*Entry{},
		history:     map[string][]Event{},
		FailedLayer: -1,
	}
}

// ApplyLayers applies each tar layer in order. On the first failure it
// keeps the last good state, records the error and stops.
func (a *Auditor) ApplyLayers(files []string) {
	for i, f := range files {
		if err := a.applyOne(i, f); err != nil {
			a.FailedLayer = i
			a.Err = err.Error()
			return
		}
	}
}

// applyOne validates and applies a single layer atomically.
func (a *Auditor) applyOne(idx int, file string) error {
	pl, err := parseLayer(file)
	if err != nil {
		return err
	}
	// Work on a copy so a failed layer leaves no partial state.
	next := make(map[string]*Entry, len(a.state))
	for k, v := range a.state {
		cp := *v
		next[k] = &cp
	}
	rep := LayerReport{Layer: idx, File: file}
	events := map[string][]Event{}
	addEv := func(p, action, detail string) {
		events[p] = append(events[p], Event{Layer: idx, Action: action, Detail: detail})
	}

	// Phase 1: whiteout deletions and opaque markers.
	deleted := map[string]bool{}
	removeTree := func(root string, reason string) {
		paths := []string{}
		if _, ok := next[root]; ok {
			paths = append(paths, root)
		}
		for p := range next {
			if isDescendantOf(p, root) {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			delete(next, p)
			if !deleted[p] {
				deleted[p] = true
				rep.Deleted = append(rep.Deleted, p)
				addEv(p, "deleted", reason)
			}
		}
	}
	for _, del := range sortedKeys(pl.deletes) {
		removeTree(del, "whiteout")
	}
	for _, dir := range sortedKeys(pl.opaque) {
		paths := []string{}
		for p := range next {
			if isDescendantOf(p, dir) {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			delete(next, p)
			if !deleted[p] {
				deleted[p] = true
				rep.Deleted = append(rep.Deleted, p)
				addEv(p, "deleted", "opaque directory "+dir)
			}
		}
	}

	// Phase 2: apply members parent-first.
	for _, m := range pl.members {
		// Existing non-directory parents must be explicitly replaced by
		// a directory from this layer; otherwise the layer is invalid.
		for par := parentOf(m.path); par != ""; par = parentOf(par) {
			if e, ok := next[par]; ok && e.Kind != KindDir {
				replaced := false
				for _, mm := range pl.members {
					if mm.path == par && mm.kind == KindDir {
						replaced = true
						break
					}
				}
				if !replaced {
					return fmt.Errorf("member %q requires non-directory parent %q to be replaced by an explicit directory", m.path, par)
				}
			}
		}
		// Fill missing parents with implicit directories.
		for par := parentOf(m.path); par != ""; par = parentOf(par) {
			if _, ok := next[par]; !ok {
				next[par] = &Entry{Kind: KindDir, Layer: idx, Implicit: true}
				rep.Added = append(rep.Added, par)
				addEv(par, "added", "implicit directory")
			}
		}
		old, existed := next[m.path]
		if existed && old.Kind == KindDir && m.kind != KindDir {
			// Replacing a directory removes its descendants.
			removeTree(m.path, "replaced by "+string(m.kind))
		}
		entry := &Entry{Kind: m.kind, Layer: idx, SHA256: m.sha256, Target: m.target}
		switch {
		case !existed:
			rep.Added = append(rep.Added, m.path)
			addEv(m.path, "added", string(m.kind))
		case old.Kind != m.kind:
			rep.Changed = append(rep.Changed, m.path)
			addEv(m.path, "changed", fmt.Sprintf("type %s -> %s", old.Kind, m.kind))
		case m.kind == KindFile && old.SHA256 != m.sha256:
			rep.Changed = append(rep.Changed, m.path)
			addEv(m.path, "changed", "content")
		case m.kind == KindSymlink && old.Target != m.target:
			rep.Changed = append(rep.Changed, m.path)
			addEv(m.path, "changed", "target")
		default:
			// Identical content: only the source layer advances.
			addEv(m.path, "rewritten", "identical content")
		}
		next[m.path] = entry
	}

	sort.Strings(rep.Added)
	sort.Strings(rep.Deleted)
	sort.Strings(rep.Changed)
	a.state = next
	for p, evs := range events {
		a.history[p] = append(a.history[p], evs...)
	}
	a.Reports = append(a.Reports, rep)
	return nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
