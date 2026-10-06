// Command scanlogic replays an offline scan-logic program against a
// recorded per-scan input log and emits per-scan state as JSONL plus a
// human-readable change summary. It never connects to real devices and
// never reads a real clock.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/plc-scan-replay/internal/scanlogic"
)

func main() {
	programPath := flag.String("program", "", "path to program JSON file (required)")
	inputsPath := flag.String("inputs", "", "path to per-scan input records JSON file (required)")
	summaryPath := flag.String("summary", "", "optional path for the change summary (default: stderr)")
	flag.Parse()
	if *programPath == "" || *inputsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: scanlogic -program program.json -inputs scans.json [-summary summary.txt]")
		os.Exit(2)
	}
	if err := run(*programPath, *inputsPath, *summaryPath); err != nil {
		fmt.Fprintln(os.Stderr, "scanlogic:", err)
		os.Exit(1)
	}
}

func run(programPath, inputsPath, summaryPath string) error {
	pf, err := os.Open(programPath)
	if err != nil {
		return err
	}
	defer pf.Close()
	prog, err := scanlogic.LoadProgram(pf)
	if err != nil {
		return fmt.Errorf("invalid program: %w", err)
	}

	inf, err := os.Open(inputsPath)
	if err != nil {
		return err
	}
	defer inf.Close()
	log, err := scanlogic.LoadInputLog(inf, prog)
	if err != nil {
		return fmt.Errorf("invalid input log: %w", err)
	}

	summary := os.Stderr
	if summaryPath != "" {
		f, err := os.Create(summaryPath)
		if err != nil {
			return err
		}
		defer f.Close()
		summary = f
	}

	eng := scanlogic.NewEngine(prog)
	enc := json.NewEncoder(os.Stdout)
	for i, scan := range log.Scans {
		res := eng.RunScan(i, scan)
		if err := enc.Encode(res); err != nil {
			return err
		}
		writeSummary(summary, res)
	}
	return nil
}

func writeSummary(w *os.File, res scanlogic.ScanResult) {
	fmt.Fprintf(w, "scan %d (%d ms):", res.Scan, res.DurationMs)
	changed := false
	for _, d := range res.Diffs {
		if d.Changed() {
			changed = true
			b, _ := json.Marshal(d.Before)
			a, _ := json.Marshal(d.After)
			fmt.Fprintf(w, "\n  stmt %d %s: %s -> %s", d.Index, d.Target, b, a)
		}
	}
	if !changed {
		fmt.Fprint(w, " no state changes")
	}
	fmt.Fprintln(w)
}
