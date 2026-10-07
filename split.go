package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// PathSegment is one step of a record path: a namespace URI ("" for no
// namespace) and a local name.
type PathSegment struct {
	URI   string
	Local string
}

// ParsePath parses a path such as "{urn:a}root/{urn:b}record/child".
// Segments without a "{uri}" prefix match the empty namespace.
func ParsePath(s string) ([]PathSegment, error) {
	if s == "" {
		return nil, fmt.Errorf("path must not be empty")
	}
	parts := strings.Split(s, "/")
	path := make([]PathSegment, 0, len(parts))
	for _, p := range parts {
		var seg PathSegment
		if strings.HasPrefix(p, "{") {
			i := strings.Index(p, "}")
			if i < 0 {
				return nil, fmt.Errorf("invalid path segment %q: missing }", p)
			}
			seg.URI, seg.Local = p[1:i], p[i+1:]
		} else {
			seg.Local = p
		}
		if seg.Local == "" {
			return nil, fmt.Errorf("invalid path segment %q: empty local name", p)
		}
		path = append(path, seg)
	}
	return path, nil
}

type nsBinding struct {
	prefix string
	uri    string
}

// nsFrame holds the namespace bindings declared on one element plus the
// effective xml:lang / xml:space values after that element.
type nsFrame struct {
	bindings []nsBinding
	lang     string
	space    string
}

type nsContext struct {
	frames []nsFrame
}

func (c *nsContext) push(f nsFrame) { c.frames = append(c.frames, f) }

func (c *nsContext) pop() { c.frames = c.frames[:len(c.frames)-1] }

// lookupPrefix finds a prefix currently bound to uri, innermost first.
// allowDefault permits returning the default (empty) prefix.
func (c *nsContext) lookupPrefix(uri string, allowDefault bool) (string, bool) {
	for i := len(c.frames) - 1; i >= 0; i-- {
		for _, b := range c.frames[i].bindings {
			if b.uri == uri && (allowDefault || b.prefix != "") {
				return b.prefix, true
			}
		}
	}
	return "", false
}

func (c *nsContext) uriInScope(uri string) bool {
	_, ok := c.lookupPrefix(uri, true)
	return ok
}

func (c *nsContext) prefixInScope(prefix string) bool {
	for i := len(c.frames) - 1; i >= 0; i-- {
		for _, b := range c.frames[i].bindings {
			if b.prefix == prefix {
				return true
			}
		}
	}
	return false
}

// inScopeBindings returns all bindings visible at the current depth,
// innermost declarations winning on duplicate prefixes.
func (c *nsContext) inScopeBindings() []nsBinding {
	seen := map[string]bool{}
	var out []nsBinding
	for i := len(c.frames) - 1; i >= 0; i-- {
		for _, b := range c.frames[i].bindings {
			if !seen[b.prefix] {
				seen[b.prefix] = true
				out = append(out, b)
			}
		}
	}
	return out
}

func (c *nsContext) effectiveLangSpace() (lang, space string) {
	if len(c.frames) > 0 {
		top := c.frames[len(c.frames)-1]
		return top.lang, top.space
	}
	return "", ""
}

// Splitter streams an XML document and extracts every element subtree whose
// resolved (namespace URI, local name) stack equals the configured path.
type Splitter struct {
	dec      *xml.Decoder
	path     []PathSegment
	maxSize  int64
	onRecord func(data []byte) error

	stack     []PathSegment
	ns        nsContext
	count     int
	gen       int
	capBuf    bytes.Buffer
	capDepth  int
	capNames  []string
	capturing bool
}

// Run consumes the whole stream, invoking onRecord for each match in
// document order. Any malformed input, DTD, undeclared prefix or oversized
// record aborts the run with a positioned error.
func Run(r io.Reader, path []PathSegment, maxSize int64, onRecord func(data []byte) error) (int, error) {
	s := &Splitter{
		dec:      xml.NewDecoder(r),
		path:     path,
		maxSize:  maxSize,
		onRecord: onRecord,
	}
	if err := s.loop(); err != nil {
		return s.count, err
	}
	return s.count, nil
}

