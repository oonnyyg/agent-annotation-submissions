package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	snapshotSeq := flag.Int("snapshot", -2, "print holdings after this event sequence number (-1 = initial state, -2 = final state)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: lotcost [-snapshot N] <events.jsonl>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *snapshotSeq, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(path string, snapshotSeq int, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	events, err := parseEvents(string(data))
	if err != nil {
		return err
	}
	if snapshotSeq == -2 {
		snapshotSeq = len(events) - 1
	}
	snap, results, err := SnapshotAt(events, snapshotSeq)
	if err != nil {
		return err
	}
	printResults(out, results)
	printSnapshot(out, snap)
	return nil
}

func printResults(out io.Writer, results []EventResult) {
	for _, r := range results {
		if r.Accepted {
			fmt.Fprintf(out, "event #%d %s: accepted\n", r.Seq, r.ID)
		} else {
			fmt.Fprintf(out, "event #%d %s: REJECTED: %s\n", r.Seq, r.ID, r.Reason)
		}
	}
}

func printSnapshot(out io.Writer, snap *Snapshot) {
	fmt.Fprintf(out, "holdings after event #%d:\n", snap.AfterSeq)
	if len(snap.Lots) == 0 {
		fmt.Fprintln(out, "  (no open lots)")
	}
	for _, lot := range snap.Lots {
		fmt.Fprintf(out, "  lot %s (buy #%d): qty=%s cost_cents=%s\n",
			lot.ID, lot.Seq, lot.Qty.RatString(), lot.Cost.RatString())
	}
	qty, cost := snap.Totals()
	fmt.Fprintf(out, "  total: qty=%s cost_cents=%s\n", qty.RatString(), cost.RatString())
	for _, sale := range snap.Sales {
		fmt.Fprintf(out, "sale %s (event #%d): qty=%s net_cents=%s\n",
			sale.EventID, sale.Seq, sale.Qty.RatString(), sale.NetCents.RatString())
		for _, src := range sale.Sources {
			fmt.Fprintf(out, "  from lot %s: qty=%s cost_cents=%s proceeds_cents=%s realized_cents=%s\n",
				src.LotID, src.Qty.RatString(), src.Cost.RatString(),
				src.Proceeds.RatString(), src.Realized.RatString())
		}
	}
}
