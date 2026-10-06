package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/offline-spf-evaluator/internal/spf"
)

type query struct {
	IP     string `json:"ip"`
	Domain string `json:"domain"`
}

type request struct {
	Queries []query `json:"queries"`
}

func main() {
	snapshotPath := flag.String("snapshot", "", "path to DNS snapshot JSON")
	queriesPath := flag.String("queries", "", "path to queries JSON (default: stdin)")
	flag.Parse()
	if *snapshotPath == "" {
		fmt.Fprintln(os.Stderr, "usage: spf -snapshot snapshot.json [-queries queries.json]")
		os.Exit(2)
	}
	snap, err := spf.LoadSnapshot(*snapshotPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	var in []byte
	if *queriesPath != "" {
		in, err = os.ReadFile(*queriesPath)
	} else {
		in, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: read queries:", err)
		os.Exit(1)
	}
	var req request
	if err := json.Unmarshal(in, &req); err != nil {
		fmt.Fprintln(os.Stderr, "error: parse queries:", err)
		os.Exit(1)
	}
	ev := spf.NewEvaluator(snap)
	results := make([]spf.Result, 0, len(req.Queries))
	for _, q := range req.Queries {
		results = append(results, ev.Check(q.IP, q.Domain))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"results": results}); err != nil {
		fmt.Fprintln(os.Stderr, "error: encode:", err)
		os.Exit(1)
	}
}
