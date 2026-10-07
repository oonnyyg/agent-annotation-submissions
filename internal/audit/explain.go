package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/hierarchical-ignore-auditor/internal/ignore"
)

// Explanation describes why a queried path is kept or excluded.
type Explanation struct {
	Path    string `json:"path"`
	Ignored bool   `json:"ignored"`
	// Kind is "direct" (a rule matched the path itself), "ancestor"
	// (an ancestor directory was ignored, blocking descent), or "none".
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Rule   string `json:"rule,omitempty"`
	// BlockedBy is the ancestor directory that pruned traversal, if any.
	BlockedBy string `json:"blockedBy,omitempty"`
}

// Explain reports why the relative path rel inside root is kept or
// excluded. It replays traversal from the root, so a path beneath an
// ignored directory is reported as ancestor-blocked even if a negation
// rule somewhere would otherwise match it.
func Explain(root, rel string) (*Explanation, error) {
	rel = toSlash(rel)
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return nil, fmt.Errorf("path %q is outside the root", rel)
	}
	segs := strings.Split(rel, "/")
	st := ignore.Stack{}
	abs := filepath.Clean(root)

	for i, seg := range segs {
		// Load .gitignore of the current directory before testing children.
		igPath := filepath.Join(abs, ".gitignore")
		if f, err := os.Open(igPath); err == nil {
			dir := strings.Join(segs[:i], "/")
			lv := ignore.ParseFile(f, dir, displayPath(dir, ".gitignore"))
			f.Close()
			if len(lv.Patterns) > 0 {
				st = st.Push(lv)
			}
		}

		cur := strings.Join(segs[:i+1], "/")
		last := i == len(segs)-1
		isDir := !last || dirExists(filepath.Join(abs, seg))

		d := st.Decide(cur, isDir)
		if d.Ignored {
			ex := &Explanation{
				Path:    rel,
				Ignored: true,
				File:    d.Pattern.File,
				Line:    d.Pattern.Line,
				Rule:    d.Pattern.Raw,
			}
			if last {
				ex.Kind = "direct"
				ex.Reason = fmt.Sprintf("matched ignore rule %q (%s:%d)",
					d.Pattern.Raw, d.Pattern.File, d.Pattern.Line)
			} else {
				ex.Kind = "ancestor"
				ex.BlockedBy = cur
				ex.Reason = fmt.Sprintf(
					"ancestor directory %q ignored by %q (%s:%d); traversal stops there, so nested negation rules cannot revive it",
					cur, d.Pattern.Raw, d.Pattern.File, d.Pattern.Line)
			}
			return ex, nil
		}
		abs = filepath.Join(abs, seg)
	}

	return &Explanation{
		Path:   rel,
		Kind:   "none",
		Reason: "no ignore rule matched; path is kept",
	}, nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
