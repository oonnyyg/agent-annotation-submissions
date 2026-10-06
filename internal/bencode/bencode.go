// Package bencode decodes bencoded data while retaining the raw byte
// offsets of every value, so callers can hash original byte slices
// (e.g. the info dictionary) without re-encoding.
package bencode

import (
	"errors"
	"fmt"
	"sort"
)

// Kind identifies the type of a decoded value.
type Kind int

const (
	Int Kind = iota
	String
	List
	Dict
)

// Value is a decoded bencode value. Start/End are byte offsets into the
// original input covering the raw encoding of this value.
type Value struct {
	Kind  Kind
	Start int
	End   int

	Int  int64
	Str  []byte
	List []*Value
	Dict []Pair
}

// Pair is one key/value entry of a dictionary, in file order.
type Pair struct {
	Key   []byte
	Value *Value
}

// Get returns the value for key, or nil.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != Dict {
		return nil
	}
	for _, p := range v.Dict {
		if string(p.Key) == key {
			return p.Value
		}
	}
	return nil
}

// UnsortedKeys reports whether dictionary keys are not in sorted order.
func (v *Value) UnsortedKeys() bool {
	if v.Kind != Dict {
		return false
	}
	return !sort.IsSorted(sortedKeys(v.Dict))
}

type sortedKeys []Pair

func (s sortedKeys) Len() int           { return len(s) }
func (s sortedKeys) Less(i, j int) bool { return string(s[i].Key) < string(s[j].Key) }
func (s sortedKeys) Swap(i, j int)      {}

// Decode parses data as a single bencode value. Trailing bytes are an error.
func Decode(data []byte) (*Value, error) {
	v, next, err := decode(data, 0)
	if err != nil {
		return nil, err
	}
	if next != len(data) {
		return nil, fmt.Errorf("bencode: %d trailing bytes after top-level value", len(data)-next)
	}
	return v, nil
}

func decode(data []byte, pos int) (*Value, int, error) {
	if pos >= len(data) {
		return nil, 0, errors.New("bencode: unexpected end of data")
	}
	switch c := data[pos]; {
	case c == 'i':
		return decodeInt(data, pos)
	case c == 'l':
		return decodeList(data, pos)
	case c == 'd':
		return decodeDict(data, pos)
	case c >= '0' && c <= '9':
		return decodeString(data, pos)
	default:
		return nil, 0, fmt.Errorf("bencode: invalid type byte %q at offset %d", c, pos)
	}
}

func decodeInt(data []byte, pos int) (*Value, int, error) {
	end := pos + 1
	for end < len(data) && data[end] != 'e' {
		end++
	}
	if end >= len(data) {
		return nil, 0, fmt.Errorf("bencode: unterminated integer at offset %d", pos)
	}
	raw := data[pos+1 : end]
	if len(raw) == 0 {
		return nil, 0, fmt.Errorf("bencode: empty integer at offset %d", pos)
	}
	if raw[0] == '-' {
		if len(raw) == 1 {
			return nil, 0, fmt.Errorf("bencode: lone minus sign at offset %d", pos)
		}
		if raw[1] == '0' {
			return nil, 0, fmt.Errorf("bencode: negative zero / leading zero at offset %d", pos)
		}
	} else if raw[0] == '0' && len(raw) > 1 {
		return nil, 0, fmt.Errorf("bencode: integer with leading zero at offset %d", pos)
	}
	var n int64
	for _, c := range raw {
		if c == '-' {
			continue
		}
		if c < '0' || c > '9' {
			return nil, 0, fmt.Errorf("bencode: non-digit in integer at offset %d", pos)
		}
		n = n*10 + int64(c-'0')
	}
	if raw[0] == '-' {
		n = -n
	}
	return &Value{Kind: Int, Start: pos, End: end + 1, Int: n}, end + 1, nil
}

func decodeString(data []byte, pos int) (*Value, int, error) {
	i := pos
	for i < len(data) && data[i] != ':' {
		if data[i] < '0' || data[i] > '9' {
			return nil, 0, fmt.Errorf("bencode: invalid string length at offset %d", pos)
		}
		i++
	}
	if i >= len(data) {
		return nil, 0, fmt.Errorf("bencode: unterminated string length at offset %d", pos)
	}
	rawLen := data[pos:i]
	if len(rawLen) > 1 && rawLen[0] == '0' {
		return nil, 0, fmt.Errorf("bencode: string length with leading zero at offset %d", pos)
	}
	var length int64
	for _, c := range rawLen {
		length = length*10 + int64(c-'0')
		if length > int64(len(data)) {
			return nil, 0, fmt.Errorf("bencode: string length overflows input at offset %d", pos)
		}
	}
	start := i + 1
	end := start + int(length)
	if end > len(data) {
		return nil, 0, fmt.Errorf("bencode: truncated string at offset %d", pos)
	}
	return &Value{Kind: String, Start: pos, End: end, Str: data[start:end]}, end, nil
}

func decodeList(data []byte, pos int) (*Value, int, error) {
	v := &Value{Kind: List, Start: pos}
	i := pos + 1
	for {
		if i >= len(data) {
			return nil, 0, fmt.Errorf("bencode: unterminated list at offset %d", pos)
		}
		if data[i] == 'e' {
			v.End = i + 1
			return v, i + 1, nil
		}
		item, next, err := decode(data, i)
		if err != nil {
			return nil, 0, err
		}
		v.List = append(v.List, item)
		i = next
	}
}

func decodeDict(data []byte, pos int) (*Value, int, error) {
	v := &Value{Kind: Dict, Start: pos}
	i := pos + 1
	for {
		if i >= len(data) {
			return nil, 0, fmt.Errorf("bencode: unterminated dict at offset %d", pos)
		}
		if data[i] == 'e' {
			v.End = i + 1
			return v, i + 1, nil
		}
		key, next, err := decodeString(data, i)
		if err != nil {
			return nil, 0, err
		}
		val, next2, err := decode(data, next)
		if err != nil {
			return nil, 0, err
		}
		for _, p := range v.Dict {
			if string(p.Key) == string(key.Str) {
				return nil, 0, fmt.Errorf("bencode: duplicate key %q at offset %d", key.Str, i)
			}
		}
		v.Dict = append(v.Dict, Pair{Key: key.Str, Value: val})
		i = next2
	}
}
