// Command batchrename previews and executes transactional batch renames
// within a single directory.
//
// Usage:
//
//	batchrename preview -dir DIR old1=new1 [old2=new2 ...] [-plan plan.json]
//	batchrename execute -plan plan.json [-fail-before N]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	rename "example.com/transactional-batch-rename"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "preview":
		err = cmdPreview(os.Args[2:])
	case "execute":
		err = cmdExecute(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  batchrename preview -dir DIR old=new [old=new ...] [-plan file]
  batchrename execute -plan file [-fail-before N]`)
}

func cmdPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	dir := fs.String("dir", ".", "directory containing the files")
	planOut := fs.String("plan", "", "write plan JSON to this file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var pairs [][2]string
	for _, a := range fs.Args() {
		old, newName, ok := strings.Cut(a, "=")
		if !ok {
			return fmt.Errorf("invalid mapping %q, want old=new", a)
		}
		pairs = append(pairs, [2]string{old, newName})
	}
	plan, err := rename.BuildPlan(*dir, pairs)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	if *planOut == "" {
		fmt.Println(string(data))
		return nil
	}
	if err := os.WriteFile(*planOut, data, 0o644); err != nil {
		return err
	}
	fmt.Println("plan written to", *planOut)
	return nil
}

func cmdExecute(args []string) error {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	planFile := fs.String("plan", "", "plan JSON produced by preview")
	failBefore := fs.Int("fail-before", -1, "inject a failure before step N (testing)")
	failRollback := fs.Int("fail-rollback", -1, "inject a failure into rollback step N (testing)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *planFile == "" {
		return fmt.Errorf("-plan is required")
	}
	data, err := os.ReadFile(*planFile)
	if err != nil {
		return err
	}
	var plan rename.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("invalid plan: %w", err)
	}
	res, err := rename.Execute(&plan, rename.Options{
		FailBeforeStep:   *failBefore,
		FailRollbackStep: *failRollback,
	})
	if res != nil {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	}
	if err != nil {
		return err
	}
	if len(res.RollbackErrors) > 0 {
		return fmt.Errorf("completed with %d rollback/cleanup problem(s)", len(res.RollbackErrors))
	}
	return nil
}
