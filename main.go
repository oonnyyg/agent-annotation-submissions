package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	var data []byte
	var err error
	if len(args) > 2 {
		return fmt.Errorf("usage: elevator-replay [input.json] (reads stdin when omitted)")
	}
	if len(args) == 2 {
		data, err = os.ReadFile(args[1])
	} else {
		data, err = io.ReadAll(stdin)
	}
	if err != nil {
		return err
	}
	var in Input
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := validate(&in); err != nil {
		return fmt.Errorf("input rejected: %w", err)
	}

	sim := NewSim(&in)
	sim.Run()

	fmt.Fprintln(stdout, "== event log ==")
	for _, line := range sim.logs {
		fmt.Fprintln(stdout, line)
	}

	fmt.Fprintln(stdout, "== passenger trips ==")
	totalWait, maxWait := 0, 0
	for _, p := range sim.all {
		if p.Board < 0 || p.Alight < 0 {
			return fmt.Errorf("passenger %q never completed a trip", p.ID)
		}
		wait := p.Board - p.Arrive
		totalWait += wait
		if wait > maxWait {
			maxWait = wait
		}
		fmt.Fprintf(stdout, "id=%s from=%d to=%d arrive=%d board=%d alight=%d wait=%d ride=%d\n",
			p.ID, p.From, p.To, p.Arrive, p.Board, p.Alight, wait, p.Alight-p.Board)
	}

	fmt.Fprintln(stdout, "== summary ==")
	n := len(sim.all)
	fmt.Fprintf(stdout, "passengers=%d\n", n)
	if n > 0 {
		fmt.Fprintf(stdout, "avgWait=%.2f maxWait=%d\n", float64(totalWait)/float64(n), maxWait)
	}
	fmt.Fprintf(stdout, "finishTime=%d maxLoad=%d capacity=%d\n", sim.time, sim.maxLoad, sim.cfg.Capacity)
	if sim.maxLoad > sim.cfg.Capacity {
		return fmt.Errorf("capacity exceeded: load %d > %d", sim.maxLoad, sim.cfg.Capacity)
	}
	return nil
}
