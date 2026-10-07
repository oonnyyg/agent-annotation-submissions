package split

import (
	"encoding/xml"
)

// resolveStart resolves a raw start tag against the in-scope namespace
// bindings, applying the xmlns declarations carried by the tag itself.
// It reports undeclared prefixes with the position of the start tag.
func resolveStart(pr *posReader, offset int64, t xml.StartElement, stack []*frame) (*frame, error) {
	f := &frame{}
	// 1. collect namespace declarations carried by this element
	for _, a := range t.Attr {
		switch {
		case a.Name.Local == "xmlns" && a.Name.Space == "":
			f.bind("", a.Value)
		case a.Name.Space == "xmlns":
			f.bind(a.Name.Local, a.Value)
		}
	}
	// 2. resolve the element name
	uri, err := f.resolve(pr, offset, t.Name.Space, stack, true)
	if err != nil {
		return nil, err
	}
	f.name = Name{URI: uri, Local: t.Name.Local}
	// 3. resolve attributes; unprefixed attributes stay in no namespace
	seen := map[Name]bool{}
	for _, a := range t.Attr {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		var an Name
		if a.Name.Space == "" {
			an = Name{Local: a.Name.Local}
		} else if a.Name.Space == "xml" {
			an = Name{URI: xmlNamespace, Local: a.Name.Local}
		} else {
			au, err := f.resolve(pr, offset, a.Name.Space, stack, false)
			if err != nil {
				return nil, err
			}
			an = Name{URI: au, Local: a.Name.Local}
		}
		if seen[an] {
			return nil, pr.errf(offset, "duplicate attribute %q after namespace resolution", an.Local)
		}
		seen[an] = true
		if an.URI == xmlNamespace {
			switch an.Local {
			case "lang":
				f.lang = a.Value
			case "space":
				f.space = a.Value
			}
		}
		f.attrs = append(f.attrs, Attr{Name: an, Value: a.Value})
	}
	return f, nil
}

func (f *frame) bind(pfx, uri string) {
	if f.bindings == nil {
		f.bindings = map[string]string{}
	}
	f.bindings[pfx] = uri
}

// resolve maps a raw prefix to a namespace URI using the current frame's
// declarations first, then walking up the stack. isElem selects default
// namespace handling: default namespaces apply to elements only.
func (f *frame) resolve(pr *posReader, offset int64, prefix string, stack []*frame, isElem bool) (string, error) {
	if prefix == "" {
		if !isElem {
			return "", nil
		}
		if uri, ok := f.bindings[""]; ok {
			return uri, nil
		}
		for i := len(stack) - 1; i >= 0; i-- {
			if uri, ok := stack[i].bindings[""]; ok {
				return uri, nil
			}
		}
		return "", nil
	}
	if prefix == "xml" {
		return xmlNamespace, nil
	}
	if uri, ok := f.bindings[prefix]; ok {
		return uri, nil
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if uri, ok := stack[i].bindings[prefix]; ok {
			return uri, nil
		}
	}
	return "", pr.errf(offset, "undeclared namespace prefix %q", prefix)
}
