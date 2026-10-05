package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Target is one build target from the JSON manifest.
type Target struct {
	Name    string   `json:"-"`
	Deps    []string `json:"deps"`
	Command []string `json:"command"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`
}

// Manifest is the parsed and validated build manifest.
type Manifest struct {
	Targets map[string]*Target
	Order   []string // sorted target names
}

// LoadManifest reads and validates the manifest file. root is the work root.
func LoadManifest(path, root string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var raw struct {
		Targets map[string]*Target `json:"targets"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if len(raw.Targets) == 0 {
		return nil, fmt.Errorf("manifest has no targets")
	}
	m := &Manifest{Targets: raw.Targets}
	for name, t := range m.Targets {
		t.Name = name
		if len(t.Command) == 0 {
			return nil, fmt.Errorf("target %q: command must be a non-empty argument array", name)
		}
		if len(t.Outputs) == 0 {
			return nil, fmt.Errorf("target %q: must declare at least one output", name)
		}
		for _, p := range append(append([]string{}, t.Inputs...), t.Outputs...) {
			if err := checkPath(root, p); err != nil {
				return nil, fmt.Errorf("target %q: %v", name, err)
			}
		}
	}
	if err := m.checkOutputOwnership(); err != nil {
		return nil, err
	}
	if err := m.checkInputDeps(); err != nil {
		return nil, err
	}
	for name := range m.Targets {
		m.Order = append(m.Order, name)
	}
	sort.Strings(m.Order)
	return m, nil
}

// checkPath rejects absolute paths, escapes from root, and symlinks.
func checkPath(root, p string) error {
	if p == "" || filepath.IsAbs(p) {
		return fmt.Errorf("path %q must be relative", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes the work root", p)
	}
	abs := filepath.Join(root, clean)
	// Reject symlinks in every existing component.
	cur := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return fmt.Errorf("path %q: %v", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q contains a symlink, which is not allowed", p)
		}
	}
	_ = abs
	return nil
}

// checkOutputOwnership ensures each output file belongs to exactly one target.
func (m *Manifest) checkOutputOwnership() error {
	owner := map[string]string{}
	for _, name := range m.OrderNames() {
		for _, out := range m.Targets[name].Outputs {
			c := filepath.Clean(out)
			if prev, ok := owner[c]; ok {
				return fmt.Errorf("output %q is declared by both %q and %q", out, prev, name)
			}
			owner[c] = name
		}
	}
	return nil
}

// checkInputDeps ensures inputs produced by other targets declare that dependency.
func (m *Manifest) checkInputDeps() error {
	producer := map[string]string{}
	for _, name := range m.OrderNames() {
		for _, out := range m.Targets[name].Outputs {
			producer[filepath.Clean(out)] = name
		}
	}
	for _, name := range m.OrderNames() {
		t := m.Targets[name]
		deps := map[string]bool{}
		for _, d := range t.Deps {
			deps[d] = true
		}
		for _, in := range t.Inputs {
			if src, ok := producer[filepath.Clean(in)]; ok && src != name && !deps[src] {
				return fmt.Errorf("target %q reads output %q of target %q without declaring it as a dependency", name, in, src)
			}
		}
	}
	return nil
}

func (m *Manifest) OrderNames() []string {
	names := make([]string, 0, len(m.Targets))
	for n := range m.Targets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
