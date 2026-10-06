package audit

import (
	"fmt"
	"sort"
	"strings"
)

// Event describes one change to a path, recorded for history queries.
type Event struct {
	Layer  int    `json:"layer"`
	Action string `json:"action"` // added, deleted, changed, rewritten
	Kind   Kind   `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// LayerReport summarizes the effect of one applied layer.
type LayerReport struct {
	Layer     int      `json:"layer"`
	File      string   `json:"file"`
	Added     []string `json:"added"`
	Deleted   []string `json:"deleted"`
	Changed   []string `json:"changed"`   // content or type change
	Rewritten []string `json:"rewritten"` // identical content, source layer updated
}

// Engine holds the reconstructed view after the last successful layer.
type Engine struct {
	state   map[string]*Node
	History map[string][]Event
	Reports []LayerReport
}

// NewEngine creates an empty engine.
func NewEngine() *Engine {
	return &Engine{state: map[string]*Node{}, History: map[string][]Event{}}
}

// State returns the current reconstructed view (do not mutate).
func (e *Engine) State() map[string]*Node { return e.state }

// ApplyLayer validates and applies one parsed layer atomically: on any
// error the previous state is kept untouched.
func (e *Engine) ApplyLayer(layerIdx int, file string, pl *parsedLayer) error {
	next := make(map[string]*Node, len(e.state))
	for k, v := range e.state {
		cp := *v
		next[k] = &cp
	}
	rep := LayerReport{
		Layer: layerIdx, File: file,
		Added: []string{}, Deleted: []string{}, Changed: []string{}, Rewritten: []string{},
	}
	events := map[string][]Event{}
	record := func(p, action string, k Kind, detail string) {
		events[p] = append(events[p], Event{Layer: layerIdx, Action: action, Kind: k, Detail: detail})
	}

	// Phase 1: deletions and opaque markers affect lower layers only.
	deleteSubtree := func(root string) {
		var doomed []string
		for p := range next {
			if p == root || IsDescendantOf(p, root) {
				doomed = append(doomed, p)
			}
		}
		sort.Strings(doomed)
		for _, p := range doomed {
			delete(next, p)
			rep.Deleted = append(rep.Deleted, p)
			record(p, "deleted", "", "")
		}
	}
	for _, d := range pl.deletions {
		deleteSubtree(d.path)
	}
	for _, dir := range pl.opaque {
		var doomed []string
		for p := range next {
			if dir == "" || IsDescendantOf(p, dir) {
				doomed = append(doomed, p)
			}
		}
		sort.Strings(doomed)
		for _, p := range doomed {
			delete(next, p)
			rep.Deleted = append(rep.Deleted, p)
			record(p, "deleted", "", "opaque directory")
		}
	}

	// Phase 2: apply members parent-before-child; tar order is irrelevant.
	members := append([]member(nil), pl.members...)
	sort.Slice(members, func(i, j int) bool {
		di, dj := strings.Count(members[i].path, "/"), strings.Count(members[j].path, "/")
		if di != dj {
			return di < dj
		}
		return members[i].path < members[j].path
	})
	inLayer := map[string]Kind{}
	for _, m := range members {
		inLayer[m.path] = m.kind
	}
	for _, m := range members {
		// A non-directory member of this layer must not have descendants
		// in the same layer.
		if m.kind != KindDir {
			for _, other := range members {
				if IsDescendantOf(other.path, m.path) {
					return fmt.Errorf("layer %d: %q is not a directory but has descendant %q", layerIdx, m.path, other.path)
				}
			}
		}
		// Ensure parents exist; a missing parent becomes an implicit
		// directory, an existing non-directory parent must be explicitly
		// replaced by a directory from this layer.
		for parent := ParentOf(m.path); parent != ""; parent = ParentOf(parent) {
			if n, ok := next[parent]; ok {
				if n.Kind != KindDir {
					if k, ok := inLayer[parent]; !ok || k != KindDir {
						return fmt.Errorf("layer %d: parent %q of %q is a %s and is not replaced by an explicit directory", layerIdx, parent, m.path, n.Kind)
					}
				}
				break // ancestors above an existing node already exist
			}
			if _, ok := inLayer[parent]; !ok {
				next[parent] = &Node{Kind: KindDir, Layer: layerIdx}
				rep.Added = append(rep.Added, parent)
				record(parent, "added", KindDir, "implicit directory")
			}
		}

		old, existed := next[m.path]
		if existed && old.Kind == KindDir && m.kind != KindDir {
			// Replacing a directory removes its descendants.
			var doomed []string
			for p := range next {
				if IsDescendantOf(p, m.path) {
					doomed = append(doomed, p)
				}
			}
			sort.Strings(doomed)
			for _, p := range doomed {
				delete(next, p)
				rep.Deleted = append(rep.Deleted, p)
				record(p, "deleted", "", "parent replaced by non-directory")
			}
		}

		node := &Node{Kind: m.kind, Layer: layerIdx, SHA256: m.sha256, Target: m.target}
		next[m.path] = node
		switch {
		case !existed:
			rep.Added = append(rep.Added, m.path)
			record(m.path, "added", m.kind, "")
		case old.Kind != m.kind || old.SHA256 != m.sha256 || old.Target != m.target:
			rep.Changed = append(rep.Changed, m.path)
			record(m.path, "changed", m.kind, fmt.Sprintf("was %s", old.Kind))
		default:
			rep.Rewritten = append(rep.Rewritten, m.path)
			record(m.path, "rewritten", m.kind, "identical content, source updated")
		}
	}

	sort.Strings(rep.Added)
	sort.Strings(rep.Deleted)
	sort.Strings(rep.Changed)
	sort.Strings(rep.Rewritten)
	e.state = next
	for p, evs := range events {
		e.History[p] = append(e.History[p], evs...)
	}
	e.Reports = append(e.Reports, rep)
	return nil
}
