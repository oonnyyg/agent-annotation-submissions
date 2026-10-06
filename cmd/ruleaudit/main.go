// Command ruleaudit statically analyzes ordered decision-table rules over
// finite enum / bounded integer fields. It reports uncovered regions,
// overlapping rules, and (under the first-match strategy) fully shadowed
// rules, each with a minimal reproducible witness input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/decision-table-coverage/internal/ruleaudit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ruleaudit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	specPath := fs.String("spec", "", "path to spec JSON (default: stdin)")
	strategy := fs.String("strategy", "unique", "resolution strategy: unique|first")
	maxCells := fs.Int("max-cells", 100000, "max equivalence cells to analyze (0 = unlimited)")
	inputJSON := fs.String("input", "", "optional sample input JSON object to evaluate")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var strat ruleaudit.Strategy
	switch *strategy {
	case string(ruleaudit.StrategyUnique):
		strat = ruleaudit.StrategyUnique
	case string(ruleaudit.StrategyFirst):
		strat = ruleaudit.StrategyFirst
	default:
		fmt.Fprintf(stderr, "error: unknown strategy %q\n", *strategy)
		return 2
	}

	var r io.Reader = stdin
	if *specPath != "" {
		f, err := os.Open(*specPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}
	spec, err := ruleaudit.LoadSpec(r)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	rep := ruleaudit.Analyze(spec, strat, *maxCells)
	printReport(stdout, spec, rep)

	exit := 0
	if !rep.Complete {
		exit = 3
	} else if len(rep.Uncovered) > 0 || len(rep.Overlaps) > 0 || len(rep.Shadowed) > 0 {
		exit = 1
	}

	if *inputJSON != "" {
		var input ruleaudit.Assignment
		if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil {
			fmt.Fprintf(stderr, "error: invalid -input JSON: %v\n", err)
			return 2
		}
		res, err := ruleaudit.Evaluate(spec, strat, input)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		printEval(stdout, spec, res)
	}
	return exit
}

func ruleName(spec *ruleaudit.Spec, i int) string {
	if spec.Rules[i].Name != "" {
		return fmt.Sprintf("#%d(%s)", i, spec.Rules[i].Name)
	}
	return fmt.Sprintf("#%d", i)
}

func ruleList(spec *ruleaudit.Spec, idxs []int) string {
	s := ""
	for i, ri := range idxs {
		if i > 0 {
			s += ", "
		}
		s += ruleName(spec, ri)
	}
	return s
}

func printReport(w io.Writer, spec *ruleaudit.Spec, rep *ruleaudit.Report) {
	fmt.Fprintf(w, "strategy: %s\n", rep.Strategy)
	fmt.Fprintf(w, "equivalence cells: %d (limit %d)\n", rep.TotalCells, rep.MaxCells)
	if !rep.Complete {
		fmt.Fprintf(w, "INCOMPLETE: cell count exceeds limit; no coverage/conflict conclusions drawn.\n")
		fmt.Fprintf(w, "hint: raise -max-cells or narrow field domains.\n")
		return
	}

	fmt.Fprintf(w, "uncovered regions: %d\n", len(rep.Uncovered))
	for _, r := range rep.Uncovered {
		fmt.Fprintf(w, "  gap: %s\n", joinComma(r.Segments))
		fmt.Fprintf(w, "    witness: %s\n", ruleaudit.FormatAssignment(spec, r.Witness))
	}

	fmt.Fprintf(w, "overlap regions: %d\n", len(rep.Overlaps))
	for _, r := range rep.Overlaps {
		fmt.Fprintf(w, "  overlap: %s\n", joinComma(r.Segments))
		fmt.Fprintf(w, "    rules: %s\n", ruleList(spec, r.Rules))
		fmt.Fprintf(w, "    witness: %s\n", ruleaudit.FormatAssignment(spec, r.Witness))
	}

	if rep.Strategy == ruleaudit.StrategyFirst {
		fmt.Fprintf(w, "shadowed rules: %d\n", len(rep.Shadowed))
		for _, s := range rep.Shadowed {
			fmt.Fprintf(w, "  shadowed: rule %s fully covered by earlier rules %s\n",
				ruleName(spec, s.Rule), ruleList(spec, s.CoveredBy))
		}
	}

	fmt.Fprintf(w, "effective rules: %d/%d\n", len(rep.Witnesses), len(spec.Rules))
	for _, rw := range rep.Witnesses {
		fmt.Fprintf(w, "  rule %s fires, witness: %s\n",
			ruleName(spec, rw.Rule), ruleaudit.FormatAssignment(spec, rw.Witness))
	}

	if len(rep.Uncovered) == 0 && len(rep.Overlaps) == 0 && len(rep.Shadowed) == 0 {
		fmt.Fprintln(w, "OK: full coverage, no overlaps, no shadowed rules.")
	}
}

func printEval(w io.Writer, spec *ruleaudit.Spec, res *ruleaudit.EvalResult) {
	fmt.Fprintf(w, "sample evaluation:\n")
	fmt.Fprintf(w, "  matched rules: %s\n", ruleList(spec, res.Matched))
	if res.OK {
		out := string(res.Output)
		if out == "" {
			out = "null"
		}
		fmt.Fprintf(w, "  outcome: rule %s output=%s\n", ruleName(spec, res.Rule), out)
	} else {
		fmt.Fprintf(w, "  outcome: none (%s)\n", res.Reason)
	}
}

func joinComma(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}
