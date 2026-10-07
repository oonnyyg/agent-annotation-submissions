// Command ignoreaudit audits a directory tree against hierarchical
// .gitignore rules without touching Git state.
//
// Usage:
//
//	ignoreaudit [-json] [-explain <path>] [root]
//
// With -explain it prints why the given root-relative path is kept or
// excluded. Otherwise it prints the kept files, pruned directories and
// skipped entries. -json emits a machine-readable report.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/hierarchical-ignore-auditor/internal/audit"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit a JSON report")
	explain := flag.String("explain", "", "explain why the root-relative path is kept or excluded")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	if *explain != "" {
		ex, err := audit.Explain(root, *explain)
		if err != nil {
			fatal(err)
		}
		if *jsonOut {
			writeJSON(ex)
			return
		}
		status := "KEPT"
		if ex.Ignored {
			status = "EXCLUDED"
		}
		fmt.Printf("%s %s\n  %s\n", status, ex.Path, ex.Reason)
		return
	}

	rep, err := audit.Walk(root)
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		writeJSON(rep)
		return
	}
	fmt.Printf("root: %s\n", rep.Root)
	fmt.Printf("kept files (%d):\n", len(rep.Kept))
	for _, e := range rep.Kept {
		fmt.Printf("  %s  (%s)\n", e.Path, e.Reason)
	}
	fmt.Printf("pruned directories (%d):\n", len(rep.Pruned))
	for _, e := range rep.Pruned {
		fmt.Printf("  %s  (%s: %s:%d %q)\n", e.Path, e.Reason, e.File, e.Line, e.Rule)
	}
	fmt.Printf("skipped entries (%d):\n", len(rep.Skipped))
	for _, e := range rep.Skipped {
		fmt.Printf("  %s  (%s)\n", e.Path, e.Reason)
	}
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ignoreaudit:", err)
	os.Exit(1)
}
