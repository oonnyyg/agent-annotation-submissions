package vm

import "fmt"

// StaticError reports a problem found before execution. The program is
// rejected as a whole; nothing runs.
type StaticError struct {
	PC  int // -1 when not tied to one instruction
	Msg string
}

func (e *StaticError) Error() string {
	if e.PC >= 0 {
		return fmt.Sprintf("static error at pc=%d: %s", e.PC, e.Msg)
	}
	return "static error: " + e.Msg
}

func staticErr(pc int, format string, args ...any) *StaticError {
	return &StaticError{PC: pc, Msg: fmt.Sprintf(format, args...)}
}

// maxAnalysisStack bounds the abstract stack so loops that grow the stack
// without bound are rejected instead of looping forever in analysis.
const maxAnalysisStack = 4096

var opNames = map[Op]bool{
	OpPushInt: true, OpPushBool: true, OpDup: true, OpPop: true, OpSwap: true,
	OpLoad: true, OpStore: true,
	OpAdd: true, OpSub: true, OpMul: true, OpDiv: true,
	OpEq: true, OpNe: true, OpLt: true, OpLe: true, OpGt: true, OpGe: true,
	OpJmp: true, OpJmpIf: true, OpJmpIfNot: true, OpHalt: true,
}

// Validate performs structural checks plus reachable-control-flow type and
// definite-assignment analysis. It returns a *StaticError on failure.
func (p *Program) Validate() error {
	if len(p.Code) == 0 {
		return &StaticError{PC: -1, Msg: "program has no instructions"}
	}
	localTypes := make([]Type, len(p.Locals))
	seen := map[string]bool{}
	for i, l := range p.Locals {
		t, err := localType(l)
		if err != nil {
			return &StaticError{PC: -1, Msg: err.Error()}
		}
		if seen[l.Name] {
			return &StaticError{PC: -1, Msg: fmt.Sprintf("duplicate local name %q", l.Name)}
		}
		seen[l.Name] = true
		localTypes[i] = t
	}
	for pc, in := range p.Code {
		if err := p.checkInstr(pc, in); err != nil {
			return err
		}
	}
	paramTypes, err := p.ParamTypes()
	if err != nil {
		return &StaticError{PC: -1, Msg: err.Error()}
	}
	return p.analyze(localTypes, paramTypes)
}

// checkInstr validates one instruction's opcode and operand statically.
func (p *Program) checkInstr(pc int, in Instr) error {
	if !opNames[in.Op] {
		return staticErr(pc, "unknown opcode %q", in.Op)
	}
	switch in.Op {
	case OpPushBool:
		if in.Arg != 0 && in.Arg != 1 {
			return staticErr(pc, "push_bool arg must be 0 or 1, got %d", in.Arg)
		}
	case OpLoad, OpStore:
		if in.Arg < 0 || in.Arg >= int64(len(p.Locals)) {
			return staticErr(pc, "%s local index %d out of range (have %d locals)", in.Op, in.Arg, len(p.Locals))
		}
	case OpJmp, OpJmpIf, OpJmpIfNot:
		if in.Arg < 0 || in.Arg >= int64(len(p.Code)) {
			return staticErr(pc, "%s target %d out of range (code length %d)", in.Op, in.Arg, len(p.Code))
		}
	}
	return nil
}

// absState is the abstract interpreter state at a program point.
type absState struct {
	stack    []Type // abstract stack types, bottom-first
	assigned []bool // definite assignment per local
}

// analyze runs a forward dataflow analysis over reachable control flow.
// At merge points the stack must match exactly (height and types); the
// assigned set is intersected so a local is readable only when every
// incoming path assigned it.
func (p *Program) analyze(localTypes, paramTypes []Type) error {
	n := len(p.Code)
	states := make([]*absState, n)
	states[0] = &absState{stack: append([]Type(nil), paramTypes...), assigned: make([]bool, len(p.Locals))}
	worklist := []int{0}

	for len(worklist) > 0 {
		pc := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		st := states[pc]
		next, fallsThrough, err := p.transfer(pc, p.Code[pc], st, localTypes)
		if err != nil {
			return err
		}
		succs := p.successors(pc, fallsThrough)
		for _, succ := range succs {
			if succ >= n {
				continue // falling off the end: runtime fault, not static
			}
			merged, changed, err := mergeState(states[succ], next, succ)
			if err != nil {
				return err
			}
			if changed {
				states[succ] = merged
				worklist = append(worklist, succ)
			}
		}
	}
	return nil
}

