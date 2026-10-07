package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"example.com/cash-cassette-dispense/dispense"
)

const usage = `cash-atm - local cash cassette dispense simulator

Usage: cash-atm [-state FILE] <command> [args]

Commands:
  load <denom:count,denom:count,...>   Import cassette inventory (rejects duplicates/negatives)
  replenish <denom> <count>            Add notes to an existing denomination
  withdraw <request-id> <amount>       Create a withdrawal request and reserve notes
  confirm <request-id>                 Confirm dispense (deducts physical stock)
  cancel <request-id>                  Cancel a non-confirmed request (releases reservation)
  status                               Show per-denomination physical/reserved/available
  requests                             List all requests with status and plan

All amounts are integer minor currency units. State persists across runs.
`

func main() {
	statePath := "atm-state.json"
	args := os.Args[1:]
	for len(args) >= 2 && args[0] == "-state" {
		statePath = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	atm, err := dispense.LoadATM(statePath)
	if err != nil {
		fatal(err)
	}
	cmd, rest := args[0], args[1:]
	dirty := true
	switch cmd {
	case "load":
		must(len(rest) == 1, "load requires <denom:count,...>")
		ds, cs := parsePairs(rest[0])
		if err := atm.LoadCassette(ds, cs); err != nil {
			fatal(err)
		}
	case "replenish":
		must(len(rest) == 2, "replenish requires <denom> <count>")
		if err := atm.Replenish(atoi(rest[0]), atoi(rest[1])); err != nil {
			fatal(err)
		}
	case "withdraw":
		must(len(rest) == 2, "withdraw requires <request-id> <amount>")
		r, err := atm.CreateRequest(rest[0], atoi(rest[1]))
		if err != nil {
			if errors.Is(err, dispense.ErrInsufficientBalance) {
				fmt.Println("REJECTED: total balance insufficient")
			} else if errors.Is(err, dispense.ErrNoCombination) {
				fmt.Println("REJECTED: amount cannot be composed with available denominations")
			} else {
				fatal(err)
			}
			os.Exit(1)
		}
		printRequest(r)
	case "confirm":
		must(len(rest) == 1, "confirm requires <request-id>")
		r, err := atm.Confirm(rest[0])
		if err != nil {
			fatal(err)
		}
		printRequest(r)
	case "cancel":
		must(len(rest) == 1, "cancel requires <request-id>")
		r, err := atm.Cancel(rest[0])
		if err != nil {
			fatal(err)
		}
		printRequest(r)
	case "status":
		dirty = false
		fmt.Printf("%-12s %10s %10s %10s\n", "DENOM", "PHYSICAL", "RESERVED", "AVAILABLE")
		for _, row := range atm.Report() {
			fmt.Printf("%-12d %10d %10d %10d\n", row.Denom, row.Physical, row.Reserved, row.Available)
		}
	case "requests":
		dirty = false
		for _, r := range atm.Requests {
			printRequest(r)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if dirty {
		if err := atm.Save(statePath); err != nil {
			fatal(err)
		}
	}
}

func printRequest(r *dispense.Request) {
	fmt.Printf("%s amount=%d status=%s plan=%s\n", r.ID, r.Amount, r.Status, formatPlan(r.Plan))
}

func formatPlan(plan map[string]int64) string {
	if len(plan) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(plan))
	for d, n := range plan {
		parts = append(parts, fmt.Sprintf("%sx%d", d, n))
	}
	return strings.Join(parts, ",")
}

func parsePairs(s string) ([]int64, []int64) {
	var ds, cs []int64
	for _, p := range strings.Split(s, ",") {
		kv := strings.SplitN(p, ":", 2)
		must(len(kv) == 2, "invalid pair: "+p)
		ds = append(ds, atoi(kv[0]))
		cs = append(cs, atoi(kv[1]))
	}
	return ds, cs
}

func atoi(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		fatal(fmt.Errorf("invalid number %q", s))
	}
	return n
}

func must(ok bool, msg string) {
	if !ok {
		if msg == "" {
			msg = "invalid arguments"
		}
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
