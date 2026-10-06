// Command scanlogic replays an offline sequential-control program against a
// recorded sequence of per-scan input snapshots. It never touches real
// devices or clocks; scan durations come from the record itself.
//
// Usage:
//
//	go run ./cmd/scanlogic -program program.json -records scans.jsonl
//
// Output is JSONL on stdout: one object per scan containing the fixed input
// snapshot, the scan duration, per-statement before/after changes, and the
// full resulting state.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/plc-scan-replay/internal/scanlogic"
)

func main() {
	programPath := flag.String("program", "", "path to program JSON file")
	recordsPath := flag.String("records", "", "path to per-scan input record JSONL file")
	flag.Parse()
	if *programPath == "" || *recordsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: scanlogic -program program.json -records scans.jsonl")
		os.Exit(2)
	}
	if err := run(*programPath, *recordsPath); err != nil {
		fmt.Fprintln(os.Stderr, "scanlogic:", err)
		os.Exit(1)
	}
}

func run(programPath, recordsPath string) error {
	pf, err := os.Open(programPath)
	if err != nil {
		return err
	}
	defer pf.Close()
	program, err := scanlogic.LoadProgram(pf)
	if err != nil {
		return fmt.Errorf("invalid program: %w", err)
	}

	rf, err := os.Open(recordsPath)
	if err != nil {
		return err
	}
	defer rf.Close()
	records, err := scanlogic.LoadRecords(rf)
	if err != nil {
		return err
	}

	results, err := program.Run(records)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	for _, res := range results {
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return nil
}