// successors lists where control may go after pc. A successor equal to
// len(code) models falling off the end of the program.
func (p *Program) successors(pc int, fallsThrough bool) []int {
	in := p.Code[pc]
	var out []int
	switch in.Op {
	case OpJmp:
		out = append(out, int(in.Arg))
	case OpJmpIf, OpJmpIfNot:
		out = append(out, int(in.Arg), pc+1)
	default:
		if fallsThrough {
			out = append(out, pc+1)
		}
	}
	return out
}

// mergeState merges incoming state into the existing state at pc.
// Stacks must be identical; assigned sets intersect.
func mergeState(existing, incoming *absState, pc int) (*absState, bool, error) {
	if existing == nil {
		cp := &absState{stack: append([]Type(nil), incoming.stack...),
			assigned: append([]bool(nil), incoming.assigned...)}
		return cp, true, nil
	}
	if !sameTypes(existing.stack, incoming.stack) {
		return nil, false, staticErr(pc,
			"inconsistent stack at control-flow merge: have %v, other path arrives with %v",
			existing.stack, incoming.stack)
	}
	changed := false
	for i := range existing.assigned {
		if existing.assigned[i] && !incoming.assigned[i] {
			existing.assigned[i] = false
			changed = true
		}
	}
	return existing, changed, nil
}

func sameTypes(a, b []Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// transfer computes the abstract state after executing the instruction at
// pc, or a static error if the instruction cannot type-check. The returned
// state never aliases the input. fallsThrough is false for halt.
func (p *Program) transfer(pc int, in Instr, st *absState, localTypes []Type) (next *absState, fallsThrough bool, err error) {
	stack := append([]Type(nil), st.stack...)
	assigned := append([]bool(nil), st.assigned...)

	pop := func() (Type, error) {
		if len(stack) == 0 {
			return 0, staticErr(pc, "%s requires a value but the stack is empty", in.Op)
		}
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return t, nil
	}
	push := func(t Type) error {
		if len(stack) >= maxAnalysisStack {
			return staticErr(pc, "stack may grow beyond %d entries (unbounded loop?)", maxAnalysisStack)
		}
		stack = append(stack, t)
		return nil
	}
	popInt := func() error {
		t, e := pop()
		if e != nil {
			return e
		}
		if t != TypeInt {
			return staticErr(pc, "%s expects int on the stack, got %s", in.Op, t)
		}
		return nil
	}

	switch in.Op {
	case OpPushInt:
		err = push(TypeInt)
	case OpPushBool:
		err = push(TypeBool)
	case OpDup:
		if len(stack) == 0 {
			err = staticErr(pc, "dup requires a value but the stack is empty")
		} else {
			err = push(stack[len(stack)-1])
		}
	case OpPop:
		_, err = pop()
	case OpSwap:
		if len(stack) < 2 {
			err = staticErr(pc, "swap requires two values, stack has %d", len(stack))
		} else {
			stack[len(stack)-1], stack[len(stack)-2] = stack[len(stack)-2], stack[len(stack)-1]
		}
	case OpLoad:
		idx := int(in.Arg)
		if !st.assigned[idx] {
			err = staticErr(pc, "local %q (slot %d) may be read before being assigned on all paths", p.Locals[idx].Name, idx)
		} else {
			err = push(localTypes[idx])
		}
	case OpStore:
		idx := int(in.Arg)
		var t Type
		t, err = pop()
		if err == nil && t != localTypes[idx] {
			err = staticErr(pc, "store of %s into local %q (slot %d) of type %s", t, p.Locals[idx].Name, idx, localTypes[idx])
		}
		if err == nil {
			assigned[idx] = true
		}
	case OpAdd, OpSub, OpMul, OpDiv:
		if err = popInt(); err == nil {
			err = popInt()
		}
		if err == nil {
			err = push(TypeInt)
		}
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		if err = popInt(); err == nil {
			err = popInt()
		}
		if err == nil {
			err = push(TypeBool)
		}
	case OpJmp:
		// no stack effect
	case OpJmpIf, OpJmpIfNot:
		var t Type
		t, err = pop()
		if err == nil && t != TypeBool {
			err = staticErr(pc, "%s condition must be bool, got %s", in.Op, t)
		}
	case OpHalt:
		if len(stack) != 1 {
			err = staticErr(pc, "halt requires exactly one result value on the stack, have %d", len(stack))
		}
		return &absState{stack: stack, assigned: assigned}, false, err
	}
	return &absState{stack: stack, assigned: assigned}, true, err
}
