// Command tbr previews and executes transactional batch renames in one
// directory. Usage:
//
//	tbr preview -dir DIR old1=new1 old2=new2 ...   (writes JSON plan to stdout)
//	tbr execute -plan plan.json [-fail-before N]   (executes a saved plan)
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/transactional-batch-rename/rename"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "preview":
		preview(os.Args[2:])
	case "execute":
		execute(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  tbr preview -dir DIR old=new ...")
	fmt.Fprintln(os.Stderr, "  tbr execute -plan FILE [-fail-before N]")
}

func preview(args []string) {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory containing the files")
	fs.Parse(args)
	var pairs [][2]string
	for _, a := range fs.Args() {
		old, newName, ok := strings.Cut(a, "=")
		if !ok {
			fatal(fmt.Errorf("mapping %q must be old=new", a))
		}
		pairs = append(pairs, [2]string{old, newName})
	}
	plan, err := rename.BuildPlan(*dir, pairs)
	if err != nil {
		fatal(err)
	}
	if err := rename.SavePlan(plan, os.Stdout); err != nil {
		fatal(err)
	}
}

func execute(args []string) {
	fs := flag.NewFlagSet("execute", flag.ExitOnError)
	planPath := fs.String("plan", "", "plan JSON file from preview")
	failBefore := fs.Int("fail-before", 0, "inject a failure before this 1-based rename step")
	fs.Parse(args)
	if *planPath == "" {
		fatal(fmt.Errorf("-plan is required"))
	}
	plan, err := rename.LoadPlan(*planPath)
	if err != nil {
		fatal(err)
	}
	res, err := rename.Execute(plan, *failBefore)
	if res != nil {
		for i, s := range res.Steps {
			fmt.Printf("step %d: %s -> %s\n", i+1, s.From, s.To)
		}
		for _, s := range res.Rollback {
			fmt.Printf("rollback: %s -> %s\n", s.From, s.To)
		}
		for _, o := range res.Orphans {
			fmt.Printf("content preserved at: %s\n", o)
		}
		for _, f := range res.Final {
			fmt.Printf("final: %s\n", f)
		}
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
