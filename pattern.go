package main

import (
	"strings"
)

// Pattern is a single parsed gitignore rule.
type Pattern struct {
	File     string // source .gitignore path (relative to root, slash-separated)
	Line     int    // 1-based line number in the source file
	Raw      string // original line text (after trailing-space trim)
	Negate   bool   // leading '!'
	DirOnly  bool   // trailing '/'
	Anchored bool   // contains a '/' (other than the trailing dir marker)
	Base     string // directory (slash-separated, relative to root) the rule file lives in; "" for root
	Segments []string
}

// ParsePatterns parses the lines of one .gitignore file located at base.
func ParsePatterns(base, file, content string) []Pattern {
	var out []Pattern
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		raw = strings.TrimSuffix(raw, "\r")
		p, ok := parseLine(raw)
		if !ok {
			continue
		}
		p.File = file
		p.Line = i + 1
		p.Base = base
		out = append(out, p)
	}
	return out
}

func parseLine(raw string) (Pattern, bool) {
	var p Pattern
	// Trim unescaped trailing spaces.
	for strings.HasSuffix(raw, " ") && !strings.HasSuffix(raw, "\\ ") {
		raw = strings.TrimSuffix(raw, " ")
	}
	raw = strings.ReplaceAll(raw, "\\ ", " ")
	if raw == "" {
		return p, false
	}
	// Comment handling: unescaped leading '#' is a comment.
	if strings.HasPrefix(raw, "#") {
		return p, false
	}
	if strings.HasPrefix(raw, "\\#") {
		raw = raw[1:]
	}
	if strings.HasPrefix(raw, "\\!") {
		raw = raw[1:]
	} else if strings.HasPrefix(raw, "!") {
		p.Negate = true
		raw = raw[1:]
	}
	if raw == "" {
		return p, false
	}
	p.Raw = raw
	if strings.HasSuffix(raw, "/") {
		p.DirOnly = true
		raw = strings.TrimSuffix(raw, "/")
	}
	if strings.HasPrefix(raw, "/") {
		p.Anchored = true
		raw = strings.TrimPrefix(raw, "/")
	} else if strings.Contains(raw, "/") {
		p.Anchored = true
	}
	if raw == "" {
		return p, false
	}
	p.Segments = strings.Split(raw, "/")
	return p, true
}

// Match reports whether the pattern matches the given path.
// rel is the slash-separated path relative to the pattern's base directory.
// isDir indicates whether the path is a directory.
func (p Pattern) Match(rel string, isDir bool) bool {
	if p.DirOnly && !isDir {
		return false
	}
	segs := strings.Split(rel, "/")
	if p.Anchored {
		return matchSegments(p.Segments, segs)
	}
	// Unanchored: match against any trailing subsequence.
	for start := 0; start < len(segs); start++ {
		if matchSegments(p.Segments, segs[start:]) {
			return true
		}
	}
	return false
}

// matchSegments matches pattern segments against path segments,
// supporting "**" per git rules.
func matchSegments(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	if pat[0] == "**" {
		// "**" matches zero or more whole segments.
		for skip := 0; skip <= len(path); skip++ {
			if matchSegments(pat[1:], path[skip:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if !matchSegment(pat[0], path[0]) {
		return false
	}
	return matchSegments(pat[1:], path[1:])
}

// matchSegment matches a single path segment against a glob with
// '*', '?' and '[...]' character classes. Case-sensitive always.
func matchSegment(pat, name string) bool {
	return globMatch(pat, name)
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
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(pat, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		case '[':
			if len(s) == 0 {
				return false
			}
			cls, rest, ok := parseClass(pat)
			if !ok {
				// Literal '[' if class is malformed.
				if s[0] != '[' {
					return false
				}
				pat = pat[1:]
				s = s[1:]
				continue
			}
			if !classMatch(cls, s[0]) {
				return false
			}
			pat = rest
			s = s[1:]
		case '\\':
			if len(pat) > 1 {
				pat = pat[1:]
				c = pat[0]
			}
			if len(s) == 0 || s[0] != c {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		default:
			if len(s) == 0 || s[0] != c {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}

// parseClass parses "[...]" at the start of pat, returning the class
// body and the remainder of the pattern.
func parseClass(pat string) (body, rest string, ok bool) {
	i := 1
	if i < len(pat) && (pat[i] == '!' || pat[i] == '^') {
		i++
	}
	if i < len(pat) && pat[i] == ']' {
		i++ // literal ']' as first char
	}
	start := i
	for i < len(pat) && pat[i] != ']' {
		i++
	}
	if i >= len(pat) || i == start {
		return "", "", false
	}
	return pat[1:i], pat[i+1:], true
}

func classMatch(cls string, c byte) bool {
	negate := false
	if strings.HasPrefix(cls, "!") || strings.HasPrefix(cls, "^") {
		negate = true
		cls = cls[1:]
	}
	matched := false
	for i := 0; i < len(cls); i++ {
		if i+2 < len(cls) && cls[i+1] == '-' {
			if c >= cls[i] && c <= cls[i+2] {
				matched = true
			}
			i += 2
		} else if cls[i] == c {
			matched = true
		}
	}
	return matched != negate
}
