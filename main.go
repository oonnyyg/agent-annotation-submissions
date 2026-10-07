// Command khcheck audits SSH host keys against a known_hosts sample
// offline. It never opens network connections and never reads or
// modifies any file other than those explicitly named on the command
// line.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type queryFlags []string

func (q *queryFlags) String() string { return strings.Join(*q, ";") }
func (q *queryFlags) Set(v string) error {
	*q = append(*q, v)
	return nil
}

// Report is the top-level JSON output.
type Report struct {
	KnownHosts    string       `json:"knownHosts"`
	AuditComplete bool         `json:"auditComplete"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	Results       []Result     `json:"results"`
}

type query struct {
	target  Target
	keyType string
	keyB64  string
}

// parseQuery parses "host[:port] <key-type> <base64-key>".
func parseQuery(s string) (query, error) {
	fields := strings.Fields(s)
	if len(fields) != 3 {
		return query{}, fmt.Errorf("want 3 fields: host[:port] key-type base64-key")
	}
	t, err := parseTarget(fields[0])
	if err != nil {
		return query{}, err
	}
	return query{target: t, keyType: fields[1], keyB64: fields[2]}, nil
}

// parseTarget parses "host", "host:port" or "[host]:port"; default 22.
func parseTarget(s string) (Target, error) {
	host, portStr := s, "22"
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return Target{}, fmt.Errorf("bad bracketed target %q", s)
		}
		host, portStr = s[1:end], s[end+2:]
	} else if i := strings.LastIndex(s, ":"); i >= 0 {
		host, portStr = s[:i], s[i+1:]
	}
	port, err := strconv.Atoi(portStr)
	if host == "" || err != nil || port < 1 || port > 65535 {
		return Target{}, fmt.Errorf("bad target %q", s)
	}
	return Target{Host: host, Port: port}, nil
}

func main() {
	var khPath, queriesPath string
	var queries queryFlags
	flag.StringVar(&khPath, "known-hosts", "", "path to known_hosts sample (required, explicit)")
	flag.StringVar(&queriesPath, "queries", "", "optional file with one query per line")
	flag.Var(&queries, "query", `query "host[:port] key-type base64-key" (repeatable)`)
	flag.Parse()

	if khPath == "" {
		fmt.Fprintln(os.Stderr, "error: -known-hosts is required; no default key files are read")
		os.Exit(2)
	}
	data, err := os.ReadFile(khPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	kh := ParseFile(string(data))

	var qs []query
	for _, raw := range queries {
		q, err := parseQuery(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: bad -query %q: %v\n", raw, err)
			os.Exit(2)
		}
		qs = append(qs, q)
	}
	if queriesPath != "" {
		qd, err := os.ReadFile(queriesPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		for i, line := range strings.Split(string(qd), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			q, err := parseQuery(line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s:%d: %v\n", queriesPath, i+1, err)
				os.Exit(2)
			}
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one -query or -queries file is required")
		os.Exit(2)
	}

	complete := len(kh.Diagnostics) == 0
	rep := Report{KnownHosts: khPath, AuditComplete: complete, Diagnostics: kh.Diagnostics}
	for _, q := range qs {
		r := audit(kh, q.target, q.keyType, q.keyB64)
		if !complete && r.Verdict == VerdictTrusted {
			r.Verdict = "trusted-incomplete"
			r.Reason += "; sample has parse errors, audit is incomplete"
		}
		rep.Results = append(rep.Results, r)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
