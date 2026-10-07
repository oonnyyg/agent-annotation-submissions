package vm

import (
	"encoding/json"
	"fmt"
)

// Op is an instruction opcode.
type Op string

const (
	OpPushInt  Op = "push_int"  // push int constant
	OpPushBool Op = "push_bool" // push bool constant
	OpPop      Op = "pop"       // discard top of stack
	OpDup      Op = "dup"       // duplicate top of stack
	OpSwap     Op = "swap"      // swap top two values
	OpLoad     Op = "load"      // push local variable
	OpStore    Op = "store"     // pop into local variable
	OpAdd      Op = "add"       // int arithmetic
	OpSub      Op = "sub"
	OpMul      Op = "mul"
	OpDiv      Op = "div" // truncates toward zero
	OpMod      Op = "mod"
	OpEq       Op = "eq" // int comparisons, push bool
	OpNe       Op = "ne"
	OpLt       Op = "lt"
	OpLe       Op = "le"
	OpGt       Op = "gt"
	OpGe       Op = "ge"
	OpNot      Op = "not"        // boolean negation
	OpJmp      Op = "jmp"        // unconditional jump
	OpJmpIf    Op = "jmp_if"     // pop bool, jump if true
	OpJmpIfNot Op = "jmp_if_not" // pop bool, jump if false
	OpHalt     Op = "halt"       // end; stack must hold exactly one value
)

// Instr is a single bytecode instruction.
type Instr struct {
	Op     Op     `json:"op"`
	Value  *int64 `json:"value,omitempty"`  // push_int operand
	Bool   *bool  `json:"bool,omitempty"`   // push_bool operand
	Var    int    `json:"var,omitempty"`    // load/store local index
	Target int    `json:"target,omitempty"` // jump target instruction index
}

// Program is a statically checked rule program.
type Program struct {
	Locals []Type  `json:"locals"` // pre-declared local variable types
	Code   []Instr `json:"code"`
}

// InputFile supplies initial values for locals. A null entry leaves the
// local unassigned; the static analyzer then requires every read path to
// store a value first.
type InputFile struct {
	Locals []*Value `json:"locals"`
}

// valueJSON is the wire form of a Value: {"type":"int","value":5} or
// {"type":"bool","value":true}.
type valueJSON struct {
	Type Type   `json:"type"`
	Int  *int64 `json:"int,omitempty"`
	Bool *bool  `json:"bool,omitempty"`
}

func (v *Value) UnmarshalJSON(data []byte) error {
	var w valueJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	switch w.Type {
	case TypeInt:
		if w.Int == nil {
			return fmt.Errorf("int value missing \"int\" field")
		}
		*v = Int(*w.Int)
	case TypeBool:
		if w.Bool == nil {
			return fmt.Errorf("bool value missing \"bool\" field")
		}
		*v = Bool(*w.Bool)
	default:
		return fmt.Errorf("unknown value type %q", w.Type)
	}
	return nil
}

func (v Value) MarshalJSON() ([]byte, error) {
	switch v.T {
	case TypeInt:
		return json.Marshal(valueJSON{Type: TypeInt, Int: &v.I})
	case TypeBool:
		return json.Marshal(valueJSON{Type: TypeBool, Bool: &v.B})
	}
	return nil, fmt.Errorf("cannot marshal invalid value")
}

// Validate performs structural checks that do not require flow analysis:
// operand presence, local indices, and jump target bounds.
func (p *Program) Validate() error {
	for i, l := range p.Locals {
		if l != TypeInt && l != TypeBool {
			return fmt.Errorf("local %d: unknown type %q", i, l)
		}
	}
	if len(p.Code) == 0 {
		return fmt.Errorf("program has no instructions")
	}
	for pc, in := range p.Code {
		switch in.Op {
		case OpPushInt:
			if in.Value == nil {
				return fmt.Errorf("pc %d: push_int requires \"value\"", pc)
			}
		case OpPushBool:
			if in.Bool == nil {
				return fmt.Errorf("pc %d: push_bool requires \"bool\"", pc)
			}
		case OpLoad, OpStore:
			if in.Var < 0 || in.Var >= len(p.Locals) {
				return fmt.Errorf("pc %d: %s local index %d out of range (have %d locals)", pc, in.Op, in.Var, len(p.Locals))
			}
		case OpJmp, OpJmpIf, OpJmpIfNot:
			if in.Target < 0 || in.Target >= len(p.Code) {
				return fmt.Errorf("pc %d: %s target %d out of range [0,%d)", pc, in.Op, in.Target, len(p.Code))
			}
		case OpPop, OpDup, OpSwap, OpAdd, OpSub, OpMul, OpDiv, OpMod,
			OpEq, OpNe, OpLt, OpLe, OpGt, OpGe, OpNot, OpHalt:
		default:
			return fmt.Errorf("pc %d: unknown op %q", pc, in.Op)
		}
	}
	return nil
}
