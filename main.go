package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadEvents reads a JSONL file into events, preserving order.
func LoadEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(text), &ev); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}

// Snapshot replays events[0..through] on a fresh ledger, skipping
// rejected events, and returns the state. It never touches other ledgers.
func Snapshot(events []Event, through int) (*Ledger, error) {
	if through < -1 || through >= len(events) {
		return nil, fmt.Errorf("snapshot index %d out of range [0,%d]", through, len(events)-1)
	}
	l := NewLedger()
	for i := 0; i <= through; i++ {
		l.Apply(events[i]) // rejected events keep their seq but change nothing
	}
	return l, nil
}

func printLedger(l *Ledger) {
	fmt.Println("open lots:")
	open := 0
	for _, lot := range l.lots {
		if lot.Closed {
			continue
		}
		open++
		fmt.Printf("  lot=%s seq=%d qty=%s cost_cents=%s\n",
			lot.ID, lot.Seq, ratString(lot.Open), ratString(lot.Cost))
	}
	if open == 0 {
		fmt.Println("  (none)")
	}
	fmt.Println("sales:")
	if len(l.sales) == 0 {
		fmt.Println("  (none)")
	}
	for _, s := range l.sales {
		fmt.Printf("  sale=%s seq=%d qty=%s proceeds_cents=%s realized_cents=%s\n",
			s.EventID, s.Seq, ratString(s.Qty), ratString(s.Proceeds), ratString(s.Realized))
		for _, src := range s.Sources {
			fmt.Printf("    from lot=%s qty=%s cost_cents=%s proceeds_cents=%s realized_cents=%s\n",
				src.LotID, ratString(src.Qty), ratString(src.Cost),
				ratString(src.Proceeds), ratString(src.Realized))
		}
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  lotcost replay <events.jsonl>")
	fmt.Fprintln(os.Stderr, "  lotcost snapshot <events.jsonl> <event-index>")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	cmd, path := os.Args[1], os.Args[2]
	events, err := LoadEvents(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	switch cmd {
	case "replay":
		l := NewLedger()
		for i, ev := range events {
			if err := l.Apply(ev); err != nil {
				fmt.Printf("event %d id=%s REJECTED: %v\n", i, ev.ID, err)
			} else {
				fmt.Printf("event %d id=%s ok\n", i, ev.ID)
			}
		}
		printLedger(l)
	case "snapshot":
		if len(os.Args) < 4 {
			usage()
		}
		idx, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad index:", err)
			os.Exit(2)
		}
		l, err := Snapshot(events, idx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "snapshot:", err)
			os.Exit(1)
		}
		printLedger(l)
	default:
		usage()
	}
}
