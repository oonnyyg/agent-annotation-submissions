package vm

import (
	"strings"
	"testing"
)

func pint(v int64) *int64 { return &v }
func pbool(v bool) *bool  { return &v }

func mustValidate(t *testing.T, p *Program) {
	t.Helper()
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func analyzeOK(t *testing.T, p *Program, assigned []bool) {
	t.Helper()
	mustValidate(t, p)
	if err := Analyze(p, assigned); err != nil {
		t.Fatalf("Analyze: unexpected error: %v", err)
	}
}

func analyzeErr(t *testing.T, p *Program, assigned []bool, want string) {
	t.Helper()
	mustValidate(t, p)
	err := Analyze(p, assigned)
	if err == nil {
		t.Fatalf("Analyze: expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Analyze: error %q does not contain %q", err.Error(), want)
	}
}

func TestStaticBranchJoinStackMismatch(t *testing.T) {
	// Then-branch leaves an extra int on the stack at the join.
	p := &Program{
		Code: []Instr{
			{Op: OpPushBool, Bool: pbool(true)}, // 0
			{Op: OpJmpIfNot, Target: 4},         // 1
			{Op: OpPushInt, Value: pint(1)},     // 2
			{Op: OpJmp, Target: 4},              // 3
			{Op: OpPushInt, Value: pint(2)},     // 4
			{Op: OpHalt},                        // 5
		},
	}
	analyzeErr(t, p, nil, "mismatched stack")
}

func TestStaticBranchJoinOK(t *testing.T) {
	p := &Program{
		Code: []Instr{
			{Op: OpPushBool, Bool: pbool(true)},
			{Op: OpJmpIfNot, Target: 4},
			{Op: OpPushInt, Value: pint(1)},
			{Op: OpJmp, Target: 5},
			{Op: OpPushInt, Value: pint(2)},
			{Op: OpHalt},
		},
	}
	analyzeOK(t, p, nil)
}

func TestStaticLoadBeforeAssignmentOnOnePath(t *testing.T) {
	// Local 0 is stored only on the true branch; the join must reject the load.
	p := &Program{
		Locals: []Type{TypeInt},
		Code: []Instr{
			{Op: OpPushBool, Bool: pbool(true)}, // 0
			{Op: OpJmpIfNot, Target: 4},         // 1
			{Op: OpPushInt, Value: pint(7)},     // 2
			{Op: OpStore, Var: 0},               // 3
			{Op: OpLoad, Var: 0},                // 4: join, local 0 not definitely assigned
			{Op: OpHalt},                        // 5
		},
	}
	analyzeErr(t, p, []bool{false}, "definite assignment")
}

func TestStaticLoopAssignment(t *testing.T) {
	p := &Program{
		Locals: []Type{TypeInt, TypeInt},
		Code: []Instr{
			{Op: OpLoad, Var: 0},            // 0
			{Op: OpPushInt, Value: pint(0)}, // 1
			{Op: OpGt},                      // 2
			{Op: OpJmpIfNot, Target: 11},    // 3
			{Op: OpPushInt, Value: pint(1)}, // 4
			{Op: OpStore, Var: 1},           // 5
			{Op: OpLoad, Var: 0},            // 6
			{Op: OpPushInt, Value: pint(1)}, // 7
			{Op: OpSub},                     // 8
			{Op: OpStore, Var: 0},           // 9
			{Op: OpJmp, Target: 0},          // 10
			{Op: OpLoad, Var: 1},            // 11: not definitely assigned
			{Op: OpHalt},                    // 12
		},
	}
	analyzeErr(t, p, []bool{true, false}, "definite assignment")

	// Same loop, but local 1 is assigned before the loop: accepted.
	pre := []Instr{
		{Op: OpPushInt, Value: pint(0)}, // 0
		{Op: OpStore, Var: 1},           // 1
	}
	code := append(pre, p.Code...)
	for i := range code[len(pre):] {
		in := &code[len(pre)+i]
		switch in.Op {
		case OpJmpIfNot:
			in.Target = 13
		case OpJmp:
			in.Target = 2
		}
	}
	ok := &Program{Locals: p.Locals, Code: code}
	analyzeOK(t, ok, []bool{true, false})
}

func TestStaticHaltStackCount(t *testing.T) {
	analyzeErr(t, &Program{Code: []Instr{{Op: OpHalt}}}, nil, "exactly 1 value")
	analyzeErr(t, &Program{Code: []Instr{
		{Op: OpPushInt, Value: pint(1)},
		{Op: OpPushInt, Value: pint(2)},
		{Op: OpHalt},
	}}, nil, "exactly 1 value")
}

func TestStaticTypeMismatch(t *testing.T) {
	// add on a bool operand
	analyzeErr(t, &Program{Code: []Instr{
		{Op: OpPushBool, Bool: pbool(true)},
		{Op: OpPushInt, Value: pint(1)},
		{Op: OpAdd},
		{Op: OpHalt},
	}}, nil, "want int")
	// store int into bool local
	analyzeErr(t, &Program{Locals: []Type{TypeBool}, Code: []Instr{
		{Op: OpPushInt, Value: pint(1)},
		{Op: OpStore, Var: 0},
		{Op: OpPushBool, Bool: pbool(true)},
		{Op: OpHalt},
	}}, []bool{false}, "want bool")
	// conditional jump on non-bool
	analyzeErr(t, &Program{Code: []Instr{
		{Op: OpPushInt, Value: pint(1)},
		{Op: OpJmpIf, Target: 2},
		{Op: OpHalt},
	}}, nil, "want bool")
}

func TestValidateStructuralErrors(t *testing.T) {
	bad := &Program{Code: []Instr{{Op: OpJmp, Target: 5}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("Validate: %v", err)
	}
	bad = &Program{Code: []Instr{{Op: OpLoad, Var: 0}, {Op: OpHalt}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("Validate: %v", err)
	}
	bad = &Program{Code: []Instr{{Op: OpPushInt}, {Op: OpHalt}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "value") {
		t.Fatalf("Validate: %v", err)
	}
}
