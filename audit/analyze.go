package audit

import (
	"fmt"
	"sort"
)

// ManifestEntry is one row of the final, deterministically sorted manifest.
type ManifestEntry struct {
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	SHA256 string `json:"sha256,omitempty"`
	Target string `json:"target,omitempty"`
	Layer  int    `json:"layer"`
}

// Result is the full output of an analysis run.
type Result struct {
	Layers   []LayerReport    `json:"layers"`
	Manifest []ManifestEntry  `json:"manifest"`
	Stopped  bool             `json:"stopped"` // true if a layer failed
	Error    string           `json:"error,omitempty"`
}

// Analyze applies the given tar layer files in order. On the first
// failing layer it stops, keeping the state of the last successful
// layer, and records the error in the result.
func Analyze(files []string) (*Result, error) {
	eng := NewEngine()
	res := &Result{}
	for i, f := range files {
		pl, err := ParseLayerFile(f)
		if err != nil {
			res.Stopped = true
			res.Error = fmt.Sprintf("layer %d (%s): %v", i, f, err)
			break
		}
		if err := eng.ApplyLayer(i, f, pl); err != nil {
			res.Stopped = true
			res.Error = err.Error()
			break
		}
	}
	res.Layers = eng.Reports
	res.Manifest = Manifest(eng.State())
	return res, nil
}

// Manifest renders the state as a path-sorted list.
func Manifest(state map[string]*Node) []ManifestEntry {
	paths := make([]string, 0, len(state))
	for p := range state {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]ManifestEntry, 0, len(paths))
	for _, p := range paths {
		n := state[p]
		out = append(out, ManifestEntry{Path: p, Kind: n.Kind, SHA256: n.SHA256, Target: n.Target, Layer: n.Layer})
	}
	return out
}

// PathHistory returns the recorded events for one normalized path.
func PathHistory(files []string, query string) ([]Event, error) {
	q, err := NormalizePath(query)
	if err != nil {
		return nil, err
	}
	eng := NewEngine()
	for i, f := range files {
		pl, err := ParseLayerFile(f)
		if err != nil {
			return nil, fmt.Errorf("layer %d (%s): %w", i, f, err)
		}
		if err := eng.ApplyLayer(i, f, pl); err != nil {
			return nil, err
		}
	}
	return eng.History[q], nil
}
