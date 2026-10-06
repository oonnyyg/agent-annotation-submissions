// Package po parses and writes a subset of the gettext PO/POT format.
package po

import (
	"fmt"
	"strings"
)

// Entry is a single message entry in a catalog.
type Entry struct {
	Comments     []string // comment/flag lines kept verbatim (without leading "#")
	Msgctxt      string
	HasCtxt      bool // distinguishes "no msgctxt" from msgctxt ""
	Msgid        string
	MsgidPlural  string
	HasPlural    bool
	Msgstr       string
	MsgstrPlural map[int]string
	Obsolete     bool
	Fuzzy        bool
}

// Key identifies an entry by context and msgid.
type Key struct {
	Ctxt    string
	HasCtxt bool
	Msgid   string
}

func (e *Entry) Key() Key {
	return Key{Ctxt: e.Msgctxt, HasCtxt: e.HasCtxt, Msgid: e.Msgid}
}

// File is a parsed catalog: a header entry (empty msgid) plus messages.
type File struct {
	Header  *Entry
	Entries []*Entry
}

// unescape interprets PO string escapes.
func unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			return "", fmt.Errorf("dangling backslash")
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case '0':
			b.WriteByte(0)
		default:
			return "", fmt.Errorf("unsupported escape \\%c", s[i])
		}
	}
	return b.String(), nil
}

// escape renders a Go string as PO string content.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}