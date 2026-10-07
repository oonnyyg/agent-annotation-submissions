package copybook

import "fmt"

// DecodeRecord converts one fixed-length record into a JSON-ready value
// (map[string]any with string leaves).
func (l *Layout) DecodeRecord(rec []byte, recNo int) (map[string]any, error) {
	if len(rec) != l.Length {
		return nil, &RecordError{recNo, l.Root.Name,
			fmt.Sprintf("record is %d bytes, expected %d", len(rec), l.Length)}
	}
	v, err := decodeGroup(l.Root, rec, recNo)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

func decodeGroup(n *Node, rec []byte, recNo int) (any, error) {
	repLen := n.Length / n.Count
	if n.Count > 1 {
		arr := make([]any, n.Count)
		for i := 0; i < n.Count; i++ {
			m, err := decodeGroupBody(n, rec, n.Offset+i*repLen, recNo, i)
			if err != nil {
				return nil, err
			}
			arr[i] = m
		}
		return arr, nil
	}
	return decodeGroupBody(n, rec, n.Offset, recNo, -1)
}

func decodeGroupBody(n *Node, rec []byte, base, recNo, idx int) (map[string]any, error) {
	m := make(map[string]any, len(n.Children))
	for _, c := range n.Children {
		off := base + (c.Offset - n.Offset)
		v, err := decodeChild(c, rec, off, recNo, idx)
		if err != nil {
			return nil, err
		}
		m[c.Name] = v
	}
	return m, nil
}

func decodeChild(n *Node, rec []byte, off, recNo, parentIdx int) (any, error) {
	if n.Kind == "group" {
		repLen := n.Length / n.Count
		if n.Count > 1 {
			arr := make([]any, n.Count)
			for i := 0; i < n.Count; i++ {
				m, err := decodeGroupBody(n, rec, off+i*repLen, recNo, i)
				if err != nil {
					return nil, err
				}
				arr[i] = m
			}
			return arr, nil
		}
		return decodeGroupBody(n, rec, off, recNo, -1)
	}
	if n.Count > 1 {
		arr := make([]any, n.Count)
		for i := 0; i < n.Count; i++ {
			v, err := decodeLeaf(n, rec[off+i*n.Length:off+(i+1)*n.Length], recNo, i)
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	}
	return decodeLeaf(n, rec[off:off+n.Length], recNo, parentIdx)
}

func leafPath(n *Node, idx int) string {
	if idx >= 0 {
		return fmt.Sprintf("%s[%d]", n.Path, idx)
	}
	return n.Path
}

func decodeLeaf(n *Node, b []byte, recNo, idx int) (string, error) {
	var v string
	var err error
	switch n.Kind {
	case "text":
		v, err = decodeText(b)
	case "display":
		v, err = decodeDisplay(b, n)
	case "comp3":
		v, err = decodeComp3(b, n)
	}
	if err != nil {
		return "", &RecordError{recNo, leafPath(n, idx), err.Error()}
	}
	return v, nil
}

// EncodeRecord converts a decoded JSON object back into a fixed-length record.
func (l *Layout) EncodeRecord(obj map[string]any, recNo int) ([]byte, error) {
	rec := make([]byte, l.Length)
	if err := encodeGroupBody(l.Root, obj, rec, 0, recNo); err != nil {
		return nil, err
	}
	return rec, nil
}

func encodeGroupBody(n *Node, obj any, rec []byte, base, recNo int) error {
	m, ok := obj.(map[string]any)
	if !ok {
		return &RecordError{recNo, n.Path, "expected JSON object for group"}
	}
	for k := range m {
		if findChild(n, k) == nil {
			return &RecordError{recNo, n.Path, fmt.Sprintf("unknown field %q", k)}
		}
	}
	for _, c := range n.Children {
		v, present := m[c.Name]
		if !present {
			return &RecordError{recNo, c.Path, "missing field"}
		}
		off := base + (c.Offset - n.Offset)
		if err := encodeChild(c, v, rec, off, recNo); err != nil {
			return err
		}
	}
	return nil
}

func findChild(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func encodeChild(n *Node, v any, rec []byte, off, recNo int) error {
	if n.Count > 1 {
		arr, ok := v.([]any)
		if !ok {
			return &RecordError{recNo, n.Path, "expected JSON array"}
		}
		if len(arr) != n.Count {
			return &RecordError{recNo, n.Path,
				fmt.Sprintf("array has %d elements, OCCURS requires %d", len(arr), n.Count)}
		}
		repLen := n.Length
		if n.Kind == "group" {
			repLen = n.Length / n.Count
		}
		for i := 0; i < n.Count; i++ {
			sub := *n
			sub.Count = 1
			sub.Path = fmt.Sprintf("%s[%d]", n.Path, i)
			if err := encodeChild(&sub, arr[i], rec, off+i*repLen, recNo); err != nil {
				return err
			}
		}
		return nil
	}
	if n.Kind == "group" {
		return encodeGroupBody(n, v, rec, off, recNo)
	}
	s, ok := v.(string)
	if !ok {
		return &RecordError{recNo, n.Path, "expected JSON string value"}
	}
	var b []byte
	var err error
	switch n.Kind {
	case "text":
		b, err = encodeText(s, n.Length)
	case "display":
		b, err = encodeDisplay(s, n)
	case "comp3":
		b, err = encodeComp3(s, n)
	}
	if err != nil {
		return &RecordError{recNo, n.Path, err.Error()}
	}
	copy(rec[off:off+n.Length], b)
	return nil
}
