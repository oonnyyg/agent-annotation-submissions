package po

import (
	"fmt"
	"sort"
	"strings"
)

// String renders the catalog back to PO text.
func (f *File) String() string {
	var b strings.Builder
	if f.Header != nil {
		writeEntry(&b, f.Header)
	}
	for _, e := range f.Entries {
		writeEntry(&b, e)
	}
	return b.String()
}

func writeEntry(b *strings.Builder, e *Entry) {
	prefix := ""
	if e.Obsolete {
		prefix = "#~ "
	}
	for _, c := range e.Comments {
		fmt.Fprintf(b, "#%s\n", c)
	}
	q := func(s string) string {
		// Continuation lines of multi-line strings also need the prefix.
		return strings.ReplaceAll(quote(s), "\n", "\n"+prefix)
	}
	if e.HasCtxt {
		fmt.Fprintf(b, "%smsgctxt %s\n", prefix, q(e.Msgctxt))
	}
	fmt.Fprintf(b, "%smsgid %s\n", prefix, q(e.Msgid))
	if e.HasPlural {
		fmt.Fprintf(b, "%smsgid_plural %s\n", prefix, q(e.MsgidPlural))
		keys := make([]int, 0, len(e.MsgstrPlural))
		for k := range e.MsgstrPlural {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		if len(keys) == 0 {
			fmt.Fprintf(b, "%smsgstr[0] \"\"\n", prefix)
		}
		for _, k := range keys {
			fmt.Fprintf(b, "%smsgstr[%d] %s\n", prefix, k, q(e.MsgstrPlural[k]))
		}
	} else {
		fmt.Fprintf(b, "%smsgstr %s\n", prefix, q(e.Msgstr))
	}
	b.WriteString("\n")
}

// quote renders a string as a PO string literal, splitting on newlines.
func quote(s string) string {
	if !strings.Contains(s, "\n") {
		return "\"" + escape(s) + "\""
	}
	parts := strings.SplitAfter(s, "\n")
	var b strings.Builder
	b.WriteString("\"\"\n")
	for i, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString("\"" + escape(p) + "\"")
		if i < len(parts)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}