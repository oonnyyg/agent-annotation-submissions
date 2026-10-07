// Package copybook parses a small, strict subset of COBOL copybook record
// definitions and compiles them into a fixed-length record layout.
package copybook

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind describes the storage class of an elementary field.
type Kind int

const (
	KindText    Kind = iota // PIC X(n), ASCII
	KindDisplay             // PIC 9(n)V9(m), unsigned zoned decimal
	KindComp3               // PIC S9(n)V9(m) COMP-3, signed packed decimal
)

// Node is one entry in the parsed record definition tree.
type Node struct {
	Name     string
	Level    int
	Kind     Kind // only meaningful when Group is false
	Group    bool
	Digits   int // integer digits
	Scale    int // implied decimal places
	Occurs   int // 1 when not an array
	Children []*Node
	Line     int // 1-based source line for diagnostics
}

// Parse parses a free-format copybook containing exactly one top-level
// (level 01) record definition.
func Parse(src string) (*Node, error) {
	entries, err := lexEntries(src)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("copybook: empty definition")
	}
	if entries[0].level != 1 {
		return nil, fmt.Errorf("copybook: line %d: record must start at level 01", entries[0].line)
	}
	root, next, err := buildTree(entries, 0)
	if err != nil {
		return nil, err
	}
	if next != len(entries) {
		return nil, fmt.Errorf("copybook: line %d: unexpected entry after end of record", entries[next].line)
	}
	if err := checkNames(root); err != nil {
		return nil, err
	}
	return root, nil
}

type entry struct {
	level  int
	name   string
	clause string
	line   int
}

var entryRe = regexp.MustCompile(`^\s*(\d{1,2})\s+([A-Za-z][A-Za-z0-9-]*)\s*(.*?)\s*$`)

// lexEntries splits the source into level/name/rest entries, one per
// terminating period.
func lexEntries(src string) ([]entry, error) {
	var entries []entry
	var buf strings.Builder
	bufLine := 0
	flush := func() error {
		text := strings.TrimSpace(buf.String())
		buf.Reset()
		if text == "" {
			return nil
		}
		m := entryRe.FindStringSubmatch(text)
		if m == nil {
			return fmt.Errorf("copybook: line %d: malformed entry %q", bufLine, text)
		}
		level, _ := strconv.Atoi(m[1])
		entries = append(entries, entry{level: level, name: strings.ToUpper(m[2]), clause: m[3], line: bufLine})
		return nil
	}
	for i, line := range strings.Split(src, "\n") {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if buf.Len() == 0 {
			bufLine = lineNo
		}
		for _, r := range line {
			if r == '.' {
				if err := flush(); err != nil {
					return nil, err
				}
				continue
			}
			buf.WriteRune(r)
		}
		buf.WriteByte(' ')
	}
	if strings.TrimSpace(buf.String()) != "" {
		return nil, fmt.Errorf("copybook: line %d: entry not terminated by '.'", bufLine)
	}
	return entries, nil
}

// buildTree builds the node subtree rooted at entries[i], returning the
// index of the first entry not consumed.
func buildTree(entries []entry, i int) (*Node, int, error) {
	e := entries[i]
	if e.level == 66 || e.level == 77 || e.level == 88 {
		return nil, 0, fmt.Errorf("copybook: line %d: level %02d is not supported", e.line, e.level)
	}
	if e.level < 1 || e.level > 49 {
		return nil, 0, fmt.Errorf("copybook: line %d: illegal level %02d", e.line, e.level)
	}
	node := &Node{Name: e.name, Level: e.level, Occurs: 1, Line: e.line}
	clause := strings.ToUpper(e.clause)
	if strings.Contains(clause, "REDEFINES") {
		return nil, 0, fmt.Errorf("copybook: line %d: REDEFINES is not supported", e.line)
	}
	if strings.Contains(clause, "DEPENDING") {
		return nil, 0, fmt.Errorf("copybook: line %d: OCCURS DEPENDING ON is not supported", e.line)
	}
	rest := clause
	if n, tail, ok, err := parseOccurs(rest, e.line); err != nil {
		return nil, 0, err
	} else if ok {
		node.Occurs = n
		rest = tail
	}
	i++
	if rest == "" {
		// Group item: consume children with deeper levels.
		node.Group = true
		for i < len(entries) && entries[i].level > e.level {
			child, next, err := buildTree(entries, i)
			if err != nil {
				return nil, 0, err
			}
			node.Children = append(node.Children, child)
			i = next
		}
		if len(node.Children) == 0 {
			return nil, 0, fmt.Errorf("copybook: line %d: group %s has no children", e.line, e.name)
		}
		return node, i, nil
	}
	if err := parsePicture(node, rest, e.line); err != nil {
		return nil, 0, err
	}
	return node, i, nil
}

