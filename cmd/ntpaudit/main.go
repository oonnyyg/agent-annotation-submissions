// Command ntpaudit audits recorded NTP exchanges from a JSONL file. It is
// strictly offline: it never sends packets and never touches the system
// clock.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	ntpaudit "example.com/ntp-exchange-auditor"
)

func main() {
	window := flag.Int("window", 8, "number of most recent valid samples kept per peer")
	flag.Parse()

	var in io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "ntpaudit:", err)
			os.Exit(2)
		}
		defer f.Close()
		in = f
	}

	aud := ntpaudit.NewAuditor(*window)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var rec ntpaudit.Record
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			fmt.Printf("line %d: invalid JSON: %v\n", line, err)
			continue
		}
		printAnalysis(aud.Process(line, rec))
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "ntpaudit:", err)
		os.Exit(2)
	}
	printSummaries(aud.Summaries())
}

func printAnalysis(an ntpaudit.Analysis) {
	switch an.Status {
	case ntpaudit.StatusAccepted:
		s := an.Sample
		fmt.Printf("line %d peer %q: accepted offset=%s delay=%s (t2=%s t3=%s)\n",
			an.Line, an.Peer, s.Offset, s.Delay,
			s.T2.Format(time.RFC3339Nano), s.T3.Format(time.RFC3339Nano))
	default:
		fmt.Printf("line %d peer %q: %s: %s\n", an.Line, an.Peer, an.Status, an.Reason)
	}
}

func printSummaries(sums []ntpaudit.PeerSummary) {
	fmt.Println("\n== peer summaries ==")
	for _, sum := range sums {
		if sum.Selected == nil {
			fmt.Printf("peer %q: no valid samples\n", sum.Peer)
			continue
		}
		fmt.Printf("peer %q: offset=%s jitter=%s window=%d\n",
			sum.Peer, sum.Selected.Offset, sum.Jitter, len(sum.Window))
		for i, s := range sum.Window {
			mark := " "
			if s == *sum.Selected {
				mark = "*"
			}
			fmt.Printf("  %s[%d] offset=%s delay=%s t4=%s\n",
				mark, i, s.Offset, s.Delay, s.T4.Format(time.RFC3339Nano))
		}
	}
}
