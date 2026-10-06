// Package ruleaudit implements static analysis of ordered decision-table
// rules over finite enum or bounded integer fields.
package ruleaudit

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// FieldKind is the kind of a field domain.
type FieldKind string

const (
	KindEnum FieldKind = "enum"
	KindInt  FieldKind = "int"
)

// Field declares one input field and its finite domain.
type Field struct {
	Name   string    `json:"name"`
	Type   FieldKind `json:"type"`
	Values []string  `json:"values,omitempty"` // enum, in declaration order
	Min    *int      `json:"min,omitempty"`    // int, inclusive bounds
	Max    *int      `json:"max,omitempty"`
}

// IntCond is an integer interval condition with open/closed endpoints.
// A nil bound means unbounded (clamped to the field domain).
type IntCond struct {
	Min          *int `json:"min,omitempty"`
	MinExclusive bool `json:"minExclusive,omitempty"`
	Max          *int `json:"max,omitempty"`
	MaxExclusive bool `json:"maxExclusive,omitempty"`
}

// Condition is the condition on one field inside a rule.
// Exactly one of Values (enum set) or Range (int interval) is used.
type Condition struct {
	Values []string `json:"values,omitempty"`
	Range  *IntCond `json:"range,omitempty"`
}

// UnmarshalJSON accepts either a shorthand enum set (["a","b"]) or an
// object form: {"values":[...]} for enums, or an int range like
// {"min":1,"max":5,"maxExclusive":true}.
func (c *Condition) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal(data, &c.Values)
	}
	var obj struct {
		Values       []string `json:"values"`
		Min          *int     `json:"min"`
		MinExclusive bool     `json:"minExclusive"`
		Max          *int     `json:"max"`
		MaxExclusive bool     `json:"maxExclusive"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&obj); err != nil {
		return err
	}
	c.Values = obj.Values
	if obj.Min != nil || obj.Max != nil {
		c.Range = &IntCond{Min: obj.Min, MinExclusive: obj.MinExclusive, Max: obj.Max, MaxExclusive: obj.MaxExclusive}
	}
	return nil
}

// Rule is one ordered rule: a conjunction of field conditions.
// Fields absent from When are unrestricted.
type Rule struct {
	Name   string               `json:"name,omitempty"`
	When   map[string]Condition `json:"when,omitempty"`
	Output json.RawMessage      `json:"output,omitempty"`
}

// Spec is the full analyzer input.
type Spec struct {
	Fields []Field `json:"fields"`
	Rules  []Rule  `json:"rules"`
}

// LoadSpec decodes and validates a spec from r.
func LoadSpec(r io.Reader) (*Spec, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

// Validate checks the spec, rejecting unknown values and out-of-domain
// conditions. Errors are located to the rule index where applicable.
func (s *Spec) Validate() error {
	if len(s.Fields) == 0 {
		return fmt.Errorf("spec: no fields declared")
	}
	seen := map[string]bool{}
	for i, f := range s.Fields {
		if f.Name == "" {
			return fmt.Errorf("field %d: empty name", i)
		}
		if seen[f.Name] {
			return fmt.Errorf("field %q: duplicate declaration", f.Name)
		}
		seen[f.Name] = true
		switch f.Type {
		case KindEnum:
			if len(f.Values) == 0 {
				return fmt.Errorf("field %q: enum requires at least one value", f.Name)
			}
			vseen := map[string]bool{}
			for _, v := range f.Values {
				if vseen[v] {
					return fmt.Errorf("field %q: duplicate enum value %q", f.Name, v)
				}
				vseen[v] = true
			}
		case KindInt:
			if f.Min == nil || f.Max == nil {
				return fmt.Errorf("field %q: int requires min and max", f.Name)
			}
			if *f.Min > *f.Max {
				return fmt.Errorf("field %q: empty domain [%d, %d]", f.Name, *f.Min, *f.Max)
			}
		default:
			return fmt.Errorf("field %q: unknown type %q", f.Name, f.Type)
		}
	}
	for ri := range s.Rules {
		if err := s.validateRule(ri, &s.Rules[ri]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Spec) validateRule(ri int, rule *Rule) error {
	for fname, cond := range rule.When {
		f := s.FieldByName(fname)
		if f == nil {
			return fmt.Errorf("rule %d: condition on unknown field %q", ri, fname)
		}
		switch f.Type {
		case KindEnum:
			if cond.Range != nil {
				return fmt.Errorf("rule %d: field %q is enum, range condition not allowed", ri, fname)
			}
			if len(cond.Values) == 0 {
				return fmt.Errorf("rule %d: field %q: empty enum condition set", ri, fname)
			}
			for _, v := range cond.Values {
				if !f.HasValue(v) {
					return fmt.Errorf("rule %d: field %q: unknown enum value %q", ri, fname, v)
				}
			}
		case KindInt:
			if len(cond.Values) > 0 {
				return fmt.Errorf("rule %d: field %q is int, enum set condition not allowed", ri, fname)
			}
			if cond.Range == nil {
				return fmt.Errorf("rule %d: field %q: int condition requires a range", ri, fname)
			}
			lo, hi, err := f.NormalizeRange(cond.Range)
			if err != nil {
				return fmt.Errorf("rule %d: field %q: %v", ri, fname, err)
			}
			if lo > hi {
				return fmt.Errorf("rule %d: field %q: empty condition interval", ri, fname)
			}
		}
	}
	return nil
}

// FieldByName returns the declared field or nil.
func (s *Spec) FieldByName(name string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

// HasValue reports whether v is in the enum domain.
func (f *Field) HasValue(v string) bool {
	for _, x := range f.Values {
		if x == v {
			return true
		}
	}
	return false
}

// NormalizeRange converts an IntCond to an inclusive [lo, hi] pair,
// rejecting bounds outside the field domain.
func (f *Field) NormalizeRange(c *IntCond) (lo, hi int, err error) {
	lo, hi = *f.Min, *f.Max
	if c.Min != nil {
		if *c.Min < *f.Min || *c.Min > *f.Max {
			return 0, 0, fmt.Errorf("min %d outside domain [%d, %d]", *c.Min, *f.Min, *f.Max)
		}
		lo = *c.Min
		if c.MinExclusive {
			lo++
		}
	}
	if c.Max != nil {
		if *c.Max < *f.Min || *c.Max > *f.Max {
			return 0, 0, fmt.Errorf("max %d outside domain [%d, %d]", *c.Max, *f.Min, *f.Max)
		}
		hi = *c.Max
		if c.MaxExclusive {
			hi--
		}
	}
	return lo, hi, nil
}
