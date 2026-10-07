package vm

import (
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, p *Program, err error) *Program {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected static rejection: %v", err)
	}
	return p
}

func prog(t *testing.T, params []string, locals []Local, code ...Instr) *Program {
	t.Helper()
	p := &Program{Params: params, Locals: locals, Code: code}
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected static rejection: %v", err)
	}
	return p
}

func expectStatic(t *testing.T, p *Program, substr string) {
	t.Helper()
	err := p.Validate()
	if err == nil {
		t.Fatalf("expected static rejection containing %q, got nil", substr)
	}
	se, ok := err.(*StaticError)
	if !ok {
		t.Fatalf("expected *StaticError, got %T: %v", err, err)
	}
	if !strings.Contains(se.Msg, substr) {
		t.Fatalf("static error %q does not contain %q", se.Msg, substr)
	}
}

func TestBranchMergeStackMismatchRejected(t *testing.T) {
	// Then-branch pushes an extra int; else-branch does not.
	p := &Program{Code: []Instr{
		{Op: OpPushBool, Arg: 1},
		{Op: OpJmpIfNot, Arg: 4},
		{Op: OpPushInt, Arg: 1},
		{Op: OpPushInt, Arg: 2}, // extra value only on this path
		{Op: OpHalt},
	}}
	expectStatic(t, p, "inconsistent stack")
}

func TestBranchMergeTypeMismatchRejected(t *testing.T) {
	p := &Program{Code: []Instr{
		{Op: OpPushBool, Arg: 1},
		{Op: OpJmpIfNot, Arg: 4},
		{Op: OpPushInt, Arg: 1},
		{Op: OpJmp, Arg: 5},
		{Op: OpPushBool, Arg: 0},
		{Op: OpHalt},
	}}
	expectStatic(t, p, "inconsistent stack")
}

func TestBranchMergeConsistentAccepted(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushBool, Arg: 1},
		Instr{Op: OpJmpIfNot, Arg: 4},
		Instr{Op: OpPushInt, Arg: 10},
		Instr{Op: OpJmp, Arg: 5},
		Instr{Op: OpPushInt, Arg: 20},
		Instr{Op: OpHalt},
	)
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != 10 {
		t.Fatalf("got %+v", res)
	}
}

func TestReadBeforeAssignRejected(t *testing.T) {
	p := &Program{
		Locals: []Local{{Name: "x", Type: "int"}},
		Code:   []Instr{{Op: OpLoad, Arg: 0}, {Op: OpHalt}},
	}
	expectStatic(t, p, "before being assigned")
}

func TestAssignOnOneBranchOnlyRejected(t *testing.T) {
	p := &Program{
		Locals: []Local{{Name: "x", Type: "int"}},
		Code: []Instr{
			{Op: OpPushBool, Arg: 1},
			{Op: OpJmpIfNot, Arg: 5},
			{Op: OpPushInt, Arg: 1},
			{Op: OpStore, Arg: 0},
			{Op: OpJmp, Arg: 5},
			{Op: OpLoad, Arg: 0}, // x assigned on only one incoming path
			{Op: OpHalt},
		},
	}
	expectStatic(t, p, "before being assigned")
}