func (s *Splitter) posErr(format string, args ...interface{}) error {
	line, col := s.dec.InputPos()
	return fmt.Errorf("%s at line %d, column %d", fmt.Sprintf(format, args...), line, col)
}

func (s *Splitter) loop() error {
	for {
		t, err := s.dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			line, col := s.dec.InputPos()
			return fmt.Errorf("invalid XML: %v (line %d, column %d)", err, line, col)
		}
		switch e := t.(type) {
		case xml.Directive:
			return s.posErr("DTD and DOCTYPE declarations are not allowed")
		case xml.StartElement:
			if err := s.start(e); err != nil {
				return err
			}
		case xml.EndElement:
			if err := s.end(); err != nil {
				return err
			}
		case xml.CharData:
			if s.capturing {
				escapeText(&s.capBuf, e)
				if err := s.checkSize(); err != nil {
					return err
				}
			}
		case xml.Comment:
			if s.capturing {
				s.capBuf.WriteString("<!--")
				s.capBuf.Write(e)
				s.capBuf.WriteString("-->")
				if err := s.checkSize(); err != nil {
					return err
				}
			}
		case xml.ProcInst:
			if s.capturing {
				s.capBuf.WriteString("<?")
				s.capBuf.WriteString(e.Target)
				if len(e.Inst) > 0 {
					s.capBuf.WriteByte(' ')
					s.capBuf.Write(e.Inst)
				}
				s.capBuf.WriteString("?>")
				if err := s.checkSize(); err != nil {
					return err
				}
			}
		}
	}
}

func (s *Splitter) checkSize() error {
	if s.maxSize > 0 && int64(s.capBuf.Len()) > s.maxSize {
		return s.posErr("record %d exceeds the maximum size of %d bytes", s.count+1, s.maxSize)
	}
	return nil
}

// start pushes namespace context, validates prefixes, detects record
// boundaries and, while capturing, serializes the start tag.
func (s *Splitter) start(e xml.StartElement) error {
	lang, space := s.ns.effectiveLangSpace()
	frame := nsFrame{lang: lang, space: space}
	for _, a := range e.Attr {
		switch {
		case a.Name.Space == "xmlns":
			frame.bindings = append(frame.bindings, nsBinding{a.Name.Local, a.Value})
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			frame.bindings = append(frame.bindings, nsBinding{"", a.Value})
		case a.Name.Space == xmlNamespace && a.Name.Local == "lang":
			frame.lang = a.Value
		case a.Name.Space == xmlNamespace && a.Name.Local == "space":
			frame.space = a.Value
		}
	}
	parentLang, parentSpace := lang, space
	s.ns.push(frame)

	if err := s.checkResolved(e.Name.Space, "element", e.Name.Local); err != nil {
		return err
	}
	for _, a := range e.Attr {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		if err := s.checkResolved(a.Name.Space, "attribute", a.Name.Local); err != nil {
			return err
		}
	}
	s.stack = append(s.stack, PathSegment{e.Name.Space, e.Name.Local})

	if !s.capturing && len(s.stack) == len(s.path) && s.matches() {
		s.capturing = true
		s.capDepth = len(s.stack)
		s.capBuf.Reset()
		s.capNames = s.capNames[:0]
		s.capBuf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		s.capBuf.WriteByte('\n')
	}
	if s.capturing {
		isRoot := len(s.stack) == s.capDepth
		if err := s.writeStartElement(e, isRoot, parentLang, parentSpace); err != nil {
			return err
		}
	}
	return s.checkSize()
}

// checkResolved rejects names whose prefix was never declared. The standard
// decoder leaves the literal prefix in Name.Space when it cannot resolve it,
// so any "URI" that is not actually in scope is an undeclared prefix.
func (s *Splitter) checkResolved(space, kind, local string) error {
	if space == "" || space == xmlNamespace {
		return nil
	}
	if !s.ns.uriInScope(space) {
		return s.posErr("undeclared prefix %q on %s %q", space, kind, local)
	}
	return nil
}

func (s *Splitter) matches() bool {
	for i, seg := range s.path {
		if s.stack[i] != seg {
			return false
		}
	}
	return true
}

