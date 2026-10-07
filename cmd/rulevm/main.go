// Command rulevm runs typed stack-machine rule programs from JSON.
// Programs are fully validated and statically analyzed before execution;
// no host scripting or dynamic evaluation is involved.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/typed-stack-rule-vm/vm"
)

type output struct {
	Status      string          `json:"status"` // ok | static_error | runtime_error | limited
	Value       *vm.Value       `json:"value,omitempty"`
	Error       string          `json:"error,omitempty"`
	ErrorPC     *int            `json:"error_pc,omitempty"`
	Steps       int             `json:"steps"`
	Trace       []vm.TraceEntry `json:"trace,omitempty"`
	TraceCutoff bool            `json:"trace_truncated,omitempty"`
}

func main() {
	programPath := flag.String("program", "", "path to program JSON file (required)")
	inputPath := flag.String("input", "", "path to input JSON file (optional; supplies initial local values)")
	maxSteps := flag.Int("max-steps", 100000, "maximum executed instructions before limited termination (0 = unlimited)")
	maxTrace := flag.Int("trace", 0, "record at most N trace entries of pc/op/stack (0 = off, -1 = unlimited)")
	flag.Parse()

	if *programPath == "" {
		fmt.Fprintln(os.Stderr, "error: -program is required")
		flag.Usage()
		os.Exit(2)
	}

	progData, err := os.ReadFile(*programPath)
	if err != nil {
		exitErr("read program: %v", err)
	}
	var prog vm.Program
	if err := json.Unmarshal(progData, &prog); err != nil {
		exitErr("parse program: %v", err)
	}

	initial := make([]vm.Value, len(prog.Locals))
	assigned := make([]bool, len(prog.Locals))
	if *inputPath != "" {
		inData, err := os.ReadFile(*inputPath)
		if err != nil {
			exitErr("read input: %v", err)
		}
		var in vm.InputFile
		if err := json.Unmarshal(inData, &in); err != nil {
			exitErr("parse input: %v", err)
		}
		if len(in.Locals) != len(prog.Locals) {
			exitErr("input provides %d locals, program declares %d", len(in.Locals), len(prog.Locals))
		}
		for i, v := range in.Locals {
			if v == nil {
				continue
			}
			if v.T != prog.Locals[i] {
				exitErr("input local %d: type %s does not match declared %s", i, v.T, prog.Locals[i])
			}
			initial[i] = *v
			assigned[i] = true
		}
	}

	if err := prog.Validate(); err != nil {
		writeOutput(output{Status: "static_error", Error: err.Error()})
		os.Exit(1)
	}
	if err := vm.Analyze(&prog, assigned); err != nil {
		writeOutput(output{Status: "static_error", Error: err.Error()})
		os.Exit(1)
	}

	res := vm.Run(&prog, initial, *maxSteps, *maxTrace)
	out := output{
		Status:      string(res.Status),
		Value:       res.Value,
		Error:       res.Error,
		ErrorPC:     res.ErrorPC,
		Steps:       res.Steps,
		Trace:       res.Trace,
		TraceCutoff: res.TraceCutoff,
	}
	writeOutput(out)
	if res.Status != vm.StatusOK {
		os.Exit(1)
	}
}

func writeOutput(o output) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(o)
}

func exitErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(2)
}
