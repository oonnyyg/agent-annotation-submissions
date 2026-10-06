// Command leavebank applies JSONL operations from stdin to a persistent
// leave-balance ledger stored in a directory, printing one JSON result
// per line.
//
// Usage: go run ./cmd/leavebank -dir <state-dir>
//
// Operations (one JSON object per line, all minutes are integers):
//
//	{"op":"grant","date":"2026-01-01","batch":"B1","employee":"alice","start":"2026-01-01","end":"2026-04-01","minutes":4800}
//	{"op":"request","date":"2026-01-10","id":"R1","employee":"alice","minutes":240}
//	{"op":"cancel","date":"2026-01-20","id":"R1"}
//	{"op":"report","date":"2026-01-15","employee":"alice"}
//	{"op":"usage","id":"R1"}
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/expiring-leave-bank/internal/ledger"
)

type op struct {
	Op       string      `json:"op"`
	Date     ledger.Date `json:"date"`
	Batch    string      `json:"batch"`
	ID       string      `json:"id"`
	Employee string      `json:"employee"`
	Start    ledger.Date `json:"start"`
	End      ledger.Date `json:"end"`
	Minutes  int         `json:"minutes"`
}

func main() {
	dir := flag.String("dir", "leavebank-data", "state directory")
	flag.Parse()

	l, err := ledger.Load(*dir)
	if err != nil {
		fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Bytes()
		if len(text) == 0 {
			continue
		}
		var o op
		if err := json.Unmarshal(text, &o); err != nil {
			fatal(fmt.Errorf("line %d: %w", line, err))
		}
		res, err := apply(l, o)
		if err != nil {
			enc.Encode(map[string]string{"status": "error", "error": err.Error()})
			continue
		}
		if err := l.Save(*dir); err != nil {
			fatal(err)
		}
		enc.Encode(res)
	}
	if err := sc.Err(); err != nil {
		fatal(err)
	}
}

func apply(l *ledger.Ledger, o op) (any, error) {
	switch o.Op {
	case "grant":
		return l.Grant(o.Date, o.Batch, o.Employee, o.Start, o.End, o.Minutes)
	case "request":
		return l.Request(o.Date, o.ID, o.Employee, o.Minutes)
	case "cancel":
		return l.Cancel(o.Date, o.ID)
	case "report":
		return l.Report(o.Employee, o.Date), nil
	case "usage":
		return l.Usage(o.ID)
	default:
		return nil, fmt.Errorf("unknown op %q", o.Op)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "leavebank:", err)
	os.Exit(1)
}
