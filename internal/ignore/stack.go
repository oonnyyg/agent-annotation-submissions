package ignore

import (
	"bufio"
	"io"
	"strings"
)

// Level holds the rules of one .gitignore file. Dir is the slash-separated
// directory (relative to the root, "" for the root) the rules apply to.
type Level struct {
	Dir      string
	Patterns []Pattern
}

// ParseFile parses .gitignore content from r. dir is the rule directory
// relative to the root ("" for root); file is the display path of the file.
func ParseFile(r io.Reader, dir, file string) Level {
	lv := Level{Dir: dir}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		if p, ok := ParseLine(sc.Text(), file, line); ok {
			lv.Patterns = append(lv.Patterns, p)
		}
	}
	return lv
}

// Stack is the ordered set of rule levels from the root down to the
// directory currently being evaluated. Deeper levels override ancestors;
// within one level the last matching pattern wins.
type Stack struct {
	Levels []Level
}

// Push returns a new Stack with the given level appended.
func (s Stack) Push(lv Level) Stack {
	out := Stack{Levels: make([]Level, len(s.Levels)+1)}
	copy(out.Levels, s.Levels)
	out.Levels[len(s.Levels)] = lv
	return out
}

// Decision explains why a path is ignored or kept.
type Decision struct {
	Ignored bool
	// Pattern is the winning rule; nil when nothing matched.
	Pattern *Pattern
}

// Decide evaluates rel (slash-separated, relative to the root) against the
// stack. isDir reports whether rel names a directory.
func (s Stack) Decide(rel string, isDir bool) Decision {
	var win *Pattern
	for li := range s.Levels {
		lv := &s.Levels[li]
		sub := rel
		if lv.Dir != "" {
			if rel == lv.Dir || !strings.HasPrefix(rel, lv.Dir+"/") {
				continue
			}
			sub = rel[len(lv.Dir)+1:]
		}
		for pi := range lv.Patterns {
			p := &lv.Patterns[pi]
			if p.Match(sub, isDir) {
				win = p // last match within reachable levels wins
			}
		}
	}
	if win == nil {
		return Decision{}
	}
	return Decision{Ignored: !win.Negate, Pattern: win}
}
