package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RuleHit records a pattern that matched a path.
type RuleHit struct {
	Pattern Pattern
	Rel     string // path relative to the pattern's base dir
}

// Entry describes one audited filesystem item.
type Entry struct {
	Path     string   // slash-separated path relative to root
	Kind     string   // "file", "dir", "symlink", "junction", "other"
	Ignored  bool     // effective ignore decision
	Matched  *Pattern // last matching pattern, nil if none
	Ancestor bool     // true if excluded only because an ancestor dir was pruned
	PrunedBy *Pattern // pattern that pruned this directory (for pruned dirs)
}

// Report is the full audit result.
type Report struct {
	Root    string   `json:"root"`
	Kept    []string `json:"kept"`
	Ignored []string `json:"ignored"`
	Pruned  []string `json:"pruned_dirs"`
	Skipped []Skip   `json:"skipped"`
}

// Skip is a non-followed item (symlink, junction, unreadable, etc.).
type Skip struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// dirState carries the active rule set while descending.
type dirState struct {
	patterns []Pattern // ordered: ancestor rules first
}

// decide evaluates patterns in order; deeper files appended later win.
func decide(patterns []Pattern, rel string, isDir bool) (bool, *Pattern) {
	ignored := false
	var last *Pattern
	for i := range patterns {
		p := &patterns[i]
		// Pattern applies only to paths under its base directory.
		if p.Base != "" && rel != p.Base && !strings.HasPrefix(rel, p.Base+"/") {
			continue
		}
		sub := rel
		if p.Base != "" {
			sub = strings.TrimPrefix(rel, p.Base+"/")
		}
		if p.Match(sub, isDir) {
			ignored = !p.Negate
			last = p
		}
	}
	return ignored, last
}

// classify returns the kind string for a DirEntry.
func classify(de os.DirEntry) string {
	m := de.Type()
	if m&os.ModeSymlink != 0 {
		return "symlink"
	}
	if de.IsDir() {
		return "dir"
	}
	if m.IsRegular() {
		return "file"
	}
	return "other"
}

// Walk audits root read-only and returns the report plus per-entry
// decisions (used by Explain).
func Walk(root string) (*Report, []Entry, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	rep := &Report{Root: abs, Kept: []string{}, Ignored: []string{}, Pruned: []string{}, Skipped: []Skip{}}
	var entries []Entry

	var walkDir func(absDir, relDir string, st dirState) error
	walkDir = func(absDir, relDir string, st dirState) error {
		des, err := os.ReadDir(absDir)
		if err != nil {
			rel := relDir
			if rel == "" {
				rel = "."
			}
			rep.Skipped = append(rep.Skipped, Skip{Path: rel, Kind: "dir", Reason: "unreadable: " + err.Error()})
			return nil
		}
		// Load this directory's .gitignore first (rules apply to siblings too).
		st = dirState{patterns: append(append([]Pattern{}, st.patterns...), loadGitignore(absDir, relDir)...)}

		names := make([]string, 0, len(des))
		byName := map[string]os.DirEntry{}
		for _, de := range des {
			names = append(names, de.Name())
			byName[de.Name()] = de
		}
		sort.Strings(names)

		for _, name := range names {
			de := byName[name]
			rel := name
			if relDir != "" {
				rel = relDir + "/" + name
			}
			kind := classify(de)
			if kind == "dir" {
				if info, ierr := de.Info(); ierr == nil && isJunction(info) {
					kind = "junction"
				}
			}
			switch kind {
			case "symlink", "junction":
				rep.Skipped = append(rep.Skipped, Skip{Path: rel, Kind: kind, Reason: "link not followed"})
				entries = append(entries, Entry{Path: rel, Kind: kind})
				continue
			case "other":
				rep.Skipped = append(rep.Skipped, Skip{Path: rel, Kind: kind, Reason: "not a regular file"})
				entries = append(entries, Entry{Path: rel, Kind: kind})
				continue
			}
			isDir := kind == "dir"
			ignored, pat := decide(st.patterns, rel, isDir)
			e := Entry{Path: rel, Kind: kind, Ignored: ignored, Matched: pat}
			entries = append(entries, e)
			if isDir {
				if ignored {
					e.PrunedBy = pat
					entries[len(entries)-1] = e
					rep.Pruned = append(rep.Pruned, rel)
					continue // do not descend
				}
				if err := walkDir(filepath.Join(absDir, name), rel, st); err != nil {
					return err
				}
			} else {
				if ignored {
					rep.Ignored = append(rep.Ignored, rel)
				} else {
					rep.Kept = append(rep.Kept, rel)
				}
			}
		}
		return nil
	}

	if err := walkDir(abs, "", dirState{}); err != nil {
		return nil, nil, err
	}
	sort.Strings(rep.Kept)
	sort.Strings(rep.Ignored)
	sort.Strings(rep.Pruned)
	sort.Slice(rep.Skipped, func(i, j int) bool { return rep.Skipped[i].Path < rep.Skipped[j].Path })
	return rep, entries, nil
}

func loadGitignore(absDir, relDir string) []Pattern {
	data, err := os.ReadFile(filepath.Join(absDir, ".gitignore"))
	if err != nil {
		return nil
	}
	file := ".gitignore"
	if relDir != "" {
		file = relDir + "/.gitignore"
	}
	return ParsePatterns(relDir, file, string(data))
}
