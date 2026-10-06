package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/piecework-payroll/internal/payroll"
)

func main() {
	dir := flag.String("state-dir", os.Getenv("PIECEPAY_STATE_DIR"), "state directory")
	flag.Parse()
	if *dir == "" {
		*dir = "state"
	}
	svc, err := payroll.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open state:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req payroll.Request
		var resp payroll.Response
		if err := json.Unmarshal(line, &req); err != nil {
			resp = payroll.Response{OK: false, Error: "invalid JSON: " + err.Error()}
		} else {
			resp = svc.Handle(req)
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "write response:", err)
			os.Exit(1)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}
}
