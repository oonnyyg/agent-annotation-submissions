// Package ignore implements hierarchical .gitignore parsing and matching
// using only the Go standard library. Matching is always case-sensitive.
package ignore

import (
	"strings"
)

// Pattern is a single parsed .gitignore rule line.
type Pattern struct {
	Raw      string // original line text (after trailing-space trimming)
	Negate   bool   // line began with '!'
	DirOnly  bool   // line ended with '/'
	Anchored bool   // pattern contains a '/' (other than trailing), relative to the rule dir
	Body     string // pattern text without leading '!' or trailing '/'
	File     string // source .gitignore path (relative to root, slash-separated)
	Line     int    // 1-based line number in the source file
}

// ParseLine parses one raw .gitignore line. ok is false for blank lines
// and comments, which carry no rule.
func ParseLine(raw, file string, line int) (Pattern, bool) {
	// Trailing spaces are ignored unless escaped with a backslash.
	raw = trimTrailingSpace(raw)
	if raw == "" {
		return Pattern{}, false
	}
	if raw[0] == '#' {
		return Pattern{}, false // comment
	}
	p := Pattern{Raw: raw, File: file, Line: line}
	if raw[0] == '!' {
		p.Negate = true
		raw = raw[1:]
	}
	// Escaped leading '#' or '!' is a literal character.
	if strings.HasPrefix(raw, `\#`) || strings.HasPrefix(raw, `\!`) {
		raw = raw[1:]
	}
	if raw == "" {
		return Pattern{}, false
	}
	if strings.HasSuffix(raw, "/") {
		p.DirOnly = true
		raw = strings.TrimSuffix(raw, "/")
	}
	if strings.HasPrefix(raw, "/") {
		p.Anchored = true
		raw = raw[1:]
	} else if strings.Contains(raw, "/") {
		p.Anchored = true
	}
	if raw == "" {
		return Pattern{}, false
	}
	p.Body = raw
	return p, true
}

// trimTrailingSpace removes unescaped trailing spaces, keeping escaped ones
// as literal spaces (the backslash is consumed).
func trimTrailingSpace(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		// Count preceding backslashes; odd count means the space is escaped.
		n := 0
		for i := len(s) - 2; i >= 0 && s[i] == '\\'; i-- {
			n++
		}
		if n%2 == 1 {
			// "\ " -> literal trailing space; drop the backslash.
			return s[:len(s)-2] + " "
		}
		s = s[:len(s)-1]
	}
	return s
}

// Match reports whether the pattern matches the given path.
// rel is the slash-separated path relative to the directory containing the
// rule's .gitignore file. isDir reports whether rel names a directory.
func (p Pattern) Match(rel string, isDir bool) bool {
	if p.DirOnly && !isDir {
		return false
	}
	if p.Anchored {
		return matchPath(p.Body, rel)
	}
	// No slash: match the basename at any depth below the rule directory.
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	return matchName(p.Body, base)
}

// matchPath matches a full (anchored) path pattern against rel, supporting
// '**' segments in addition to '*', '?' and character classes.
func matchPath(pattern, path string) bool {
	psegs := strings.Split(pattern, "/")
	segs := strings.Split(path, "/")
	return matchSegs(psegs, segs)
}

func matchSegs(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		// '**' matches zero or more whole segments.
		for skip := 0; skip <= len(segs); skip++ {
			if matchSegs(pat[1:], segs[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if !matchName(pat[0], segs[0]) {
		return false
	}
	return matchSegs(pat[1:], segs[1:])
}

// matchName matches a single path segment against a glob supporting
// '*', '?', '[...]' character classes and backslash escapes.
// It never crosses '/'; matching is case-sensitive.
func matchName(pattern, name string) bool {
	return globMatch(pattern, name)
}

func globMatch(pat, s string) bool {
	for len(pat) > 0 {
		c := pat[0]
		switch c {
		case '*':
			// Collapse consecutive stars.
			for len(pat) > 0 && pat[0] == '*' {
				pat = pat[1:]
			}
			if pat == "" {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(pat, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if s == "" {
				return false
			}
			pat, s = pat[1:], s[1:]
		case '[':
			if s == "" {
				return false
			}
			matched, rest, ok := matchClass(pat, s[0])
			if !ok {
				// Unterminated class: treat '[' literally.
				if s[0] != '[' {
					return false
				}
				pat, s = pat[1:], s[1:]
				continue
			}
			if !matched {
				return false
			}
			pat, s = rest, s[1:]
		case '\\':
			if len(pat) > 1 {
				pat = pat[1:]
				c = pat[0]
			}
			if s == "" || s[0] != c {
				return false
			}
			pat, s = pat[1:], s[1:]
		default:
			if s == "" || s[0] != c {
				return false
			}
			pat, s = pat[1:], s[1:]
		}
	}
	return s == ""
}

// matchClass parses a '[...]' class at the start of pat and reports whether
// it matches byte c. rest is the remainder of pat after the class.
func matchClass(pat string, c byte) (matched bool, rest string, ok bool) {
	i := 1
	negate := false
	if i < len(pat) && (pat[i] == '!' || pat[i] == '^') {
		negate = true
		i++
	}
	matched = false
	first := true
	for i < len(pat) {
		if pat[i] == ']' && !first {
			return matched != negate, pat[i+1:], true
		}
		first = false
		var lo byte
		if pat[i] == '\\' && i+1 < len(pat) {
			lo = pat[i+1]
			i += 2
		} else {
			lo = pat[i]
			i++
		}
		if i+1 < len(pat) && pat[i] == '-' && pat[i+1] != ']' {
			hi := pat[i+1]
			i += 2
			if lo <= c && c <= hi {
				matched = true
			}
		} else if c == lo {
			matched = true
		}
	}
	return false, "", false // unterminated class
}
