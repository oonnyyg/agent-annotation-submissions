// Package bencode decodes bencoded data while recording the raw byte
// offsets of every value so callers can hash original byte slices.
package bencode

import (
	"errors"
	"fmt"
	"sort"
)

// Kind identifies the type of a decoded value.
type Kind int

const (
	String Kind = iota
	Int
	List
	Dict
)

// Value is a decoded bencode value. Start/End are byte offsets into the
// original input; End is exclusive.
type Value struct {
	Kind  Kind
	Start int
	End   int

	Str  []byte   // String
	Int  int64    // Int
	List []*Value // List
	Dict []Pair   // Dict, in wire order
}

// Pair is one dictionary entry.
type Pair struct {
	Key   []byte
	Value *Value
}

// Error describes a decode failure with a byte offset.
type Error struct {
	Offset int
	Msg    string
}

func (e *Error) Error() string { return fmt.Sprintf("bencode: offset %d: %s", e.Offset, e.Msg) }

// Decode parses data as a single bencode value and rejects trailing bytes.
func Decode(data []byte) (*Value, error) {
	p := &parser{data: data}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	if p.pos != len(data) {
		return nil, &Error{p.pos, "trailing data after top-level value"}
	}
	return v, nil
}

type parser struct {
	data []byte
	pos  int
}

func (p *parser) value() (*Value, error) {
	if p.pos >= len(p.data) {
		return nil, &Error{p.pos, "unexpected end of input"}
	}
	c := p.data[p.pos]
	switch {
	case c == 'i':
		return p.integer()
	case c == 'l':
		return p.list()
	case c == 'd':
		return p.dict()
	case c >= '0' && c <= '9':
		return p.str()
	default:
		return nil, &Error{p.pos, fmt.Sprintf("unexpected byte %q", c)}
	}
}

func (p *parser) integer() (*Value, error) {
	start := p.pos
	p.pos++ // 'i'
	end := p.pos
	for end < len(p.data) && p.data[end] != 'e' {
		end++
	}
	if end >= len(p.data) {
		return nil, &Error{start, "unterminated integer"}
	}
	digits := p.data[p.pos:end]
	p.pos = end + 1
	if len(digits) == 0 {
		return nil, &Error{start, "empty integer"}
	}
	if digits[0] == '-' {
		if len(digits) == 1 {
			return nil, &Error{start, "integer sign without digits"}
		}
		if digits[1] == '0' {
			return nil, &Error{start, "negative integer with leading zero"}
		}
	} else if len(digits) > 1 && digits[0] == '0' {
		return nil, &Error{start, "integer with leading zero"}
	}
	var n int64
	neg := digits[0] == '-'
	body := digits
	if neg {
		body = digits[1:]
	}
	for _, b := range body {
		if b < '0' || b > '9' {
			return nil, &Error{start, "non-digit in integer"}
		}
		d := int64(b - '0')
		if n > (1<<63-1-d)/10 {
			return nil, &Error{start, "integer overflow"}
		}
		n = n*10 + d
	}
	if neg {
		n = -n
	}
	return &Value{Kind: Int, Start: start, End: p.pos, Int: n}, nil
}

func (p *parser) str() (*Value, error) {
	start := p.pos
	colon := p.pos
	for colon < len(p.data) && p.data[colon] != ':' {
		b := p.data[colon]
		if b < '0' || b > '9' {
			return nil, &Error{start, "invalid string length"}
		}
		colon++
	}
	if colon >= len(p.data) {
		return nil, &Error{start, "unterminated string length"}
	}
	digits := p.data[p.pos:colon]
	if len(digits) > 1 && digits[0] == '0' {
		return nil, &Error{start, "string length with leading zero"}
	}
	var n int64
	for _, b := range digits {
		n = n*10 + int64(b-'0')
		if n > int64(len(p.data)) {
			return nil, &Error{start, "string length exceeds input"}
		}
	}
	p.pos = colon + 1
	if int64(p.pos)+n > int64(len(p.data)) {
		return nil, &Error{start, "truncated string"}
	}
	s := p.data[p.pos : p.pos+int(n)]
	p.pos += int(n)
	return &Value{Kind: String, Start: start, End: p.pos, Str: s}, nil
}

func (p *parser) list() (*Value, error) {
	start := p.pos
	p.pos++ // 'l'
	v := &Value{Kind: List, Start: start}
	for {
		if p.pos >= len(p.data) {
			return nil, &Error{start, "unterminated list"}
		}
		if p.data[p.pos] == 'e' {
			p.pos++
			v.End = p.pos
			return v, nil
		}
		item, err := p.value()
		if err != nil {
			return nil, err
		}
		v.List = append(v.List, item)
	}
}

func (p *parser) dict() (*Value, error) {
	start := p.pos
	p.pos++ // 'd'
	v := &Value{Kind: Dict, Start: start}
	for {
		if p.pos >= len(p.data) {
			return nil, &Error{start, "unterminated dictionary"}
		}
		if p.data[p.pos] == 'e' {
			p.pos++
			v.End = p.pos
			return v, nil
		}
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		val, err := p.value()
		if err != nil {
			return nil, err
		}
		v.Dict = append(v.Dict, Pair{Key: key.Str, Value: val})
	}
}

// Get returns the value for key, or nil if absent.
func (v *Value) Get(key string) *Value {
	if v.Kind != Dict {
		return nil
	}
	for _, p := range v.Dict {
		if string(p.Key) == key {
			return p.Value
		}
	}
	return nil
}

// DuplicateKeys reports keys that appear more than once in any dictionary
// reachable from v.
func DuplicateKeys(v *Value) []string {
	var dups []string
	var walk func(n *Value, path string)
	walk = func(n *Value, path string) {
		switch n.Kind {
		case Dict:
			seen := map[string]bool{}
			for _, p := range n.Dict {
				k := string(p.Key)
				if seen[k] {
					dups = append(dups, path+"/"+k)
				}
				seen[k] = true
				walk(p.Value, path+"/"+k)
			}
		case List:
			for i, item := range n.List {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(v, "")
	return dups
}

// UnsortedKeys reports dictionaries whose keys are not in sorted order.
func UnsortedKeys(v *Value) []string {
	var out []string
	var walk func(n *Value, path string)
	walk = func(n *Value, path string) {
		switch n.Kind {
		case Dict:
			keys := make([]string, 0, len(n.Dict))
			for _, p := range n.Dict {
				keys = append(keys, string(p.Key))
			}
			if !sort.StringsAreSorted(keys) {
				out = append(out, path)
			}
			for _, p := range n.Dict {
				walk(p.Value, path+"/"+string(p.Key))
			}
		case List:
			for i, item := range n.List {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(v, "")
	return out
}

// ErrType is returned on type mismatches by the accessor helpers.
var ErrType = errors.New("bencode: unexpected value type")
