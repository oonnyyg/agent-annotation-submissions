package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func main() {
	statePath := flag.String("state", "cash_state.json", "path to state file")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cashsim [--state file] <init|add|withdraw|confirm|cancel|status> ...")
		os.Exit(2)
	}
	s, err := LoadState(*statePath)
	if err != nil {
		fatal(err)
	}
	// mutated tracks whether state must be persisted after the command.
	mutated := false
	switch args[0] {
	case "init":
		if len(args) != 2 {
			fatal(errors.New("usage: init <denom:count,...>"))
		}
		counts, err := ParseCounts(args[1])
		if err != nil {
			fatal(err)
		}
		if err := s.SetInventory(counts); err != nil {
			fatal(err)
		}
		mutated = true
		fmt.Println("inventory initialized")
	case "add":
		if len(args) != 2 {
			fatal(errors.New("usage: add <denom:count,...>"))
		}
		counts, err := ParseCounts(args[1])
		if err != nil {
			fatal(err)
		}
		if err := s.AddCash(counts); err != nil {
			fatal(err)
		}
		mutated = true
		fmt.Println("cash added")
	case "withdraw":
		if len(args) != 3 {
			fatal(errors.New("usage: withdraw <request-id> <amount>"))
		}
		var amount int
		if _, err := fmt.Sscanf(args[2], "%d", &amount); err != nil {
			fatal(fmt.Errorf("bad amount: %w", err))
		}
		r, err := s.Withdraw(args[1], amount)
		if err != nil {
			fatal(err)
		}
		mutated = true
		printRequest(r)
	case "confirm":
		if len(args) != 2 {
			fatal(errors.New("usage: confirm <request-id>"))
		}
		r, err := s.Confirm(args[1])
		if err != nil {
			fatal(err)
		}
		mutated = true
		printRequest(r)
	case "cancel":
		if len(args) != 2 {
			fatal(errors.New("usage: cancel <request-id>"))
		}
		r, err := s.Cancel(args[1])
		if err != nil {
			fatal(err)
		}
		mutated = true
		printRequest(r)
	case "status":
		printStatus(s)
	default:
		fatal(fmt.Errorf("unknown command %q", args[0]))
	}
	if mutated {
		if err := SaveState(*statePath, s); err != nil {
			fatal(err)
		}
	}
}

func printRequest(r *Request) {
	fmt.Printf("request %s amount=%d status=%s plan=%s\n",
		r.ID, r.Amount, r.Status, formatPlan(r.Plan))
}

func formatPlan(p Plan) string {
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for _, d := range denomsDesc(p) {
		if !first {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%d:%d", d, p[d])
		first = false
	}
	sb.WriteString("}")
	return sb.String()
}

func printStatus(s *Store) {
	res := s.Reserved()
	avail := s.Available()
	fmt.Println("denom  physical  reserved  available")
	for _, d := range s.SortedDenoms() {
		fmt.Printf("%5d  %8d  %8d  %9d\n", d, s.Inventory[d], res[d], avail[d])
	}
	fmt.Println("requests:")
	for _, r := range s.SortedRequests() {
		fmt.Printf("  ")
		printRequest(r)
	}
}
