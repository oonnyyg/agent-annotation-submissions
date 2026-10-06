// Command tokens is an offline JSONL driver for the refresh-token family
// ledger. It reads one operation per line from stdin and writes one JSON
// result per line to stdout. State is persisted to a local directory.
//
// This is a local simulation with caller-supplied mock identifiers; it is
// not a complete OAuth service.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/refresh-family-ledger/ledger"
)

type request struct {
	Op        string   `json:"op"`
	SessionID string   `json:"session_id,omitempty"`
	ClientID  string   `json:"client_id,omitempty"`
	TokenID   string   `json:"token_id,omitempty"`
	NewToken  string   `json:"new_token_id,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
	Seconds   int64    `json:"seconds,omitempty"`
}

type response struct {
	OK     bool        `json:"ok"`
	Error  string      `json:"error,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

func main() {
	dir := flag.String("dir", "token-ledger-data", "state directory")
	flag.Parse()

	led, err := ledger.Load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(response{OK: false, Error: "bad json: " + err.Error()})
			continue
		}
		enc.Encode(apply(led, *dir, req))
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
}

// apply executes one op. Mutating ops are persisted before a success
// response is returned; if the save fails the in-memory change is rolled
// back so no half-completed rotation survives.
func apply(led *ledger.Ledger, dir string, req request) response {
	mutate := func(fn func() error) response {
		backup := led.Clone()
		if err := fn(); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := ledger.Save(dir, led); err != nil {
			led.Restore(backup)
			return response{OK: false, Error: "persist: " + err.Error()}
		}
		return response{OK: true}
	}

	switch req.Op {
	case "create":
		return mutate(func() error {
			return led.CreateSession(req.SessionID, req.ClientID, req.TokenID, req.Scopes, req.ExpiresAt)
		})
	case "rotate":
		return mutate(func() error {
			return led.Rotate(req.TokenID, req.ClientID, req.NewToken, req.Scopes)
		})
	case "revoke":
		return mutate(func() error {
			return led.Revoke(req.SessionID)
		})
	case "advance":
		return mutate(func() error {
			return led.Advance(req.Seconds)
		})
	case "status":
		st, err := led.Status(req.SessionID)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return response{OK: true, Result: st}
	case "history":
		ev, err := led.History(req.SessionID)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return response{OK: true, Result: ev}
	case "chain":
		ch, err := led.TokenChain(req.SessionID)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return response{OK: true, Result: ch}
	default:
		return response{OK: false, Error: "unknown op: " + req.Op}
	}
}
