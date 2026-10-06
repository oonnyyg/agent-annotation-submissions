package main

import (
	"flag"
	"fmt"
	"os"
)

// Command-line entry point. Reads JSONL events from a file (or stdin) and
// writes one JSON result per event to stdout. Replaying the same input
// produces identical output.
func main() {
	input := flag.String("input", "", "JSONL input file (default: stdin)")
	flag.Parse()

	in := os.Stdin
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open input: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}
	if err := Replay(in, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}
}
