package vm

import "fmt"

// StaticError describes a problem found before execution.
type StaticError struct {
	PC  int
	Msg string
}

func (e *StaticError) Error() string {
	if e.PC >= 0 {
		return fmt.Sprintf("static error at pc %d: %s", e.PC, e.Msg)
	}
	return "static error: " + e.Msg
}

// state is the abstract interpreter state at one instruction.
type state struct {
	stack    []Type // abstract operand stack
	assigned []bool // definite assignment per local
}

func (s state) clone() state {
	st := state{assigned: append([]bool(nil), s.assigned...)}
	st.stack = append([]Type(nil), s.stack...)
	return st
}

func sameStack(a, b []Type) bool {
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

// Analyze verifies the reachable control flow of p. initialAssigned marks
// which locals already hold a value (from the input file). It checks stack
// typing, stack/assignment consistency at control-flow joins, definite
// assignment before loads, and that every halt leaves exactly one value.
func Analyze(p *Program, initialAssigned []bool) error {
	if len(initialAssigned) != len(p.Locals) {
		return &StaticError{PC: -1, Msg: "initial assignment mask does not match locals"}
	}
	n := len(p.Code)
	states := make([]state, n)
	seen := make([]bool, n)
	work := []int{0}
	states[0] = state{assigned: append([]bool(nil), initialAssigned...)}
	seen[0] = true

	// merge records incoming state at pc; returns true if pc must be reprocessed.
	merge := func(pc int, in state) (bool, error) {
		if !seen[pc] {
			states[pc] = in.clone()
			seen[pc] = true
			return true, nil
		}
		cur := states[pc]
		if !sameStack(cur.stack, in.stack) {
			return false, &StaticError{PC: pc, Msg: fmt.Sprintf(
				"control-flow join with mismatched stack: have %v, other path has %v",
				formatStack(cur.stack), formatStack(in.stack))}
		}
		changed := false
		for i := range cur.assigned {
			if cur.assigned[i] && !in.assigned[i] {
				cur.assigned[i] = false
				changed = true
			}
		}
		states[pc] = cur
		return changed, nil
	}

	for len(work) > 0 {
		pc := work[len(work)-1]
		work = work[:len(work)-1]
		s := states[pc].clone()
		in := p.Code[pc]

		pop := func(what string, want ...Type) error {
			if len(s.stack) < len(want) {
				return &StaticError{PC: pc, Msg: fmt.Sprintf("%s needs %d operand(s), stack has %d", what, len(want), len(s.stack))}
			}
			base := len(s.stack) - len(want)
			for i, w := range want {
				if s.stack[base+i] != w {
					return &StaticError{PC: pc, Msg: fmt.Sprintf("%s operand %d is %s, want %s", what, i+1, s.stack[base+i], w)}
				}
			}
			s.stack = s.stack[:base]
			return nil
		}
		push := func(t Type) { s.stack = append(s.stack, t) }

		next := -1 // pc+1 successor, -1 = none
		var jump *state

		switch in.Op {
		case OpPushInt:
			push(TypeInt)
			next = pc + 1
		case OpPushBool:
			push(TypeBool)
			next = pc + 1
		case OpPop:
			if len(s.stack) < 1 {
				return &StaticError{PC: pc, Msg: "pop on empty stack"}
			}
			s.stack = s.stack[:len(s.stack)-1]
			next = pc + 1
		case OpDup:
			if len(s.stack) < 1 {
				return &StaticError{PC: pc, Msg: "dup on empty stack"}
			}
			push(s.stack[len(s.stack)-1])
			next = pc + 1
		case OpSwap:
			if len(s.stack) < 2 {
				return &StaticError{PC: pc, Msg: "swap needs 2 values"}
			}
			top := len(s.stack) - 1
			s.stack[top], s.stack[top-1] = s.stack[top-1], s.stack[top]
			next = pc + 1
		case OpLoad:
			if !s.assigned[in.Var] {
				return &StaticError{PC: pc, Msg: fmt.Sprintf("load of local %d before definite assignment on all paths", in.Var)}
			}
			push(p.Locals[in.Var])
			next = pc + 1
		case OpStore:
			if err := pop("store", p.Locals[in.Var]); err != nil {
				return err
			}
			s.assigned[in.Var] = true
			next = pc + 1
		case OpAdd, OpSub, OpMul, OpDiv, OpMod:
			if err := pop(string(in.Op), TypeInt, TypeInt); err != nil {
				return err
			}
			push(TypeInt)
			next = pc + 1
		case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
			if err := pop(string(in.Op), TypeInt, TypeInt); err != nil {
				return err
			}
			push(TypeBool)
			next = pc + 1
		case OpNot:
			if err := pop("not", TypeBool); err != nil {
				return err
			}
			push(TypeBool)
			next = pc + 1
		case OpJmp:
			j := s.clone()
			jump = &j
		case OpJmpIf, OpJmpIfNot:
			if err := pop(string(in.Op), TypeBool); err != nil {
				return err
			}
			j := s.clone()
			jump = &j
			next = pc + 1
		case OpHalt:
			if len(s.stack) != 1 {
				return &StaticError{PC: pc, Msg: fmt.Sprintf("halt requires exactly 1 value on stack, have %d", len(s.stack))}
			}
		}

		if next >= 0 && next < n {
			ok, err := merge(next, s)
			if err != nil {
				return err
			}
			if ok {
				work = append(work, next)
			}
		}
		if jump != nil {
			ok, err := merge(in.Target, *jump)
			if err != nil {
				return err
			}
			if ok {
				work = append(work, in.Target)
			}
		}
	}
	return nil
}

func formatStack(s []Type) string {
	if len(s) == 0 {
		return "empty"
	}
	out := "["
	for i, t := range s {
		if i > 0 {
			out += " "
		}
		out += string(t)
	}
	return out + "]"
}
