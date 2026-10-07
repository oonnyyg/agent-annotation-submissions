package vm

import (
	"fmt"
	"math"
	"math/bits"
)

// Outcome classifies how a run ended.
type Outcome string

const (
	OutcomeOK      Outcome = "ok"      // halted with exactly one result
	OutcomeFault   Outcome = "fault"   // runtime error (div by zero, overflow, ...)
	OutcomeLimited Outcome = "limited" // step budget exhausted
)

// Fault is a runtime stop with a locatable position.
type Fault struct {
	PC  int
	Msg string
}

func (f *Fault) Error() string { return fmt.Sprintf("runtime fault at pc=%d: %s", f.PC, f.Msg) }

// TraceEntry records one executed instruction and the stack after it.
type TraceEntry struct {
	PC    int     `json:"pc"`
	Op    Op      `json:"op"`
	Stack []Value `json:"stack_after"`
}

// Result summarizes one execution.
type Result struct {
	Outcome      Outcome      `json:"outcome"`
	Value        *Value       `json:"value,omitempty"` // final result, ok only
	Fault        *Fault       `json:"fault,omitempty"` // fault only
	StopPC       int          `json:"stop_pc"`         // pc where execution stopped
	Steps        int          `json:"steps"`           // instructions executed
	Trace        []TraceEntry `json:"trace,omitempty"`
	TraceDropped int          `json:"trace_dropped,omitempty"` // entries omitted due to limit
}

// Options configures a run.
type Options struct {
	MaxSteps   int  // 0 means defaultMaxSteps
	Trace      bool // record a trace
	TraceLimit int  // max retained trace entries; 0 means unlimited
}

const defaultMaxSteps = 100000

// Run executes a validated program against an initial stack.
// Each call gets fresh locals and a fresh stack copy: runs never
// pollute each other.
func Run(p *Program, input []Value, opts Options) *Result {
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}
	stack := append([]Value(nil), input...)
	locals := make([]Value, len(p.Locals))
	assigned := make([]bool, len(p.Locals))
	res := &Result{Outcome: OutcomeOK}
	var trace []TraceEntry
	dropped := 0

	record := func(pc int) {
		if !opts.Trace {
			return
		}
		entry := TraceEntry{PC: pc, Op: p.Code[pc].Op, Stack: append([]Value{}, stack...)}
		if opts.TraceLimit > 0 && len(trace) >= opts.TraceLimit {
			dropped++
			return
		}
		trace = append(trace, entry)
	}

	pc := 0
	for {
		if pc < 0 || pc >= len(p.Code) {
			res.Outcome = OutcomeFault
			res.Fault = &Fault{PC: pc, Msg: "execution ran off the end of the program"}
			res.StopPC = pc
			break
		}
		if res.Steps >= maxSteps {
			res.Outcome = OutcomeLimited
			res.StopPC = pc
			break
		}
		in := p.Code[pc]
		next, fault := execInstr(in, pc, &stack, locals, assigned)
		if fault != nil {
			res.Outcome = OutcomeFault
			res.Fault = fault
			res.StopPC = pc
			break
		}
		res.Steps++
		record(pc)
		if in.Op == OpHalt {
			v := stack[0]
			res.Value = &v
			res.StopPC = pc
			break
		}
		pc = next
	}
	res.Trace = trace
	res.TraceDropped = dropped
	return res
}

func faultf(pc int, format string, args ...any) *Fault {
	return &Fault{PC: pc, Msg: fmt.Sprintf(format, args...)}
}

// execInstr executes one instruction, returning the next pc. Static
// validation guarantees stack shapes and types, so only genuinely
// runtime conditions (overflow, division by zero) produce faults here.
func execInstr(in Instr, pc int, stack *[]Value, locals []Value, assigned []bool) (int, *Fault) {
	st := *stack
	pop := func() Value {
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	push := func(v Value) { st = append(st, v) }

	next := pc + 1
	switch in.Op {
	case OpPushInt:
		push(Int(in.Arg))
	case OpPushBool:
		push(Bool(in.Arg != 0))
	case OpDup:
		push(st[len(st)-1])
	case OpPop:
		pop()
	case OpSwap:
		st[len(st)-1], st[len(st)-2] = st[len(st)-2], st[len(st)-1]
	case OpLoad:
		push(locals[in.Arg])
	case OpStore:
		locals[in.Arg] = pop()
		assigned[in.Arg] = true
	case OpAdd, OpSub, OpMul:
		b, a := pop().I, pop().I
		r, ok := arithChecked(in.Op, a, b)
		if !ok {
			return 0, faultf(pc, "%s overflows int64 (%d, %d)", in.Op, a, b)
		}
		push(Int(r))
	case OpDiv:
		b, a := pop().I, pop().I
		if b == 0 {
			return 0, faultf(pc, "division by zero (%d / 0)", a)
		}
		if a == -9223372036854775808 && b == -1 {
			return 0, faultf(pc, "division overflows int64 (%d / %d)", a, b)
		}
		push(Int(a / b)) // Go integer division truncates toward zero
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		b, a := pop().I, pop().I
		push(Bool(compare(in.Op, a, b)))
	case OpJmp:
		next = int(in.Arg)
	case OpJmpIf:
		if pop().B {
			next = int(in.Arg)
		}
	case OpJmpIfNot:
		if !pop().B {
			next = int(in.Arg)
		}
	case OpHalt:
		// handled by Run
	}
	*stack = st
	return next, nil
}

// arithChecked computes add/sub/mul with overflow detection.
func arithChecked(op Op, a, b int64) (int64, bool) {
	switch op {
	case OpAdd:
		sum := a + b
		// overflow only when operands share a sign and the sum differs
		if (a >= 0) == (b >= 0) && (sum >= 0) != (a >= 0) {
			return 0, false
		}
		return sum, true
	case OpSub:
		diff := a - b
		if (a >= 0) != (b >= 0) && (diff >= 0) != (a >= 0) {
			return 0, false
		}
		return diff, true
	default: // OpMul
		neg := (a < 0) != (b < 0)
		ua, ub := uint64(a), uint64(b)
		if a < 0 {
			ua = -ua // two's-complement negation yields the magnitude
		}
		if b < 0 {
			ub = -ub
		}
		hi, lo := bits.Mul64(ua, ub)
		if hi != 0 {
			return 0, false
		}
		if neg {
			if lo > 1<<63 { // -2^63 is the most negative representable value
				return 0, false
			}
			return -int64(lo), true
		}
		if lo > math.MaxInt64 {
			return 0, false
		}
		return int64(lo), true
	}
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
	default:
		return a >= b
	}
}
