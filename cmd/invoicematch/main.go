// Command invoicematch is a local three-way match tool. It reads business
// operations as JSONL from stdin, persists them in a data directory, and
// writes one JSON result line per operation to stdout.
//
// Usage: go run ./cmd/invoicematch -dir <data-dir>
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/procurement-three-way-match/internal/match"
)

func main() {
	dir := flag.String("dir", "invoicematch-data", "data directory for the persistent op log")
	flag.Parse()

	eng, err := match.OpenEngine(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open engine:", err)
		os.Exit(1)
	}
	defer eng.Close()

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var op match.Op
		if err := json.Unmarshal(line, &op); err != nil {
			enc.Encode(match.Result{OK: false, Error: "invalid JSON: " + err.Error()})
			continue
		}
		enc.Encode(eng.Execute(&op))
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
}
