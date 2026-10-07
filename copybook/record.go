package copybook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// DecodeRecord converts one fixed-length record into a JSON value tree
// (map[string]any / []any / string) mirroring the group structure.
func (l *Layout) DecodeRecord(rec []byte) (any, error) {
	if len(rec) != l.RecordLength {
		return nil, fmt.Errorf("record length %d, want %d", len(rec), l.RecordLength)
	}
	return l.decodeNode(l.Root, rec, l.Root.Node.Name)
}

func (l *Layout) decodeNode(ln *LayoutNode, rec []byte, path string) (any, error) {
	n := ln.Node
	one := ln.Length / n.Occurs
	base := ln.Offset
	decodeOne := func(off int) (any, error) {
		off += base
		if n.Group {
			obj := map[string]any{}
			for _, c := range ln.Children {
				childPath := path + "." + c.Node.Name
				sub := &LayoutNode{Node: c.Node, Offset: off + c.Offset, Length: c.Length, Children: c.Children}
				v, err := l.decodeNode(sub, rec, childPath)
				if err != nil {
					return nil, err
				}
				obj[c.Node.Name] = v
			}
			return obj, nil
		}
		v, err := decodeField(n, rec[off:off+one])
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", path, err)
		}
		return v, nil
	}
	if n.Occurs == 1 {
		return decodeOne(0)
	}
	arr := make([]any, n.Occurs)
	for k := 0; k < n.Occurs; k++ {
		v, err := decodeOne(k * one)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", path, k, err)
		}
		arr[k] = v
	}
	return arr, nil
}

// EncodeRecord converts a decoded JSON value tree back into a record.
// Numbers must be decimal strings; structure and array lengths must match
// the layout exactly.
func (l *Layout) EncodeRecord(v any) ([]byte, error) {
	rec := make([]byte, l.RecordLength)
	if err := l.encodeNode(l.Root, rec, 0, v, l.Root.Node.Name); err != nil {
		return nil, err
	}
	return rec, nil
}

func (l *Layout) encodeNode(ln *LayoutNode, rec []byte, abs int, v any, path string) error {
	n := ln.Node
	one := ln.Length / n.Occurs
	if n.Occurs > 1 {
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("field %s: expected JSON array of %d elements", path, n.Occurs)
		}
		if len(arr) != n.Occurs {
			return fmt.Errorf("field %s: array has %d elements, want %d", path, len(arr), n.Occurs)
		}
		for k := 0; k < n.Occurs; k++ {
			if err := l.encodeOne(ln, rec, abs+k*one, arr[k], fmt.Sprintf("%s[%d]", path, k)); err != nil {
				return err
			}
		}
		return nil
	}
	return l.encodeOne(ln, rec, abs, v, path)
}

func (l *Layout) encodeOne(ln *LayoutNode, rec []byte, abs int, v any, path string) error {
	n := ln.Node
	if n.Group {
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("field %s: expected JSON object", path)
		}
		for key := range obj {
			found := false
			for _, c := range ln.Children {
				if c.Node.Name == key {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("field %s: unknown key %q", path, key)
			}
		}
		for _, c := range ln.Children {
			cv, ok := obj[c.Node.Name]
			if !ok {
				return fmt.Errorf("field %s: missing key %q", path, c.Node.Name)
			}
			if err := l.encodeNode(c, rec, abs+c.Offset, cv, path+"."+c.Node.Name); err != nil {
				return err
			}
		}
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("field %s: expected JSON string (numbers travel as decimal strings)", path)
	}
	if err := encodeField(n, rec[abs:abs+ln.Length], s); err != nil {
		return fmt.Errorf("field %s: %w", path, err)
	}
	return nil
}

// MarshalJSONValue renders a decoded value tree with stable key order.
func MarshalJSONValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case string:
		b, _ := json.Marshal(t)
		buf.Write(b)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSON(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("cannot marshal value of type %T", v)
	}
	return nil
}
