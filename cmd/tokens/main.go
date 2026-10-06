// Command tokens runs the refresh-token family ledger as an offline JSONL
// loop: one operation per line on stdin, one result per line on stdout.
// State is persisted under the directory given by -dir.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/refresh-family-ledger/ledger"
)

type request struct {
	Op        string   `json:"op"`
	Session   string   `json:"session,omitempty"`
	Client    string   `json:"client,omitempty"`
	Token     string   `json:"token,omitempty"`
	NewToken  string   `json:"new_token,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
	Now       int64    `json:"now,omitempty"`
}

type response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

func main() {
	dir := flag.String("dir", "state", "directory holding the ledger archive")
	flag.Parse()
	if err := run(*dir, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tokens:", err)
		os.Exit(1)
	}
}

func run(dir string, in io.Reader, out io.Writer) error {
	l, err := ledger.Load(dir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			enc.Encode(response{OK: false, Error: fmt.Sprintf("line %d: bad json: %v", line, err)})
			continue
		}
		resp, save := apply(l, &req)
		if resp.OK && save {
			if err := ledger.Save(dir, l); err != nil {
				resp = response{OK: false, Error: "persist: " + err.Error()}
			}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// apply executes one operation. The save flag reports whether the ledger may
// have changed and must be persisted before the success answer is emitted.
func apply(l *ledger.Ledger, req *request) (response, bool) {
	switch req.Op {
	case "create":
		if err := l.Create(req.Session, req.Client, req.Scopes, req.Token, req.ExpiresAt); err != nil {
			return fail(err), false
		}
		return response{OK: true, Data: map[string]string{"session": req.Session}}, true
	case "rotate":
		res, err := l.Rotate(req.Token, req.Client, req.NewToken, req.Scopes)
		if err != nil {
			return fail(err), false
		}
		return response{OK: true, Data: res}, true
	case "revoke":
		if err := l.Revoke(req.Token, req.Client); err != nil {
			return fail(err), false
		}
		return response{OK: true, Data: map[string]string{"revoked": req.Token}}, true
	case "advance":
		if err := l.Advance(req.Now); err != nil {
			return fail(err), false
		}
		return response{OK: true, Data: map[string]int64{"now": req.Now}}, true
	case "query":
		view, err := l.Query(req.Session)
		if err != nil {
			return fail(err), false
		}
		return response{OK: true, Data: view}, true
	default:
		return response{OK: false, Error: "unknown op: " + req.Op}, false
	}
}

func fail(err error) response {
	return response{OK: false, Error: err.Error()}
}