func TestLoopAssignmentAnalysis(t *testing.T) {
	// Sum 1..n with a loop; variables assigned before the loop are
	// readable inside and after it.
	p := prog(t,
		[]string{"int"},
		[]Local{{Name: "n", Type: "int"}, {Name: "acc", Type: "int"}, {Name: "i", Type: "int"}},
		Instr{Op: OpStore, Arg: 0},
		Instr{Op: OpPushInt, Arg: 0},
		Instr{Op: OpStore, Arg: 1},
		Instr{Op: OpPushInt, Arg: 1},
		Instr{Op: OpStore, Arg: 2},
		Instr{Op: OpLoad, Arg: 2},
		Instr{Op: OpLoad, Arg: 0},
		Instr{Op: OpLe},
		Instr{Op: OpJmpIfNot, Arg: 18},
		Instr{Op: OpLoad, Arg: 1},
		Instr{Op: OpLoad, Arg: 2},
		Instr{Op: OpAdd},
		Instr{Op: OpStore, Arg: 1},
		Instr{Op: OpLoad, Arg: 2},
		Instr{Op: OpPushInt, Arg: 1},
		Instr{Op: OpAdd},
		Instr{Op: OpStore, Arg: 2},
		Instr{Op: OpJmp, Arg: 5},
		Instr{Op: OpLoad, Arg: 1},
		Instr{Op: OpHalt},
	)
	res := Run(p, []Value{Int(100)}, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != 5050 {
		t.Fatalf("got %+v", res)
	}
}

func TestLoopOnlyAssignmentRejected(t *testing.T) {
	// x is assigned inside the loop body; the path that skips the loop
	// never assigns it, so reading it after the loop must be rejected.
	p := &Program{
		Params: []string{"bool"},
		Locals: []Local{{Name: "x", Type: "int"}},
		Code: []Instr{
			{Op: OpJmpIfNot, Arg: 5},
			{Op: OpPushInt, Arg: 1},
			{Op: OpStore, Arg: 0},
			{Op: OpPushBool, Arg: 0},
			{Op: OpJmpIf, Arg: 1},
			{Op: OpLoad, Arg: 0},
			{Op: OpHalt},
		},
	}
	expectStatic(t, p, "before being assigned")
}

func TestStoreTypeMismatchRejected(t *testing.T) {
	p := &Program{
		Locals: []Local{{Name: "x", Type: "int"}},
		Code: []Instr{
			{Op: OpPushBool, Arg: 1},
			{Op: OpStore, Arg: 0},
			{Op: OpPushInt, Arg: 0},
			{Op: OpHalt},
		},
	}
	expectStatic(t, p, "store of bool")
}

func TestJumpIfRequiresBool(t *testing.T) {
	p := &Program{Code: []Instr{
		{Op: OpPushInt, Arg: 1},
		{Op: OpJmpIf, Arg: 2},
		{Op: OpHalt},
	}}
	expectStatic(t, p, "condition must be bool")
}

func TestBadJumpTargetRejected(t *testing.T) {
	p := &Program{Code: []Instr{
		{Op: OpJmp, Arg: 99},
		{Op: OpHalt},
	}}
	expectStatic(t, p, "out of range")
}

func TestHaltRequiresExactlyOneValue(t *testing.T) {
	p := &Program{Code: []Instr{
		{Op: OpPushInt, Arg: 1},
		{Op: OpPushInt, Arg: 2},
		{Op: OpHalt},
	}}
	expectStatic(t, p, "exactly one result")
}

func TestStepLimitMarksLimited(t *testing.T) {
	// Infinite loop: jmp to itself.
	p := prog(t, nil, nil, Instr{Op: OpJmp, Arg: 0})
	res := Run(p, nil, Options{MaxSteps: 50})
	if res.Outcome != OutcomeLimited {
		t.Fatalf("expected limited, got %+v", res)
	}
	if res.Steps != 50 || res.StopPC != 0 {
		t.Fatalf("steps=%d stop_pc=%d", res.Steps, res.StopPC)
	}
	if res.Value != nil {
		t.Fatalf("limited run must not produce a value: %+v", res.Value)
	}
}

func TestFallOffEndFaults(t *testing.T) {
	p := prog(t, nil, nil, Instr{Op: OpPushInt, Arg: 1})
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeFault || res.StopPC != 1 {
		t.Fatalf("got %+v", res)
	}
	if !strings.Contains(res.Fault.Msg, "off the end") {
		t.Fatalf("fault message: %v", res.Fault.Msg)
	}
}

func TestTraceAndTruncation(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushInt, Arg: 2},
		Instr{Op: OpPushInt, Arg: 3},
		Instr{Op: OpAdd},
		Instr{Op: OpHalt},
	)
	full := Run(p, nil, Options{Trace: true})
	if len(full.Trace) != 4 || full.TraceDropped != 0 {
		t.Fatalf("full trace: %+v", full)
	}
	if got := full.Trace[2].Stack[0].I; got != 5 {
		t.Fatalf("trace stack after add: %v", got)
	}
	trunc := Run(p, nil, Options{Trace: true, TraceLimit: 2})
	if len(trunc.Trace) != 2 || trunc.TraceDropped != 2 {
		t.Fatalf("truncated trace: %+v", trunc)
	}
	// Truncation must not change the outcome.
	if trunc.Outcome != full.Outcome || *trunc.Value != *full.Value {
		t.Fatalf("truncation changed result: %+v vs %+v", trunc, full)
	}
}

func TestRunsDoNotShareState(t *testing.T) {
	p := prog(t,
		[]string{"int"},
		[]Local{{Name: "x", Type: "int"}},
		Instr{Op: OpStore, Arg: 0},
		Instr{Op: OpLoad, Arg: 0},
		Instr{Op: OpLoad, Arg: 0},
		Instr{Op: OpAdd},
		Instr{Op: OpHalt},
	)
	input := []Value{Int(21)}
	r1 := Run(p, input, Options{})
	r2 := Run(p, input, Options{})
	if r1.Value.I != 42 || r2.Value.I != 42 {
		t.Fatalf("r1=%+v r2=%+v", r1, r2)
	}
	// The caller's input slice must be untouched.
	if input[0] != Int(21) {
		t.Fatalf("input mutated: %+v", input)
	}
}

