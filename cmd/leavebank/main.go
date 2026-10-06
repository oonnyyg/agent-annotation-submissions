// Command leavebank applies JSONL leave-ledger operations from stdin and
// prints one JSON result per line. State persists in a directory.
//
// Usage: go run ./cmd/leavebank -dir <state-dir>
//
// Operations (one JSON object per line):
//
//	{"op":"grant","batch":"B1","employee":"alice","start":"2026-01-01","end":"2026-04-01","minutes":4800,"date":"2026-01-01"}
//	{"op":"request","id":"R1","employee":"alice","date":"2026-02-10","minutes":240}
//	{"op":"cancel","id":"R1","date":"2026-03-01"}
//	{"op":"balance","employee":"alice","date":"2026-02-11"}
//	{"op":"usage","employee":"alice"}
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
	Op       string `json:"op"`
	Date     string `json:"date"`
	Batch    string `json:"batch"`
	ID       string `json:"id"`
	Employee string `json:"employee"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Minutes  int    `json:"minutes"`
}

func main() {
	dir := flag.String("dir", "leavebank-data", "state directory")
	flag.Parse()
	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "leavebank:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	st, err := ledger.Load(dir)
	if err != nil {
		return err
	}
	l := ledger.New(st)
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var o op
		if err := json.Unmarshal(raw, &o); err != nil {
			enc.Encode(map[string]string{"error": fmt.Sprintf("line %d: %v", line, err)})
			continue
		}
		enc.Encode(apply(l, o))
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return ledger.Save(dir, st)
}

func apply(l *ledger.Ledger, o op) any {
	switch o.Op {
	case "grant":
		err := l.Grant(ledger.Batch{
			ID: o.Batch, Employee: o.Employee,
			Start: o.Start, End: o.End, Minutes: o.Minutes,
		}, o.Date)
		if err != nil {
			return errResult(o, err)
		}
		return map[string]any{"op": o.Op, "batch": o.Batch, "status": "granted"}
	case "request":
		res, err := l.Apply(o.ID, o.Employee, o.Date, o.Minutes)
		if err != nil {
			return errResult(o, err)
		}
		return res
	case "cancel":
		res, err := l.Cancel(o.ID, o.Date)
		if err != nil {
			return errResult(o, err)
		}
		return res
	case "balance":
		res, err := l.Balance(o.Employee, o.Date)
		if err != nil {
			return errResult(o, err)
		}
		return res
	case "usage":
		return map[string]any{"employee": o.Employee, "usage": l.Usage(o.Employee)}
	default:
		return errResult(o, fmt.Errorf("unknown op %q", o.Op))
	}
}

func errResult(o op, err error) any {
	return map[string]any{"op": o.Op, "id": o.ID, "batch": o.Batch, "status": "error", "error": err.Error()}
}