var occursRe = regexp.MustCompile(`^OCCURS\s+(\d+)(?:\s+TIMES)?\s*(.*)$`)

func parseOccurs(clause string, line int) (int, string, bool, error) {
	m := occursRe.FindStringSubmatch(clause)
	if m == nil {
		if strings.HasPrefix(clause, "OCCURS") {
			return 0, "", false, fmt.Errorf("copybook: line %d: unsupported OCCURS clause (only fixed OCCURS n TIMES)", line)
		}
		return 0, clause, false, nil
	}
	n, _ := strconv.Atoi(m[1])
	if n < 1 {
		return 0, "", false, fmt.Errorf("copybook: line %d: OCCURS count must be positive", line)
	}
	return n, m[2], true, nil
}

var picRe = regexp.MustCompile(`^PIC(?:TURE)?\s+(?:IS\s+)?(\S+)(?:\s+(.*))?$`)

func parsePicture(node *Node, clause string, line int) error {
	m := picRe.FindStringSubmatch(clause)
	if m == nil {
		return fmt.Errorf("copybook: line %d: expected PIC clause on %s", line, node.Name)
	}
	pic, usage := m[1], m[2]
	comp3 := false
	if usage != "" {
		u := strings.Join(strings.Fields(usage), " ")
		switch u {
		case "COMP-3", "PACKED-DECIMAL", "COMPUTATIONAL-3":
			comp3 = true
		case "DISPLAY":
		default:
			return fmt.Errorf("copybook: line %d: unsupported usage %q", line, usage)
		}
	}
	signed := false
	if strings.HasPrefix(pic, "S") {
		signed = true
		pic = pic[1:]
	}
	intPart, scale := pic, 0
	if idx := strings.IndexByte(pic, 'V'); idx >= 0 {
		intPart = pic[:idx]
		frac := pic[idx+1:]
		n, err := picCount(frac, '9')
		if err != nil {
			return fmt.Errorf("copybook: line %d: bad PIC %q", line, pic)
		}
		scale = n
	}
	if strings.HasPrefix(intPart, "X") {
		if signed || scale != 0 {
			return fmt.Errorf("copybook: line %d: bad text PIC %q", line, pic)
		}
		if comp3 {
			return fmt.Errorf("copybook: line %d: text field cannot be COMP-3", line)
		}
		n, err := picCount(intPart, 'X')
		if err != nil {
			return fmt.Errorf("copybook: line %d: bad PIC %q", line, pic)
		}
		node.Kind, node.Digits = KindText, n
		return nil
	}
	n, err := picCount(intPart, '9')
	if err != nil || n == 0 {
		return fmt.Errorf("copybook: line %d: bad numeric PIC %q", line, pic)
	}
	node.Digits, node.Scale = n, scale
	if comp3 {
		if !signed {
			return fmt.Errorf("copybook: line %d: COMP-3 field must be signed (S9...)", line)
		}
		node.Kind = KindComp3
		return nil
	}
	if signed {
		return fmt.Errorf("copybook: line %d: signed DISPLAY fields are not supported; use COMP-3", line)
	}
	node.Kind = KindDisplay
	return nil
}

// picCount counts repeated picture characters, e.g. "999" or "9(3)".
func picCount(s string, ch byte) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty picture")
	}
	total := 0
	for i := 0; i < len(s); {
		if s[i] != ch {
			return 0, fmt.Errorf("unexpected character %q", s[i])
		}
		i++
		count := 1
		if i < len(s) && s[i] == '(' {
			j := strings.IndexByte(s[i:], ')')
			if j < 0 {
				return 0, fmt.Errorf("unbalanced '('")
			}
			n, err := strconv.Atoi(s[i+1 : i+j])
			if err != nil || n < 1 {
				return 0, fmt.Errorf("bad repeat count")
			}
			count = n
			i += j + 1
		}
		total += count
	}
	return total, nil
}

func checkNames(n *Node) error {
	seen := map[string]int{}
	for _, c := range n.Children {
		if prev, dup := seen[c.Name]; dup {
			return fmt.Errorf("copybook: line %d: duplicate name %s (first defined on line %d)", c.Line, c.Name, prev)
		}
		seen[c.Name] = c.Line
		if err := checkNames(c); err != nil {
			return err
		}
	}
	return nil
}
