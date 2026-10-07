package vm

import (
	"fmt"
	"math/bits"
)

// Status is the outcome classification of a run.
type Status string

const (
	StatusOK      Status = "ok"            // halted normally with one result
	StatusRuntime Status = "runtime_error" // abnormal stop (overflow, div0, ...)
	StatusLimited Status = "limited"       // step limit reached, not a normal completion
)

// TraceEntry records one executed instruction.
type TraceEntry struct {
	PC    int     `json:"pc"`
	Op    Op      `json:"op"`
	Stack []Value `json:"stack_after"`
}

// Result summarizes one execution.
type Result struct {
	Status      Status       `json:"status"`
	Value       *Value       `json:"value,omitempty"`
	Error       string       `json:"error,omitempty"`
	ErrorPC     *int         `json:"error_pc,omitempty"`
	Steps       int          `json:"steps"`
	Trace       []TraceEntry `json:"trace,omitempty"`
	TraceCutoff bool         `json:"trace_truncated,omitempty"`
}

// RuntimeError is an abnormal stop with a locatable position.
type RuntimeError struct {
	PC  int
	Msg string
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("runtime error at pc %d: %s", e.PC, e.Msg)
}

// Run executes p with the given initial local values (nil = unassigned,
// but Analyze must already have passed). maxSteps > 0 caps executed
// instructions; maxTrace >= 0 bounds recorded trace entries without
// affecting the result.
func Run(p *Program, initial []Value, maxSteps int, maxTrace int) Result {
	locals := make([]Value, len(p.Locals))
	assigned := make([]bool, len(p.Locals))
	for i, v := range initial {
		if i < len(locals) {
			locals[i] = v
			assigned[i] = true
		}
	}
	var stack []Value
	var trace []TraceEntry
	cutoff := false
	pc := 0
	steps := 0

	fail := func(pc int, format string, args ...any) Result {
		msg := fmt.Sprintf(format, args...)
		return Result{Status: StatusRuntime, Error: (&RuntimeError{PC: pc, Msg: msg}).Error(),
			ErrorPC: &pc, Steps: steps, Trace: trace, TraceCutoff: cutoff}
	}
	record := func(pc int, op Op) {
		if maxTrace == 0 {
			return
		}
		if maxTrace < 0 || len(trace) < maxTrace {
			trace = append(trace, TraceEntry{PC: pc, Op: op, Stack: append([]Value(nil), stack...)})
		} else {
			cutoff = true
		}
	}

	for {
		if maxSteps > 0 && steps >= maxSteps {
			return Result{Status: StatusLimited,
				Error:   fmt.Sprintf("step limit %d reached; stopped before pc %d", maxSteps, pc),
				ErrorPC: &pc, Steps: steps, Trace: trace, TraceCutoff: cutoff}
		}
		if pc < 0 || pc >= len(p.Code) {
			return fail(pc, "program counter moved past end of program")
		}
		in := p.Code[pc]
		steps++
		next := pc + 1

		pop := func() Value { v := stack[len(stack)-1]; stack = stack[:len(stack)-1]; return v }

		switch in.Op {
		case OpPushInt:
			stack = append(stack, Int(*in.Value))
		case OpPushBool:
			stack = append(stack, Bool(*in.Bool))
		case OpPop:
			pop()
		case OpDup:
			stack = append(stack, stack[len(stack)-1])
		case OpSwap:
			top := len(stack) - 1
			stack[top], stack[top-1] = stack[top-1], stack[top]
		case OpLoad:
			stack = append(stack, locals[in.Var])
		case OpStore:
			locals[in.Var] = pop()
			assigned[in.Var] = true
		case OpAdd, OpSub, OpMul, OpDiv, OpMod:
			b, a := pop().I, pop().I
			v, err := arith(in.Op, a, b)
			if err != nil {
				return fail(pc, "%s", err)
			}
			stack = append(stack, Int(v))
		case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
			b, a := pop().I, pop().I
			stack = append(stack, Bool(compare(in.Op, a, b)))
		case OpNot:
			stack = append(stack, Bool(!pop().B))
		case OpJmp:
			next = in.Target
		case OpJmpIf:
			if pop().B {
				next = in.Target
			}
		case OpJmpIfNot:
			if !pop().B {
				next = in.Target
			}
		case OpHalt:
			v := stack[0]
			record(pc, in.Op)
			return Result{Status: StatusOK, Value: &v, Steps: steps, Trace: trace, TraceCutoff: cutoff}
		}
		record(pc, in.Op)
		pc = next
	}
}

func arith(op Op, a, b int64) (int64, error) {
	switch op {
	case OpAdd:
		sum, _ := bits.Add64(uint64(a), uint64(b), 0)
		if (a > 0 && b > 0 && int64(sum) < 0) || (a < 0 && b < 0 && int64(sum) >= 0) {
			return 0, fmt.Errorf("integer overflow: %d + %d", a, b)
		}
		return int64(sum), nil
	case OpSub:
		diff := a - b
		if (a >= 0 && b < 0 && diff < 0) || (a < 0 && b > 0 && diff >= 0) {
			return 0, fmt.Errorf("integer overflow: %d - %d", a, b)
		}
		return diff, nil
	case OpMul:
		hi, lo := bits.Mul64(uint64(a), uint64(b))
		if int64(hi) != int64(lo)>>63 {
			return 0, fmt.Errorf("integer overflow: %d * %d", a, b)
		}
		return int64(lo), nil
	case OpDiv:
		if b == 0 {
			return 0, fmt.Errorf("division by zero: %d / 0", a)
		}
		if a == -1<<63 && b == -1 {
			return 0, fmt.Errorf("integer overflow: %d / %d", a, b)
		}
		return a / b, nil // Go integer division truncates toward zero
	case OpMod:
		if b == 0 {
			return 0, fmt.Errorf("division by zero: %d %% 0", a)
		}
		if a == -1<<63 && b == -1 {
			return 0, fmt.Errorf("integer overflow: %d %% %d", a, b)
		}
		return a % b, nil
	}
	return 0, fmt.Errorf("unknown arithmetic op %q", op)
}

func compare(op Op, a, b int64) bool {
	switch op {
	case OpEq:
		return a == b
	case OpNe:
		return a != b
	case OpLt:
		return a < b
	case OpLe:
		return a <= b
	case OpGt:
		return a > b
	case OpGe:
		return a >= b
	}
	return false
}
