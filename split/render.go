package split

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

// renderRecord serializes one buffered record as a standalone XML
// document. Namespace prefixes are regenerated per record: the default
// namespace is reused for the record root's namespace (if any), every
// other URI used by elements or attributes gets a fresh "ns<N>" prefix
// declared on the record root. Unprefixed attributes are never given a
// namespace, so they cannot accidentally inherit the default one.
func renderRecord(rec []event, maxBytes int64) ([]byte, error) {
	if len(rec) == 0 {
		return nil, fmt.Errorf("internal error: empty record")
	}
	root := rec[0].start

	// Collect every namespace URI used inside the record.
	used := map[string]bool{}
	attrUsed := map[string]bool{}
	for _, ev := range rec {
		if ev.kind != evStart {
			continue
		}
		if ev.start.Name.URI != "" {
			used[ev.start.Name.URI] = true
		}
		for _, a := range ev.start.Attrs {
			if a.Name.URI != "" {
				used[a.Name.URI] = true
				attrUsed[a.Name.URI] = true
			}
		}
	}

	defaultURI := root.Name.URI
	prefixOf := map[string]string{xmlNamespace: "xml"}
	var declURIs []string
	for uri := range used {
		if uri == xmlNamespace {
			continue // "xml" prefix is predefined, never declared
		}
		if uri == defaultURI {
			continue // covered by the default xmlns declaration
		}
		declURIs = append(declURIs, uri)
	}
	sort.Strings(declURIs)
	for i, uri := range declURIs {
		prefixOf[uri] = "ns" + strconv.Itoa(i+1)
	}
	// Attributes in the default namespace still need an explicit prefix.
	attrPrefixOf := map[string]string{}
	for uri, p := range prefixOf {
		attrPrefixOf[uri] = p
	}
	attrDefault := ""
	if attrUsed[defaultURI] && defaultURI != "" {
		attrDefault = "ns" + strconv.Itoa(len(declURIs)+1)
		attrPrefixOf[defaultURI] = attrDefault
	}

	elemName := func(n Name) string {
		if n.URI == "" || n.URI == defaultURI {
			return n.Local
		}
		return prefixOf[n.URI] + ":" + n.Local
	}
	attrName := func(n Name) string {
		if n.URI == "" {
			return n.Local
		}
		return attrPrefixOf[n.URI] + ":" + n.Local
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	depth := 0
	for _, ev := range rec {
		switch ev.kind {
		case evStart:
			buf.WriteByte('<')
			buf.WriteString(elemName(ev.start.Name))
			if depth == 0 {
				if defaultURI != "" {
					writeAttr(&buf, "xmlns", defaultURI)
				}
				for _, uri := range declURIs {
					writeAttr(&buf, "xmlns:"+prefixOf[uri], uri)
				}
				if attrDefault != "" {
					writeAttr(&buf, "xmlns:"+attrDefault, defaultURI)
				}
				writeInheritedXML(&buf, ev.start)
			}
			for _, a := range ev.start.Attrs {
				writeAttr(&buf, attrName(a.Name), a.Value)
			}
			buf.WriteByte('>')
			depth++
		case evEnd:
			buf.WriteString("</")
			buf.WriteString(elemName(ev.end))
			buf.WriteByte('>')
			depth--
		case evText:
			escapeText(&buf, ev.text)
		case evComment:
			buf.WriteString("<!--")
			buf.Write(ev.extra)
			buf.WriteString("-->")
		case evProcInst:
			buf.WriteString("<?")
			buf.Write(ev.extra)
			buf.WriteString("?>")
		}
		if int64(buf.Len()) > maxBytes {
			return nil, fmt.Errorf("record exceeds size limit of %d bytes after rendering", maxBytes)
		}
	}
	return buf.Bytes(), nil
}

// writeInheritedXML re-emits xml:lang / xml:space inherited from ancestors
// on the record root, unless the root already declares them itself.
func writeInheritedXML(buf *bytes.Buffer, root startEvent) {
	hasLang, hasSpace := false, false
	for _, a := range root.Attrs {
		if a.Name.URI == xmlNamespace {
			switch a.Name.Local {
			case "lang":
				hasLang = true
			case "space":
				hasSpace = true
			}
		}
	}
	if root.lang != "" && !hasLang {
		writeAttr(buf, "xml:lang", root.lang)
	}
	if root.space != "" && !hasSpace {
		writeAttr(buf, "xml:space", root.space)
	}
}

func writeAttr(buf *bytes.Buffer, name, value string) {
	buf.WriteByte(' ')
	buf.WriteString(name)
	buf.WriteString(`="`)
	escapeAttr(buf, value)
	buf.WriteByte('"')
}

var attrEscaper = map[byte]string{
	'&':  "&amp;",
	'<':  "&lt;",
	'>':  "&gt;",
	'"':  "&quot;",
	'\t': "&#9;",
	'\n': "&#10;",
	'\r': "&#13;",
}

func escapeAttr(buf *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		if esc, ok := attrEscaper[s[i]]; ok {
			buf.WriteString(esc)
		} else {
			buf.WriteByte(s[i])
		}
	}
}

var textEscaper = map[byte]string{
	'&': "&amp;",
	'<': "&lt;",
	'>': "&gt;",
}

func escapeText(buf *bytes.Buffer, s []byte) {
	for _, c := range s {
		if esc, ok := textEscaper[c]; ok {
			buf.WriteString(esc)
		} else {
			buf.WriteByte(c)
		}
	}
}
