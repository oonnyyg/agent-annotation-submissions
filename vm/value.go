package vm

import (
	"encoding/json"
	"fmt"
)

// Type is the static type of a value or local variable.
type Type int

const (
	TypeInt Type = iota
	TypeBool
)

func (t Type) String() string {
	if t == TypeInt {
		return "int"
	}
	return "bool"
}

// Value is a tagged union: either a signed 64-bit integer or a boolean.
type Value struct {
	T Type
	I int64
	B bool
}

func Int(v int64) Value { return Value{T: TypeInt, I: v} }
func Bool(v bool) Value { return Value{T: TypeBool, B: v} }

func (v Value) String() string {
	if v.T == TypeInt {
		return fmt.Sprintf("int(%d)", v.I)
	}
	return fmt.Sprintf("bool(%t)", v.B)
}

// MarshalJSON renders values as {"type":"int","value":5} style objects.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.T == TypeInt {
		return json.Marshal(struct {
			Type  string `json:"type"`
			Value int64  `json:"value"`
		}{"int", v.I})
	}
	return json.Marshal(struct {
		Type  string `json:"type"`
		Value bool   `json:"value"`
	}{"bool", v.B})
}

// UnmarshalJSON accepts {"type":"int","value":5} or {"type":"bool","value":true}.
func (v *Value) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("value must be an object with type and value: %w", err)
	}
	switch raw.Type {
	case "int":
		var n int64
		if err := json.Unmarshal(raw.Value, &n); err != nil {
			return fmt.Errorf("int value must be a 64-bit integer: %w", err)
		}
		*v = Int(n)
	case "bool":
		var b bool
		if err := json.Unmarshal(raw.Value, &b); err != nil {
			return fmt.Errorf("bool value must be true or false: %w", err)
		}
		*v = Bool(b)
	default:
		return fmt.Errorf("unknown value type %q (want \"int\" or \"bool\")", raw.Type)
	}
	return nil
}