func TestStackOps(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushInt, Arg: 1},
		Instr{Op: OpPushInt, Arg: 2},
		Instr{Op: OpSwap},
		Instr{Op: OpSub}, // 2 - 1
		Instr{Op: OpDup},
		Instr{Op: OpAdd}, // 1 + 1
		Instr{Op: OpPushInt, Arg: 9},
		Instr{Op: OpPop},
		Instr{Op: OpHalt},
	)
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != 2 {
		t.Fatalf("got %+v", res)
	}
}

func TestCheckInput(t *testing.T) {
	p := prog(t, []string{"int"}, nil, Instr{Op: OpHalt})
	if err := p.CheckInput([]Value{Int(1)}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := p.CheckInput(nil); err == nil {
		t.Fatal("expected arity error")
	}
	if err := p.CheckInput([]Value{Bool(true)}); err == nil {
		t.Fatal("expected type error")
	}
}

func TestParseProgramJSON(t *testing.T) {
	data := []byte(`{
	  "params": ["int"],
	  "locals": [{"name":"x","type":"int"}],
	  "code": [
	    {"op":"store","arg":0},
	    {"op":"load","arg":0},
	    {"op":"halt"}
	  ]
	}`)
	p, err := ParseProgram(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := Run(p, []Value{Int(7)}, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != 7 {
		t.Fatalf("got %+v", res)
	}

	if _, err := ParseProgram([]byte(`{"code":[{"op":"nope"}]}`)); err == nil {
		t.Fatal("expected rejection of unknown opcode")
	}
	var se *StaticError
	if _, err := ParseProgram([]byte(`{"code":[{"op":"pop"}]}`)); !errors.As(err, &se) {
		t.Fatalf("expected StaticError, got %v", err)
	}
}

func TestArithmeticAndComparisons(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushInt, Arg: 7},
		Instr{Op: OpPushInt, Arg: 2},
		Instr{Op: OpDiv},
		Instr{Op: OpHalt},
	)
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != 3 {
		t.Fatalf("got %+v", res)
	}
}

func TestDivisionTruncatesTowardZero(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushInt, Arg: -7},
		Instr{Op: OpPushInt, Arg: 2},
		Instr{Op: OpDiv},
		Instr{Op: OpHalt},
	)
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeOK || res.Value.I != -3 {
		t.Fatalf("got %+v", res)
	}
}

func TestDivisionByZeroFaults(t *testing.T) {
	p := prog(t, nil, nil,
		Instr{Op: OpPushInt, Arg: 1},
		Instr{Op: OpPushInt, Arg: 0},
		Instr{Op: OpDiv},
		Instr{Op: OpHalt},
	)
	res := Run(p, nil, Options{})
	if res.Outcome != OutcomeFault || res.Fault.PC != 2 {
		t.Fatalf("got %+v", res)
	}
	if !strings.Contains(res.Fault.Msg, "division by zero") {
		t.Fatalf("fault message: %v", res.Fault.Msg)
	}
}

func TestOverflowFaults(t *testing.T) {
	const max = int64(9223372036854775807)
	cases := []struct {
		name string
		code []Instr
	}{
		{"add", []Instr{{Op: OpPushInt, Arg: max}, {Op: OpPushInt, Arg: 1}, {Op: OpAdd}, {Op: OpHalt}}},
		{"sub", []Instr{{Op: OpPushInt, Arg: -max - 1}, {Op: OpPushInt, Arg: 1}, {Op: OpSub}, {Op: OpHalt}}},
		{"mul", []Instr{{Op: OpPushInt, Arg: max}, {Op: OpPushInt, Arg: 2}, {Op: OpMul}, {Op: OpHalt}}},
		{"mul-neg", []Instr{{Op: OpPushInt, Arg: -1}, {Op: OpPushInt, Arg: -1}, {Op: OpMul}, {Op: OpHalt}}},
		{"div-min-neg1", []Instr{{Op: OpPushInt, Arg: -max - 1}, {Op: OpPushInt, Arg: -1}, {Op: OpDiv}, {Op: OpHalt}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := prog(t, nil, nil, tc.code...)
			res := Run(p, nil, Options{})
			if tc.name == "mul-neg" {
				if res.Outcome != OutcomeOK || res.Value.I != 1 {
					t.Fatalf("got %+v", res)
				}
				return
			}
			if res.Outcome != OutcomeFault {
				t.Fatalf("expected fault, got %+v", res)
			}
			if !strings.Contains(res.Fault.Msg, "overflow") && !strings.Contains(res.Fault.Msg, "zero") {
				t.Fatalf("fault message: %v", res.Fault.Msg)
			}
		})
	}
}
