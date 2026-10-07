package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "root directory to audit (read-only)")
	explain := flag.String("explain", "", "explain why a relative path is kept or excluded")
	jsonOut := flag.Bool("json", false, "emit a JSON report")
	flag.Parse()

	rep, entries, err := Walk(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit failed:", err)
		os.Exit(2)
	}

	if *explain != "" {
		ex, err := Explain(*root, *explain, entries)
		if err != nil {
			fmt.Fprintln(os.Stderr, "explain failed:", err)
			os.Exit(2)
		}
		if *jsonOut {
			writeJSON(ex)
		} else {
			printExplanation(ex)
		}
		return
	}

	if *jsonOut {
		writeJSON(rep)
		return
	}
	fmt.Printf("root: %s\n", rep.Root)
	fmt.Printf("kept files (%d):\n", len(rep.Kept))
	for _, p := range rep.Kept {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("ignored files (%d):\n", len(rep.Ignored))
	for _, p := range rep.Ignored {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("pruned directories (%d):\n", len(rep.Pruned))
	for _, p := range rep.Pruned {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("skipped items (%d):\n", len(rep.Skipped))
	for _, s := range rep.Skipped {
		fmt.Printf("  %s [%s] %s\n", s.Path, s.Kind, s.Reason)
	}
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "json encode failed:", err)
		os.Exit(2)
	}
}

func printExplanation(ex *Explanation) {
	fmt.Printf("path:     %s\n", ex.Path)
	fmt.Printf("decision: %s\n", ex.Decision)
	switch ex.Reason {
	case "direct":
		if ex.MatchedBy != nil {
			r := ex.MatchedBy
			fmt.Printf("reason:   last matching rule %s:%d %q (negate=%v)\n", r.File, r.Line, r.Pattern, r.Negate)
		} else {
			fmt.Println("reason:   no rule matched; kept by default")
		}
	case "pruned-dir":
		r := ex.MatchedBy
		fmt.Printf("reason:   directory pruned by %s:%d %q; contents not read\n", r.File, r.Line, r.Pattern)
	case "ancestor":
		r := ex.BlockedBy
		fmt.Printf("reason:   ancestor directory %q pruned by %s:%d %q\n", ex.BlockedAt, r.File, r.Line, r.Pattern)
		fmt.Println("          negations inside a pruned directory cannot re-include it")
	case "skipped":
		fmt.Println("reason:   item is a symlink/junction/special file; never followed")
	default:
		fmt.Println("reason:   path does not exist under the root")
	}
}
