package copybook

import (
	"encoding/json"
	"fmt"
)

// Field is a flattened, fully-located elementary item in the record layout.
// Array fields appear once; their ElementStride is the size of one element.
type Field struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	Occurs        int    `json:"occurs"`
	Digits        int    `json:"digits"`
	Scale         int    `json:"scale"`
	ElementStride int    `json:"elementStride"`
}

// Layout is the compiled record layout.
type Layout struct {
	RecordName   string
	RecordLength int
	Root         *LayoutNode
}

// LayoutNode mirrors the parse tree with computed offsets.
type LayoutNode struct {
	Node     *Node
	Offset   int // offset of the item (first element when Occurs > 1)
	Length   int // total length across all elements
	Children []*LayoutNode
}

// Compile computes offsets and lengths for every item in the tree.
func Compile(root *Node) (*Layout, error) {
	l := &Layout{RecordName: root.Name}
	ln, err := compileNode(root, 0)
	if err != nil {
		return nil, err
	}
	l.Root = ln
	l.RecordLength = ln.Length
	return l, nil
}

func elemLength(n *Node) (int, error) {
	switch n.Kind {
	case KindText, KindDisplay:
		return n.Digits + n.Scale, nil
	case KindComp3:
		total := n.Digits + n.Scale
		return total/2 + 1, nil
	}
	return 0, fmt.Errorf("copybook: line %d: unknown field kind", n.Line)
}

func compileNode(n *Node, offset int) (*LayoutNode, error) {
	ln := &LayoutNode{Node: n, Offset: offset}
	var one int
	if n.Group {
		childOff := 0
		for _, c := range n.Children {
			cl, err := compileNode(c, childOff)
			if err != nil {
				return nil, err
			}
			ln.Children = append(ln.Children, cl)
			childOff += cl.Length
		}
		one = childOff
	} else {
		el, err := elemLength(n)
		if err != nil {
			return nil, err
		}
		one = el
	}
	ln.Length = one * n.Occurs
	return ln, nil
}

// Fields flattens the layout into elementary fields with absolute offsets.
// For fields inside repeated groups, Occurs is the total element count and
// ElementStride is the stride of the innermost enclosing repetition.
func (l *Layout) Fields() []Field {
	var out []Field
	var walk func(ln *LayoutNode, base int, prefix string, repOccurs, repStride int)
	walk = func(ln *LayoutNode, base int, prefix string, repOccurs, repStride int) {
		path := ln.Node.Name
		if prefix != "" {
			path = prefix + "." + path
		}
		abs := base + ln.Offset
		if ln.Node.Group {
			ro, rs := repOccurs, repStride
			if ln.Node.Occurs > 1 {
				ro *= ln.Node.Occurs
				rs = ln.Length / ln.Node.Occurs
			}
			for _, c := range ln.Children {
				walk(c, abs, path, ro, rs)
			}
			return
		}
		el, _ := elemLength(ln.Node)
		occurs := repOccurs * ln.Node.Occurs
		stride := el
		if ln.Node.Occurs > 1 {
			stride = el
		} else if repOccurs > 1 {
			stride = repStride
		}
		out = append(out, Field{
			Path:          path,
			Kind:          kindName(ln.Node.Kind),
			Offset:        abs,
			Length:        el,
			Occurs:        occurs,
			Digits:        ln.Node.Digits,
			Scale:         ln.Node.Scale,
			ElementStride: stride,
		})
	}
	walk(l.Root, 0, "", 1, 0)
	return out
}

func kindName(k Kind) string {
	switch k {
	case KindText:
		return "text"
	case KindDisplay:
		return "display"
	case KindComp3:
		return "comp-3"
	}
	return "unknown"
}

type layoutJSON struct {
	Record       string     `json:"record"`
	RecordLength int        `json:"recordLength"`
	Items        []itemJSON `json:"items"`
}

type itemJSON struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Kind     string     `json:"kind"`
	Offset   int        `json:"offset"`
	Length   int        `json:"length"`
	Occurs   int        `json:"occurs"`
	Digits   int        `json:"digits,omitempty"`
	Scale    int        `json:"scale,omitempty"`
	Children []itemJSON `json:"children,omitempty"`
}

// MarshalLayout renders the layout as indented JSON for verification.
func (l *Layout) MarshalLayout() ([]byte, error) {
	var build func(ln *LayoutNode, prefix string) itemJSON
	build = func(ln *LayoutNode, prefix string) itemJSON {
		path := ln.Node.Name
		if prefix != "" {
			path = prefix + "." + path
		}
		one := ln.Length / ln.Node.Occurs
		it := itemJSON{
			Name:   ln.Node.Name,
			Path:   path,
			Offset: ln.Offset,
			Length: one,
			Occurs: ln.Node.Occurs,
			Digits: ln.Node.Digits,
			Scale:  ln.Node.Scale,
		}
		if ln.Node.Group {
			it.Kind = "group"
			for _, c := range ln.Children {
				it.Children = append(it.Children, build(c, path))
			}
		} else {
			it.Kind = kindName(ln.Node.Kind)
		}
		return it
	}
	lj := layoutJSON{
		Record:       l.RecordName,
		RecordLength: l.RecordLength,
		Items:        []itemJSON{build(l.Root, "")},
	}
	return json.MarshalIndent(lj, "", "  ")
}
