// Command ssh-known-host-audit audits SSH host keys against an explicitly
// specified known_hosts sample, fully offline. It never opens network
// connections, never reads the user's real key directories, and never
// modifies the sample files.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/ssh-known-host-audit/knownhosts"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: ssh-known-host-audit -knownhosts FILE [flags]

Audits SSH host keys against a known_hosts sample, offline.

Flags:
  -knownhosts FILE   known_hosts sample to audit against (required, explicit)
  -query SPEC        query: "target key-type base64key"; repeatable
  -queries FILE      batch file, one query per line (same format)
  -pretty            indent JSON output

Target forms: host, host:port, [host]:port. Bare host means port 22.
`)
}

func main() {
	var khFile, queriesFile string
	var pretty bool
	var queries multiFlag
	flag.StringVar(&khFile, "knownhosts", "", "known_hosts sample file (required)")
	flag.StringVar(&queriesFile, "queries", "", "file with one query per line")
	flag.Var(&queries, "query", "query spec: \"target key-type base64key\"")
	flag.BoolVar(&pretty, "pretty", false, "indent JSON output")
	flag.Usage = usage
	flag.Parse()

	if khFile == "" {
		fmt.Fprintln(os.Stderr, "error: -knownhosts is required; only explicitly specified files are read")
		os.Exit(2)
	}
	content, err := os.ReadFile(khFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", khFile, err)
		os.Exit(2)
	}
	sample := knownhosts.Parse(string(content))

	var specs []string
	specs = append(specs, queries...)
	if queriesFile != "" {
		b, err := os.ReadFile(queriesFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", queriesFile, err)
			os.Exit(2)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				specs = append(specs, line)
			}
		}
	}
	if len(specs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no queries given (use -query or -queries)")
		os.Exit(2)
	}

	out := report{Sample: khFile, Diagnostics: sample.Diagnostics}
	for _, spec := range specs {
		out.Results = append(out.Results, runQuery(sample, spec))
	}

	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "error: encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ";") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

type report struct {
	Sample      string                  `json:"sample"`
	Diagnostics []knownhosts.Diagnostic `json:"diagnostics,omitempty"`
	Results     []knownhosts.Result     `json:"results"`
}

func runQuery(sample *knownhosts.Sample, spec string) knownhosts.Result {
	fields := strings.Fields(spec)
	if len(fields) != 3 {
		return knownhosts.Result{
			Target:  spec,
			Verdict: knownhosts.VerdictInvalid,
			Reason:  fmt.Sprintf("query must have 3 fields (target key-type base64key), got %d", len(fields)),
		}
	}
	target, err := knownhosts.ParseTarget(fields[0])
	if err != nil {
		return knownhosts.Result{Target: fields[0], Verdict: knownhosts.VerdictInvalid, Reason: err.Error()}
	}
	return sample.Audit(knownhosts.Query{Target: target, KeyType: fields[1], KeyB64: fields[2]})
}
