package po

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse reads a PO/POT file from text.
func Parse(text string) (*File, error) {
	f := &File{}
	var cur *Entry
	// state: which field a continued string belongs to.
	var field string // "ctxt","id","idp","str" or "strN"
	strIdx := -1
	flush := func() {
		if cur == nil {
			return
		}
		if cur.Msgid == "" && !cur.HasPlural && !cur.HasCtxt && !cur.Obsolete {
			if f.Header == nil {
				f.Header = cur
			} else {
				f.Entries = append(f.Entries, cur)
			}
		} else {
			f.Entries = append(f.Entries, cur)
		}
		cur = nil
	}
	lines := strings.Split(text, "\n")
	for ln, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		pos := ln + 1
		if strings.TrimSpace(line) == "" {
			flush()
			field = ""
			continue
		}
		obsolete := strings.HasPrefix(line, "#~")
		body := line
		if obsolete {
			body = strings.TrimSpace(line[2:])
		}
		if strings.HasPrefix(body, "#") && !obsolete {
			if cur == nil {
				cur = &Entry{MsgstrPlural: map[int]string{}}
			}
			cur.Comments = append(cur.Comments, body[1:])
			if strings.Contains(body, "fuzzy") && strings.HasPrefix(strings.TrimSpace(body[1:]), ",") {
				cur.Fuzzy = true
			}
			continue
		}
		if strings.HasPrefix(body, "#") { // obsolete comment line "#~ #..."
			continue
		}
		if cur == nil {
			cur = &Entry{MsgstrPlural: map[int]string{}}
		}
		if obsolete {
			cur.Obsolete = true
		}
		// keyword lines
		switch {
		case strings.HasPrefix(body, "msgctxt"):
			s, err := parseString(strings.TrimSpace(body[len("msgctxt"):]), pos)
			if err != nil {
				return nil, err
			}
			cur.Msgctxt, cur.HasCtxt, field = s, true, "ctxt"
		case strings.HasPrefix(body, "msgid_plural"):
			s, err := parseString(strings.TrimSpace(body[len("msgid_plural"):]), pos)
			if err != nil {
				return nil, err
			}
			cur.MsgidPlural, cur.HasPlural, field = s, true, "idp"
		case strings.HasPrefix(body, "msgid"):
			s, err := parseString(strings.TrimSpace(body[len("msgid"):]), pos)
			if err != nil {
				return nil, err
			}
			cur.Msgid, field = s, "id"
		case strings.HasPrefix(body, "msgstr"):
			rest := strings.TrimSpace(body[len("msgstr"):])
			if strings.HasPrefix(rest, "[") {
				end := strings.Index(rest, "]")
				if end < 0 {
					return nil, fmt.Errorf("line %d: bad msgstr index", pos)
				}
				n, err := strconv.Atoi(rest[1:end])
				if err != nil {
					return nil, fmt.Errorf("line %d: bad msgstr index", pos)
				}
				s, err := parseString(strings.TrimSpace(rest[end+1:]), pos)
				if err != nil {
					return nil, err
				}
				cur.MsgstrPlural[n], field, strIdx = s, "strN", n
			} else {
				s, err := parseString(rest, pos)
				if err != nil {
					return nil, err
				}
				cur.Msgstr, field = s, "str"
			}
		case strings.HasPrefix(body, "\""):
			s, err := parseString(body, pos)
			if err != nil {
				return nil, err
			}
			switch field {
			case "ctxt":
				cur.Msgctxt += s
			case "id":
				cur.Msgid += s
			case "idp":
				cur.MsgidPlural += s
			case "str":
				cur.Msgstr += s
			case "strN":
				cur.MsgstrPlural[strIdx] += s
			default:
				return nil, fmt.Errorf("line %d: stray string", pos)
			}
		default:
			return nil, fmt.Errorf("line %d: unrecognized content: %s", pos, line)
		}
	}
	flush()
	return f, nil
}

// parseString parses a quoted PO string token.
func parseString(s string, line int) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("line %d: expected quoted string, got %q", line, s)
	}
	out, err := unescape(s[1 : len(s)-1])
	if err != nil {
		return "", fmt.Errorf("line %d: %w", line, err)
	}
	return out, nil
}