func (s *Splitter) end() error {
	if s.capturing {
		name := s.capNames[len(s.capNames)-1]
		s.capNames = s.capNames[:len(s.capNames)-1]
		s.capBuf.WriteString("</")
		s.capBuf.WriteString(name)
		s.capBuf.WriteByte('>')
		if len(s.stack) == s.capDepth {
			s.capturing = false
			s.count++
			if err := s.onRecord(s.capBuf.Bytes()); err != nil {
				return err
			}
			s.capBuf.Reset()
		}
	}
	s.stack = s.stack[:len(s.stack)-1]
	s.ns.pop()
	return nil
}

// prefixFor returns a prefix usable for uri, generating and declaring a new
// one when no suitable binding is in scope.
func (s *Splitter) prefixFor(uri string, allowDefault bool, decls *[]nsBinding) string {
	if p, ok := s.ns.lookupPrefix(uri, allowDefault); ok {
		return p
	}
	for {
		s.gen++
		p := fmt.Sprintf("ns%d", s.gen)
		if s.ns.prefixInScope(p) {
			continue
		}
		b := nsBinding{p, uri}
		*decls = append(*decls, b)
		top := &s.ns.frames[len(s.ns.frames)-1]
		top.bindings = append(top.bindings, b)
		return p
	}
}

// writeStartElement serializes one start tag into the record buffer,
// re-declaring every namespace binding the subtree relies on.
func (s *Splitter) writeStartElement(e xml.StartElement, isRoot bool, parentLang, parentSpace string) error {
	var decls []nsBinding
	if isRoot {
		decls = s.ns.inScopeBindings()
	} else {
		decls = append(decls, s.ns.frames[len(s.ns.frames)-1].bindings...)
	}

	name := e.Name.Local
	if e.Name.Space != "" {
		if p := s.prefixFor(e.Name.Space, true, &decls); p != "" {
			name = p + ":" + e.Name.Local
		}
	}

	type attr struct{ name, value string }
	var attrs []attr
	hasLang, hasSpace := false, false
	for _, a := range e.Attr {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		var an string
		switch {
		case a.Name.Space == "":
			an = a.Name.Local
		case a.Name.Space == xmlNamespace:
			an = "xml:" + a.Name.Local
			if a.Name.Local == "lang" {
				hasLang = true
			}
			if a.Name.Local == "space" {
				hasSpace = true
			}
		default:
			an = s.prefixFor(a.Name.Space, false, &decls) + ":" + a.Name.Local
		}
		attrs = append(attrs, attr{an, a.Value})
	}
	if isRoot {
		if parentLang != "" && !hasLang {
			attrs = append(attrs, attr{"xml:lang", parentLang})
		}
		if parentSpace != "" && !hasSpace {
			attrs = append(attrs, attr{"xml:space", parentSpace})
		}
	}

	s.capBuf.WriteByte('<')
	s.capBuf.WriteString(name)
	for _, b := range decls {
		if b.prefix == "" {
			s.capBuf.WriteString(` xmlns="`)
		} else {
			s.capBuf.WriteString(` xmlns:`)
			s.capBuf.WriteString(b.prefix)
			s.capBuf.WriteString(`="`)
		}
		escapeAttr(&s.capBuf, b.uri)
		s.capBuf.WriteByte('"')
	}
	for _, a := range attrs {
		s.capBuf.WriteByte(' ')
		s.capBuf.WriteString(a.name)
		s.capBuf.WriteString(`="`)
		escapeAttr(&s.capBuf, a.value)
		s.capBuf.WriteByte('"')
	}
	s.capBuf.WriteByte('>')
	s.capNames = append(s.capNames, name)
	return nil
}

var textEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\r", "&#xD;",
)

var attrEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"\t", "&#x9;",
	"\n", "&#xA;",
	"\r", "&#xD;",
)

func escapeText(buf *bytes.Buffer, data []byte) {
	textEscaper.WriteString(buf, string(data))
}

func escapeAttr(buf *bytes.Buffer, s string) {
	attrEscaper.WriteString(buf, s)
}
