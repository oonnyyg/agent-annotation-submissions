package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/blocking-workshop-replay/internal/workshop"
)

func main() {
	input := flag.String("input", "", "path to the workshop JSON file (required)")
	csvOut := flag.String("csv", "", "path for the operation-times CSV (default: stdout)")
	sumOut := flag.String("summary", "", "path for the status summary JSON (default: stdout)")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "usage: workshop -input FILE [-csv FILE] [-summary FILE]")
		os.Exit(2)
	}

	f, err := os.Open(*input)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	in, err := workshop.LoadInput(f)
	if err != nil {
		fatal(err)
	}

	res := workshop.Simulate(in)

	if err := writeTo(*csvOut, "operations.csv", func(w *os.File) error {
		return workshop.WriteCSV(w, res)
	}); err != nil {
		fatal(err)
	}
	if err := writeTo(*sumOut, "summary.json", func(w *os.File) error {
		return workshop.WriteSummary(w, res)
	}); err != nil {
		fatal(err)
	}

	if res.Stalled {
		fmt.Fprintf(os.Stderr, "stalled at t=%d: %d job(s) unfinished\n", res.StallTime, countStalled(res))
		os.Exit(1)
	}
}

func writeTo(path, defName string, fn func(*os.File) error) error {
	if path == "" {
		return fn(os.Stdout)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

func countStalled(res *workshop.Result) int {
	n := 0
	for _, j := range res.Jobs {
		if j.Status != "completed" {
			n++
		}
	}
	return n
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(2)
}
