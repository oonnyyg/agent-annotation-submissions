package po

import (
	"strconv"
	"strings"
)

func quote(s string) string {
	return strconv.Quote(s)
}

func writeField(b *strings.Builder, prefix, name, value string) {
	if value == "" {
		b.WriteString(prefix + name + " \"\"\n")
		return
	}
	b.WriteString(prefix + name + " " + quote(value) + "\n")
}

func writeEntry(b *strings.Builder, e *Entry) {
	prefix := ""
	if e.Obsolete {
		prefix = "#~ "
	}
	if !e.Obsolete {
		for _, c := range e.Comments {
			b.WriteString(c + "\n")
		}
	}
	if e.HasCtx {
		writeField(b, prefix, "msgctxt", e.Context)
	}
	writeField(b, prefix, "msgid", e.MsgID)
	if e.HasPlural {
		writeField(b, prefix, "msgid_plural", e.MsgIDPl)
		if len(e.MsgStrPl) == 0 {
			b.WriteString(prefix + "msgstr[0] \"\"\n")
			b.WriteString(prefix + "msgstr[1] \"\"\n")
		}
		for i, s := range e.MsgStrPl {
			b.WriteString(prefix + "msgstr[" + strconv.Itoa(i) + "] " + quote(s) + "\n")
		}
	} else {
		writeField(b, prefix, "msgstr", e.MsgStr)
	}
	b.WriteString("\n")
}

// String renders the file back to PO text.
func (f *File) String() string {
	var b strings.Builder
	for _, c := range f.HeaderComments {
		b.WriteString(c + "\n")
	}
	b.WriteString("msgid \"\"\n")
	writeField(&b, "", "msgstr", f.Header)
	b.WriteString("\n")
	for _, e := range f.Entries {
		writeEntry(&b, e)
	}
	return b.String()
}
