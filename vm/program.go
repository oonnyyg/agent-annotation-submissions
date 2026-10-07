package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Op is an instruction opcode.
type Op string

const (
	OpPushInt  Op = "push_int"  // arg: int64 literal
	OpPushBool Op = "push_bool" // arg: bool literal
	OpDup      Op = "dup"       // duplicate top of stack
	OpPop      Op = "pop"       // discard top of stack
	OpSwap     Op = "swap"      // swap top two stack values
	OpLoad     Op = "load"      // arg: local index, push local
	OpStore    Op = "store"     // arg: local index, pop into local
	OpAdd      Op = "add"       // int arithmetic
	OpSub      Op = "sub"
	OpMul      Op = "mul"
	OpDiv      Op = "div" // truncates toward zero
	OpEq       Op = "eq"  // comparisons push bool
	OpNe       Op = "ne"
	OpLt       Op = "lt"
	OpLe       Op = "le"
	OpGt       Op = "gt"
	OpGe       Op = "ge"
	OpJmp      Op = "jmp"        // arg: absolute target pc
	OpJmpIf    Op = "jmp_if"     // arg: target; pop bool, jump if true
	OpJmpIfNot Op = "jmp_if_not" // arg: target; pop bool, jump if false
	OpHalt     Op = "halt"       // end; stack must hold exactly one value
)

// Instr is a single decoded instruction.
type Instr struct {
	Op  Op    `json:"op"`
	Arg int64 `json:"arg,omitempty"`
}

// Local declares a typed local variable slot.
type Local struct {
	Name string `json:"name"`
	Type string `json:"type"` // "int" or "bool"
}

// Program is a validated-once, executable rule program.
type Program struct {
	Params []string `json:"params"` // expected input stack types, bottom-first
	Locals []Local  `json:"locals"`
	Code   []Instr  `json:"code"`
}

// ParamTypes returns the declared input stack types.
func (p *Program) ParamTypes() ([]Type, error) {
	ts := make([]Type, len(p.Params))
	for i, s := range p.Params {
		switch s {
		case "int":
			ts[i] = TypeInt
		case "bool":
			ts[i] = TypeBool
		default:
			return nil, fmt.Errorf("param %d: unknown type %q (want \"int\" or \"bool\")", i, s)
		}
	}
	return ts, nil
}

// CheckInput verifies that a runtime input stack matches the declared
// params (length and types, bottom-first).
func (p *Program) CheckInput(input []Value) error {
	ts, err := p.ParamTypes()
	if err != nil {
		return err
	}
	if len(input) != len(ts) {
		return fmt.Errorf("input stack has %d value(s), program declares %d param(s)", len(input), len(ts))
	}
	for i, v := range input {
		if v.T != ts[i] {
			return fmt.Errorf("input stack slot %d: got %s, want %s", i, v.T, ts[i])
		}
	}
	return nil
}

// ParseProgram decodes JSON and runs full static validation.
func ParseProgram(data []byte) (*Program, error) {
	var p Program
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("invalid program JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func localType(l Local) (Type, error) {
	switch l.Type {
	case "int":
		return TypeInt, nil
	case "bool":
		return TypeBool, nil
	}
	return 0, fmt.Errorf("local %q: unknown type %q (want \"int\" or \"bool\")", l.Name, l.Type)
}
