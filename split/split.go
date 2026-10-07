// Package split implements a streaming XML record splitter.
//
// It reads an XML document token by token, matches elements against an
// absolute namespace-aware path, and extracts each matching subtree as a
// self-contained XML fragment that preserves namespace semantics.
package split

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Name identifies an element by namespace URI and local name.
// An empty URI means "no namespace".
type Name struct {
	URI   string
	Local string
}

// ParsePath parses a record path such as "{urn:a}root/{urn:b}item".
// A leading "/" is optional. Every step must be "{uri}local" or a bare
// "local" (no namespace); descendant wildcards are not supported.
func ParsePath(s string) ([]Name, error) {
	s = strings.TrimPrefix(s, "/")
	if s == "" {
		return nil, fmt.Errorf("record path is empty")
	}
	var path []Name
	for _, step := range strings.Split(s, "/") {
		if step == "" {
			return nil, fmt.Errorf("record path %q contains an empty step", s)
		}
		var n Name
		if strings.HasPrefix(step, "{") {
			i := strings.Index(step, "}")
			if i < 0 {
				return nil, fmt.Errorf("record path step %q misses closing '}'", step)
			}
			n.URI = step[1:i]
			n.Local = step[i+1:]
		} else {
			n.Local = step
		}
		if n.Local == "" || strings.ContainsAny(n.Local, "{}") {
			return nil, fmt.Errorf("record path step %q has an invalid local name", step)
		}
		path = append(path, n)
	}
	return path, nil
}

const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// Attr is a namespace-resolved attribute.
type Attr struct {
	Name  Name
	Value string
}

// event is one buffered token of a record being captured.
type event struct {
	kind  eventKind
	start startEvent
	end   Name
	text  []byte // character data, unescaped by the decoder
	extra []byte // comment or processing instruction, verbatim
}

type eventKind int

const (
	evStart eventKind = iota
	evEnd
	evText
	evComment
	evProcInst
)

type startEvent struct {
	Name  Name
	Attrs []Attr
	// inherited xml:lang / xml:space, only set on the record root
	lang  string
	space string
}

// Error describes a failure with a document position.
type Error struct {
	Offset int64
	Line   int
	Col    int
	Msg    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (offset %d, line %d, col %d)", e.Msg, e.Offset, e.Line, e.Col)
}

// posReader tracks byte offset and 1-based line/column of the stream.
type posReader struct {
	r      io.Reader
	offset int64
	line   int
	col    int
}

func newPosReader(r io.Reader) *posReader { return &posReader{r: r, line: 1} }

func (p *posReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	for _, c := range b[:n] {
		p.offset++
		if c == '\n' {
			p.line++
			p.col = 0
		} else {
			p.col++
		}
	}
	return n, err
}

func (p *posReader) errf(offset int64, format string, args ...any) *Error {
	return &Error{
		Offset: offset,
		Line:   p.line,
		Col:    p.col,
		Msg:    fmt.Sprintf(format, args...),
	}
}

// frame is one open element on the parser stack.
type frame struct {
	name     Name
	bindings map[string]string // prefix -> URI declared on this element
	attrs    []Attr
	lang     string // xml:lang declared on this element, "" if none
	space    string // xml:space declared on this element, "" if none
}

// Splitter extracts records from one XML stream.
type Splitter struct {
	Path      []Name
	MaxRecord int64 // maximum buffered bytes per record; 0 means 16 MiB
	OnRecord  func(data []byte) error
}

func (s *Splitter) maxRecord() int64 {
	if s.MaxRecord > 0 {
		return s.MaxRecord
	}
	return 16 << 20
}

// Split consumes the whole stream. It returns an error (with position)
// for malformed XML, DTDs, undeclared prefixes or entities, and records
// exceeding the size limit. The stream must be UTF-8 XML.
func (s *Splitter) Split(r io.Reader) (int, error) {
	pr := newPosReader(r)
	dec := xml.NewDecoder(pr)
	var stack []*frame
	var rec []event
	var recSize int64
	recDepth := 0 // element depth inside the captured record
	recordN := 0

	flush := func() error {
		data, err := renderRecord(rec, s.maxRecord())
		if err != nil {
			return err
		}
		recordN++
		if s.OnRecord != nil {
			if err := s.OnRecord(data); err != nil {
				return err
			}
		}
		rec = nil
		recSize = 0
		return nil
	}

	for {
		offset := dec.InputOffset()
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return recordN, pr.errf(offset, "malformed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			if bytes.Contains(bytes.ToUpper(t), []byte("DOCTYPE")) {
				return recordN, pr.errf(offset, "DTD/DOCTYPE is not allowed")
			}
		case xml.StartElement:
			f, err := resolveStart(pr, offset, t, stack)
			if err != nil {
				return recordN, err
			}
			stack = append(stack, f)
			match := len(stack) == len(s.Path) && pathMatches(stack, s.Path)
			if recDepth > 0 || match {
				ev := startEvent{Name: f.name, Attrs: f.attrs}
				if match && recDepth == 0 {
					ev.lang, ev.space = inheritedXMLAttrs(stack)
				}
				rec = append(rec, event{kind: evStart, start: ev})
				recSize += int64(eventSize(ev)) + 64
				recDepth++
				if recSize > s.maxRecord() {
					return recordN, pr.errf(offset, "record exceeds size limit of %d bytes", s.maxRecord())
				}
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return recordN, pr.errf(offset, "unexpected closing tag </%s>", t.Name.Local)
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if recDepth > 0 {
				rec = append(rec, event{kind: evEnd, end: top.name})
				recDepth--
				if recDepth == 0 {
					if err := flush(); err != nil {
						return recordN, err
					}
				}
			}
		case xml.CharData:
			if recDepth > 0 {
				cp := append([]byte(nil), t...)
				rec = append(rec, event{kind: evText, text: cp})
				recSize += int64(len(cp))
				if recSize > s.maxRecord() {
					return recordN, pr.errf(offset, "record exceeds size limit of %d bytes", s.maxRecord())
				}
			}
		case xml.Comment:
			if recDepth > 0 {
				rec = append(rec, event{kind: evComment, extra: append([]byte(nil), t...)})
			}
		case xml.ProcInst:
			if recDepth > 0 {
				buf := append([]byte(t.Target), ' ')
				buf = append(buf, t.Inst...)
				rec = append(rec, event{kind: evProcInst, extra: buf})
			}
		}
	}
	if len(stack) != 0 {
		return recordN, pr.errf(dec.InputOffset(), "unexpected end of document: unclosed element <%s>", stack[len(stack)-1].name.Local)
	}
	return recordN, nil
}

func pathMatches(stack []*frame, path []Name) bool {
	for i, n := range path {
		if stack[i].name != n {
			return false
		}
	}
	return true
}

func eventSize(ev startEvent) int {
	n := len(ev.Name.Local) + len(ev.Name.URI)
	for _, a := range ev.Attrs {
		n += len(a.Name.Local) + len(a.Value)
	}
	return n
}

// inheritedXMLAttrs returns the xml:lang / xml:space values in scope at the
// record root, i.e. the effective values contributed by its ancestors.
func inheritedXMLAttrs(stack []*frame) (lang, space string) {
	for _, f := range stack[:len(stack)-1] {
		if f.lang != "" {
			lang = f.lang
		}
		if f.space != "" {
			space = f.space
		}
	}
	return lang, space
}
