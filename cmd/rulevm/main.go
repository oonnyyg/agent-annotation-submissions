// Command rulevm runs statically validated stack-machine rule programs.
// It never shells out or evaluates host code: programs are pure bytecode.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/typed-stack-rule-vm/vm"
)

const usage = `rulevm - execute a typed stack rule bytecode program

Usage:
  rulevm -program program.json [-input input.json] [flags]

Input JSON (optional):
  {"stack": [{"type":"int","value":5}, {"type":"bool","value":true}]}

Flags:`

func main() {
	fs := flag.NewFlagSet("rulevm", flag.ContinueOnError)
	programPath := fs.String("program", "", "path to the program JSON file (required)")
	inputPath := fs.String("input", "", "path to the input JSON file (default: empty stack)")
	maxSteps := fs.Int("max-steps", 100000, "instruction budget; reaching it stops as 'limited'")
	trace := fs.Bool("trace", false, "record per-instruction pc and stack")
	traceLimit := fs.Int("trace-limit", 0, "max retained trace entries (0 = unlimited); truncation never changes the result")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}
	if *programPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	progData, err := os.ReadFile(*programPath)
	if err != nil {
		fatal(1, "reading program: %v", err)
	}
	prog, err := vm.ParseProgram(progData)
	if err != nil {
		fatal(2, "%v", err)
	}

	var input struct {
		Stack []vm.Value `json:"stack"`
	}
	if *inputPath != "" {
		data, err := os.ReadFile(*inputPath)
		if err != nil {
			fatal(1, "reading input: %v", err)
		}
		if err := json.Unmarshal(data, &input); err != nil {
			fatal(1, "invalid input JSON: %v", err)
		}
	}

	if err := prog.CheckInput(input.Stack); err != nil {
		fatal(1, "input does not match program params: %v", err)
	}

	res := vm.Run(prog, input.Stack, vm.Options{
		MaxSteps:   *maxSteps,
		Trace:      *trace,
		TraceLimit: *traceLimit,
	})
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))

	switch res.Outcome {
	case vm.OutcomeOK:
		os.Exit(0)
	case vm.OutcomeLimited:
		os.Exit(4)
	default:
		os.Exit(3)
	}
}

func fatal(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "rulevm: "+format+"\n", args...)
	os.Exit(code)
}
