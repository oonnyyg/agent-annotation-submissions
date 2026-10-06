package po

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse parses PO/POT content (UTF-8) into a File.
func Parse(data string) (*File, error) {
	data = strings.TrimPrefix(data, "\ufeff")
	f := &File{}
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var cur *Entry
	var field string // current multi-line field target
	var plIdx int

	flush := func() error {
		if cur == nil {
			return nil
		}
		if cur.MsgID == "" && !cur.HasCtx && !cur.Obsolete && len(f.Entries) == 0 && f.Header == "" {
			f.Header = cur.MsgStr
			f.HeaderComments = cur.Comments
		} else {
			f.Entries = append(f.Entries, cur)
		}
		cur = nil
		return nil
	}

	for ln, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			if err := flush(); err != nil {
				return nil, err
			}
		case strings.HasPrefix(t, "#"):
			if strings.HasPrefix(t, "#~") {
				// obsolete entry
				if cur == nil {
					cur = &Entry{Obsolete: true}
				}
				rest := strings.TrimSpace(t[2:])
				if rest == "" {
					continue
				}
				if err := parseDirective(cur, rest, &field, &plIdx, ln); err != nil {
					return nil, err
				}
				continue
			}
			if cur == nil {
				cur = &Entry{}
			}
			cur.Comments = append(cur.Comments, t)
			if strings.HasPrefix(t, "#,") {
				for _, fl := range strings.Split(strings.TrimSpace(t[2:]), ",") {
					cur.Flags = append(cur.Flags, strings.TrimSpace(fl))
				}
			}
		default:
			if cur == nil {
				cur = &Entry{}
			}
			if err := parseDirective(cur, t, &field, &plIdx, ln); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return f, nil
}

func unquote(s string, ln int) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("line %d: expected quoted string: %q", ln+1, s)
	}
	v, err := strconv.Unquote(s)
	if err != nil {
		return "", fmt.Errorf("line %d: bad string: %w", ln+1, err)
	}
	return v, nil
}

func parseDirective(e *Entry, t string, field *string, plIdx *int, ln int) error {
	if strings.HasPrefix(t, `"`) {
		v, err := unquote(t, ln)
		if err != nil {
			return err
		}
		switch *field {
		case "msgctxt":
			e.Context += v
		case "msgid":
			e.MsgID += v
		case "msgid_plural":
			e.MsgIDPl += v
		case "msgstr":
			e.MsgStr += v
		case "msgstr[]":
			for len(e.MsgStrPl) <= *plIdx {
				e.MsgStrPl = append(e.MsgStrPl, "")
			}
			e.MsgStrPl[*plIdx] += v
		default:
			return fmt.Errorf("line %d: stray string continuation", ln+1)
		}
		return nil
	}
	name, rest := t, ""
	if i := strings.IndexByte(t, ' '); i >= 0 {
		name, rest = t[:i], strings.TrimSpace(t[i+1:])
	}
	switch {
	case name == "msgctxt":
		v, err := unquote(rest, ln)
		if err != nil {
			return err
		}
		e.Context, e.HasCtx = v, true
		*field = "msgctxt"
	case name == "msgid":
		v, err := unquote(rest, ln)
		if err != nil {
			return err
		}
		e.MsgID = v
		*field = "msgid"
	case name == "msgid_plural":
		v, err := unquote(rest, ln)
		if err != nil {
			return err
		}
		e.MsgIDPl, e.HasPlural = v, true
		*field = "msgid_plural"
	case name == "msgstr":
		v, err := unquote(rest, ln)
		if err != nil {
			return err
		}
		e.MsgStr = v
		*field = "msgstr"
	case strings.HasPrefix(name, "msgstr["):
		var idx int
		if _, err := fmt.Sscanf(name, "msgstr[%d]", &idx); err != nil {
			return fmt.Errorf("line %d: bad msgstr index %q", ln+1, name)
		}
		v, err := unquote(rest, ln)
		if err != nil {
			return err
		}
		for len(e.MsgStrPl) <= idx {
			e.MsgStrPl = append(e.MsgStrPl, "")
		}
		e.MsgStrPl[idx] = v
		*field, *plIdx = "msgstr[]", idx
	default:
		return fmt.Errorf("line %d: unknown directive %q", ln+1, name)
	}
	return nil
}
