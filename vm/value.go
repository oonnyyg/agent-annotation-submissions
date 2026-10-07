package vm

import "fmt"

// Type is the static type of a value or local variable.
type Type string

const (
	TypeInt  Type = "int"
	TypeBool Type = "bool"
)

// Value is a runtime value: either a signed 64-bit integer or a boolean.
type Value struct {
	T Type
	I int64
	B bool
}

func Int(v int64) Value { return Value{T: TypeInt, I: v} }
func Bool(v bool) Value { return Value{T: TypeBool, B: v} }

func (v Value) String() string {
	switch v.T {
	case TypeInt:
		return fmt.Sprintf("%d", v.I)
	case TypeBool:
		return fmt.Sprintf("%t", v.B)
	default:
		return "<invalid>"
	}
}
