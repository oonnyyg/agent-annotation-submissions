package vm

import (
	"math"
	"strings"
	"testing"
)

func runOK(t *testing.T, p *Program, initial []Value) Value {
	t.Helper()
	mustValidate(t, p)
	assigned := make([]bool, len(p.Locals))
	for i := range initial {
		assigned[i] = true
	}
	if err := Analyze(p, assigned); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	res := Run(p, initial, 0, 0)
	if res.Status != StatusOK {
		t.Fatalf("Run: status %s, error %s", res.Status, res.Error)
	}
	return *res.Value
}

func arithProg(op Op, a, b int64) *Program {
	return &Program{Code: []Instr{
		{Op: OpPushInt, Value: pint(a)},
		{Op: OpPushInt, Value: pint(b)},
		{Op: op},
		{Op: OpHalt},
	}}
}

func TestDivTruncatesTowardZero(t *testing.T) {
	if v := runOK(t, arithProg(OpDiv, -7, 2), nil); v.I != -3 {
		t.Fatalf("-7/2 = %d, want -3", v.I)
	}
	if v := runOK(t, arithProg(OpDiv, 7, -2), nil); v.I != -3 {
		t.Fatalf("7/-2 = %d, want -3", v.I)
	}
}

func TestDivByZeroRuntimeError(t *testing.T) {
	p := arithProg(OpDiv, 1, 0)
	mustValidate(t, p)
	if err := Analyze(p, nil); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	res := Run(p, nil, 0, 0)
	if res.Status != StatusRuntime {
		t.Fatalf("status %s, want runtime_error", res.Status)
	}
	if res.ErrorPC == nil || *res.ErrorPC != 2 {
		t.Fatalf("error pc %v, want 2", res.ErrorPC)
	}
	if !strings.Contains(res.Error, "division by zero") {
		t.Fatalf("error %q", res.Error)
	}
}

func TestOverflowRuntimeErrors(t *testing.T) {
	cases := []struct {
		op   Op
		a, b int64
	}{
		{OpAdd, math.MaxInt64, 1},
		{OpAdd, math.MinInt64, -1},
		{OpSub, math.MinInt64, 1},
		{OpMul, math.MaxInt64, 2},
		{OpDiv, math.MinInt64, -1},
	}
	for _, c := range cases {
		p := arithProg(c.op, c.a, c.b)
		mustValidate(t, p)
		if err := Analyze(p, nil); err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		res := Run(p, nil, 0, 0)
		if res.Status != StatusRuntime || !strings.Contains(res.Error, "overflow") {
			t.Fatalf("%s(%d,%d): status %s error %q", c.op, c.a, c.b, res.Status, res.Error)
		}
	}
}

func TestFallOffEndIsRuntimeError(t *testing.T) {
	p := &Program{Code: []Instr{{Op: OpPushInt, Value: pint(1)}}}
	mustValidate(t, p)
	if err := Analyze(p, nil); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	res := Run(p, nil, 0, 0)
	if res.Status != StatusRuntime || !strings.Contains(res.Error, "past end") {
		t.Fatalf("status %s error %q", res.Status, res.Error)
	}
}

func TestStepLimit(t *testing.T) {
	// Infinite loop: 0: jmp 0
	p := &Program{Code: []Instr{{Op: OpJmp, Target: 0}}}
	mustValidate(t, p)
	if err := Analyze(p, nil); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	res := Run(p, nil, 100, 0)
	if res.Status != StatusLimited {
		t.Fatalf("status %s, want limited", res.Status)
	}
	if res.Steps != 100 {
		t.Fatalf("steps %d, want 100", res.Steps)
	}
	if res.ErrorPC == nil || *res.ErrorPC != 0 {
		t.Fatalf("stop pc %v, want 0", res.ErrorPC)
	}
}

func TestTraceTruncationDoesNotChangeResult(t *testing.T) {
	p := &Program{Code: []Instr{
		{Op: OpPushInt, Value: pint(3)},
		{Op: OpPushInt, Value: pint(4)},
		{Op: OpAdd},
		{Op: OpHalt},
	}}
	mustValidate(t, p)
	if err := Analyze(p, nil); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	full := Run(p, nil, 0, -1)
	cut := Run(p, nil, 0, 2)
	if full.Status != StatusOK || cut.Status != StatusOK {
		t.Fatalf("statuses %s %s", full.Status, cut.Status)
	}
	if full.Value.I != 7 || cut.Value.I != 7 {
		t.Fatalf("values %d %d", full.Value.I, cut.Value.I)
	}
	if len(cut.Trace) != 2 || !cut.TraceCutoff {
		t.Fatalf("trace len %d cutoff %v", len(cut.Trace), cut.TraceCutoff)
	}
	if len(full.Trace) != 4 || full.TraceCutoff {
		t.Fatalf("full trace len %d cutoff %v", len(full.Trace), full.TraceCutoff)
	}
	if full.Trace[0].PC != 0 || full.Trace[0].Stack[0].I != 3 {
		t.Fatalf("trace[0] %+v", full.Trace[0])
	}
}

func TestRunsDoNotShareState(t *testing.T) {
	// local 0 starts as 5; program stores 99 and returns it.
	p := &Program{
		Locals: []Type{TypeInt},
		Code: []Instr{
			{Op: OpPushInt, Value: pint(99)},
			{Op: OpStore, Var: 0},
			{Op: OpLoad, Var: 0},
			{Op: OpHalt},
		},
	}
	initial := []Value{Int(5)}
	for i := 0; i < 3; i++ {
		if v := runOK(t, p, initial); v.I != 99 {
			t.Fatalf("run %d: %d", i, v.I)
		}
		if initial[0].I != 5 {
			t.Fatalf("run %d mutated caller's initial locals", i)
		}
	}
}

func TestBranchMergeRuntime(t *testing.T) {
	// if input0 > 10 then 1 else 2
	p := &Program{
		Locals: []Type{TypeInt},
		Code: []Instr{
			{Op: OpLoad, Var: 0},             // 0
			{Op: OpPushInt, Value: pint(10)}, // 1
			{Op: OpGt},                       // 2
			{Op: OpJmpIfNot, Target: 6},      // 3
			{Op: OpPushInt, Value: pint(1)},  // 4
			{Op: OpJmp, Target: 7},           // 5
			{Op: OpPushInt, Value: pint(2)},  // 6
			{Op: OpHalt},                     // 7
		},
	}
	if v := runOK(t, p, []Value{Int(20)}); v.I != 1 {
		t.Fatalf("got %d, want 1", v.I)
	}
	if v := runOK(t, p, []Value{Int(5)}); v.I != 2 {
		t.Fatalf("got %d, want 2", v.I)
	}
}
