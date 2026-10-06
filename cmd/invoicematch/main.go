// Command invoicematch processes JSONL procurement operations from stdin
// and writes one JSON response per line to stdout. State persists in the
// directory given by -dir.
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
	dir := flag.String("dir", "invoicematch-data", "state directory")
	flag.Parse()

	svc, err := match.OpenService(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open state:", err)
		os.Exit(1)
	}
	defer svc.Close()

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := svc.Handle(json.RawMessage(line))
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "write response:", err)
			os.Exit(1)
		}
		out.Flush()
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
}
