package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Explanation describes why a path is kept or excluded.
type Explanation struct {
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	Decision  string   `json:"decision"` // "kept", "excluded", "missing"
	Reason    string   `json:"reason"`   // "direct", "ancestor", "pruned-dir", "skipped", "none"
	MatchedBy *RuleRef `json:"matched_by,omitempty"`
	BlockedBy *RuleRef `json:"blocked_by,omitempty"`
	BlockedAt string   `json:"blocked_at,omitempty"`
}

// RuleRef identifies a rule by file and line.
type RuleRef struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Pattern string `json:"pattern"`
	Negate  bool   `json:"negate"`
}

func ruleRef(p *Pattern) *RuleRef {
	if p == nil {
		return nil
	}
	return &RuleRef{File: p.File, Line: p.Line, Pattern: p.Raw, Negate: p.Negate}
}

// Explain reports why relPath (slash- or OS-separated, relative to root)
// is kept or excluded, using the walk results.
func Explain(root, relPath string, entries []Entry) (*Explanation, error) {
	rel := strings.ReplaceAll(filepath.ToSlash(relPath), "\\", "/")
	rel = strings.Trim(rel, "/")
	if rel == "" || rel == "." {
		return nil, fmt.Errorf("path must name an item inside the root, not the root itself")
	}
	if strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("path %q escapes the root directory", relPath)
	}

	byPath := map[string]Entry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	ex := &Explanation{Path: rel}
	if e, ok := byPath[rel]; ok {
		ex.Exists = true
		switch e.Kind {
		case "symlink", "junction", "other":
			ex.Decision = "excluded"
			ex.Reason = "skipped"
			return ex, nil
		}
		if e.Kind == "dir" && e.PrunedBy != nil {
			ex.Decision = "excluded"
			ex.Reason = "pruned-dir"
			ex.MatchedBy = ruleRef(e.PrunedBy)
			return ex, nil
		}
		if e.Ignored {
			ex.Decision = "excluded"
			ex.Reason = "direct"
			ex.MatchedBy = ruleRef(e.Matched)
		} else {
			ex.Decision = "kept"
			ex.Reason = "direct"
			ex.MatchedBy = ruleRef(e.Matched) // may be a negation hit or nil
		}
		return ex, nil
	}

	// Not visited: either blocked by a pruned ancestor or missing on disk.
	anc := rel
	for {
		idx := strings.LastIndex(anc, "/")
		if idx < 0 {
			break
		}
		anc = anc[:idx]
		if e, ok := byPath[anc]; ok && e.PrunedBy != nil {
			ex.Exists = diskExists(root, rel)
			ex.Decision = "excluded"
			ex.Reason = "ancestor"
			ex.BlockedAt = anc
			ex.BlockedBy = ruleRef(e.PrunedBy)
			return ex, nil
		}
	}
	ex.Decision = "missing"
	ex.Reason = "none"
	return ex, nil
}

func diskExists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
