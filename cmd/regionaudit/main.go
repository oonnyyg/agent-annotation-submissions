// Command regionaudit reads a region/point audit request from a JSON file
// (or standard input) and writes the audit report as JSON to standard
// output.
//
// Usage:
//
//	go run ./cmd/regionaudit [input.json]
//
// With no argument, input is read from stdin.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"example.com/polygon-region-audit/internal/audit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "regionaudit:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var data []byte
	var err error
	if len(args) > 1 {
		return fmt.Errorf("usage: regionaudit [input.json]")
	}
	if len(args) == 1 {
		data, err = os.ReadFile(args[0])
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return err
	}
	var in audit.Input
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("invalid input JSON: %w", err)
	}
	out := audit.Run(in)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
