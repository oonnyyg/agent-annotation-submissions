// Package audit walks a directory tree read-only, applies hierarchical
// .gitignore rules, and produces a traceable report. It never reads the
// Git index, global config, or invokes Git, and it never follows symlinks
// or directory junctions.
package audit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/hierarchical-ignore-auditor/internal/ignore"
)

// Entry is one recorded item in the report.
type Entry struct {
	Path   string `json:"path"`             // slash-separated, relative to root
	Reason string `json:"reason"`           // human-readable explanation
	File   string `json:"file,omitempty"`   // deciding .gitignore (relative path)
	Line   int    `json:"line,omitempty"`   // deciding rule line number
	Rule   string `json:"rule,omitempty"`   // deciding rule text
	Negate bool   `json:"negate,omitempty"` // deciding rule was a negation
}

// Report is the stable, sorted audit result.
type Report struct {
	Root    string  `json:"root"`
	Kept    []Entry `json:"kept"`    // regular files that survive filtering
	Pruned  []Entry `json:"pruned"`  // directories ignored and not descended into
	Skipped []Entry `json:"skipped"` // symlinks, junctions, non-regular files
}

// Walk traverses root read-only and returns the audit report. Every regular
// file is treated as untracked. Symlinks and junctions are recorded in
// Skipped and never followed, so traversal cannot escape root.
func Walk(root string) (*Report, error) {
	rep := &Report{Root: filepath.Clean(root)}
	err := walkDir(root, "", ignore.Stack{}, rep)
	if err != nil {
		return nil, err
	}
	sortEntries(rep.Kept)
	sortEntries(rep.Pruned)
	sortEntries(rep.Skipped)
	return rep, nil
}

func sortEntries(es []Entry) {
	sort.Slice(es, func(i, j int) bool { return es[i].Path < es[j].Path })
}

func walkDir(abs, rel string, st ignore.Stack, rep *Report) error {
	// Load this directory's .gitignore, if present and readable.
	igPath := filepath.Join(abs, ".gitignore")
	if f, err := os.Open(igPath); err == nil {
		lv := ignore.ParseFile(f, rel, displayPath(rel, ".gitignore"))
		f.Close()
		if len(lv.Patterns) > 0 {
			st = st.Push(lv)
		}
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return err
	}
	for _, de := range entries {
		name := de.Name()
		childRel := displayPath(rel, name)
		childAbs := filepath.Join(abs, name)

		// Never follow symlinks or junctions; record them instead.
		if de.Type()&os.ModeSymlink != 0 {
			rep.Skipped = append(rep.Skipped, Entry{
				Path:   childRel,
				Reason: "symbolic link or junction; not followed",
			})
			continue
		}

		info, err := de.Info()
		if err != nil {
			rep.Skipped = append(rep.Skipped, Entry{
				Path:   childRel,
				Reason: "stat failed: " + err.Error(),
			})
			continue
		}

		if info.IsDir() {
			d := st.Decide(childRel, true)
			if d.Ignored {
				rep.Pruned = append(rep.Pruned, Entry{
					Path:   childRel,
					Reason: "directory ignored; subtree pruned",
					File:   d.Pattern.File,
					Line:   d.Pattern.Line,
					Rule:   d.Pattern.Raw,
				})
				continue // do not descend; negations inside cannot revive it
			}
			if err := walkDir(childAbs, childRel, st, rep); err != nil {
				return err
			}
			continue
		}

		if !info.Mode().IsRegular() {
			rep.Skipped = append(rep.Skipped, Entry{
				Path:   childRel,
				Reason: "not a regular file (mode " + info.Mode().String() + ")",
			})
			continue
		}

		d := st.Decide(childRel, false)
		if d.Ignored {
			continue // excluded file: omitted from kept list
		}
		e := Entry{Path: childRel}
		if d.Pattern != nil {
			e.Reason = "kept by negation rule"
			e.File = d.Pattern.File
			e.Line = d.Pattern.Line
			e.Rule = d.Pattern.Raw
			e.Negate = true
		} else {
			e.Reason = "no matching ignore rule"
		}
		rep.Kept = append(rep.Kept, e)
	}
	return nil
}

func displayPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// toSlash normalizes a user-supplied relative path for querying.
func toSlash(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return strings.Trim(p, "/")
}
