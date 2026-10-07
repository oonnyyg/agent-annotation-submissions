package copybook

import "encoding/json"

// Node is a compiled layout node with absolute offsets in the record.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Pos      Pos     `json:"-"`
	Offset   int     `json:"offset"`
	Length   int     `json:"length"`
	Count    int     `json:"count"`
	Kind     string  `json:"kind"`
	Scale    int     `json:"scale,omitempty"`
	Signed   bool    `json:"signed,omitempty"`
	Digits   int     `json:"digits,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

// Layout is a compiled record layout.
type Layout struct {
	Root   *Node
	Length int
}

// LayoutJSON renders the layout as indented JSON.
func (l *Layout) LayoutJSON() ([]byte, error) {
	return json.MarshalIndent(struct {
		RecordLength int   `json:"recordLength"`
		Root         *Node `json:"root"`
	}{l.Length, l.Root}, "", "  ")
}

// Compile validates the parsed fields and computes offsets and lengths.
func Compile(fields []*Field) (*Layout, error) {
	if len(fields) == 0 {
		return nil, errAt(Pos{1, 1}, "empty copybook")
	}
	if fields[0].Level != 1 {
		return nil, errAt(fields[0].Pos, "first entry must be level 01, got level %02d", fields[0].Level)
	}
	for _, f := range fields[1:] {
		if f.Level == 1 {
			return nil, errAt(f.Pos, "multiple level-01 records are not supported")
		}
	}
	root, err := buildNode(fields[0], fields[1:], "")
	if err != nil {
		return nil, err
	}
	off := 0
	if err := assignOffsets(root, &off); err != nil {
		return nil, err
	}
	return &Layout{Root: root, Length: off}, nil
}

func buildNode(f *Field, rest []*Field, parentPath string) (*Node, error) {
	n := &Node{Name: f.Name, Pos: f.Pos, Count: 1}
	if f.Occurs > 0 {
		n.Count = f.Occurs
	}
	if parentPath == "" {
		n.Path = f.Name
	} else {
		n.Path = parentPath + "." + f.Name
	}
	if f.Level == 1 && f.Occurs > 0 {
		return nil, errAt(f.Pos, "level-01 record cannot have OCCURS")
	}

	var kids []*Field
	for _, r := range rest {
		if r.Level > f.Level {
			kids = append(kids, r)
		}
	}
	if len(kids) > 0 {
		if f.HasPic {
			return nil, errAt(f.Pos, "field %s has both PIC and subordinate items", f.Name)
		}
		minLevel := kids[0].Level
		seen := map[string]Pos{}
		i := 0
		for i < len(kids) {
			k := kids[i]
			if k.Level != minLevel {
				return nil, errAt(k.Pos, "level %02d does not match sibling level %02d under %s", k.Level, minLevel, f.Name)
			}
			if prev, dup := seen[k.Name]; dup {
				return nil, errAt(k.Pos, "duplicate field name %s (first defined at %s)", k.Name, prev)
			}
			seen[k.Name] = k.Pos
			j := i + 1
			for j < len(kids) && kids[j].Level > minLevel {
				j++
			}
			child, err := buildNode(k, kids[i+1:j], n.Path)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
			i = j
		}
		n.Kind = "group"
		return n, nil
	}

	if !f.HasPic {
		return nil, errAt(f.Pos, "field %s has no PIC clause and no subordinate items", f.Name)
	}
	p := f.Pic
	switch p.Kind {
	case PicText:
		if p.Comp3 {
			return nil, errAt(f.Pos, "text field %s cannot be COMP-3", f.Name)
		}
		n.Kind = "text"
		n.Length = p.Length
	case PicNumeric:
		n.Scale = p.Scale
		n.Digits = p.Length
		n.Signed = p.Signed
		if p.Comp3 {
			if !p.Signed {
				return nil, errAt(f.Pos, "COMP-3 field %s must be signed (PIC S9...)", f.Name)
			}
			n.Kind = "comp3"
		} else {
			if p.Signed {
				return nil, errAt(f.Pos, "signed display numeric %s is not supported; use COMP-3", f.Name)
			}
			n.Kind = "display"
		}
	}
	return n, nil
}

func elemLength(n *Node) int {
	switch n.Kind {
	case "text":
		return n.Length
	case "display":
		return n.Digits + n.Scale
	case "comp3":
		return (n.Digits + n.Scale + 2) / 2
	}
	return 0
}

func assignOffsets(n *Node, off *int) error {
	n.Offset = *off
	if n.Kind == "group" {
		for _, c := range n.Children {
			if err := assignOffsets(c, off); err != nil {
				return err
			}
		}
		repLen := *off - n.Offset
		n.Length = repLen * n.Count
		*off = n.Offset + n.Length
	} else {
		n.Length = elemLength(n)
		*off += n.Length * n.Count
	}
	return nil
}
