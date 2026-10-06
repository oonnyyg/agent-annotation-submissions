// Command spf evaluates SPF policies offline against a fixed DNS snapshot.
//
// Usage:
//
//	go run ./cmd/spf -snapshot snapshot.json -queries queries.json
//	go run ./cmd/spf -snapshot snapshot.json -ip 192.0.2.10 -domain example.com
//
// Output is a JSON array of evaluation results on stdout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/offline-spf-evaluator/spf"
)

type query struct {
	IP     string `json:"ip"`
	Domain string `json:"domain"`
}

func main() {
	snapPath := flag.String("snapshot", "", "path to DNS snapshot JSON (required)")
	queriesPath := flag.String("queries", "", "path to queries JSON array")
	ip := flag.String("ip", "", "single query sender IP")
	domain := flag.String("domain", "", "single query domain")
	flag.Parse()

	if *snapPath == "" {
		fmt.Fprintln(os.Stderr, "error: -snapshot is required")
		os.Exit(2)
	}
	data, err := os.ReadFile(*snapPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: reading snapshot:", err)
		os.Exit(2)
	}
	snap, err := spf.ParseSnapshot(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	var qs []query
	if *queriesPath != "" {
		raw, err := os.ReadFile(*queriesPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: reading queries:", err)
			os.Exit(2)
		}
		if err := json.Unmarshal(raw, &qs); err != nil {
			fmt.Fprintln(os.Stderr, "error: invalid queries JSON:", err)
			os.Exit(2)
		}
	}
	if *ip != "" || *domain != "" {
		qs = append(qs, query{IP: *ip, Domain: *domain})
	}
	if len(qs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no queries (use -queries or -ip/-domain)")
		os.Exit(2)
	}

	ev := spf.NewEvaluator(snap)
	out := make([]spf.Evaluation, 0, len(qs))
	for _, q := range qs {
		// Each query is independent: errors never propagate across the batch.
		out = append(out, ev.Check(q.IP, q.Domain))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "error: encoding output:", err)
		os.Exit(1)
	}
}
