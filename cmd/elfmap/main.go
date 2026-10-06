package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/elf-load-map-audit/internal/elfmap"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "elfmap:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("elfmap", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to JSON query configuration")
	pretty := fs.Bool("pretty", true, "pretty-print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: elfmap [-config file.json] <elf-file>")
	}
	elfPath := fs.Arg(0)

	cfg := elfmap.Config{Reads: []elfmap.ReadQuery{}}
	if *configPath != "" {
		c, err := elfmap.LoadConfig(*configPath)
		if err != nil {
			return err
		}
		cfg = c
	}

	rep, err := elfmap.Analyze(elfPath, cfg)
	if err != nil {
		return err
	}

	var out []byte
	if *pretty {
		out, err = json.MarshalIndent(rep, "", "  ")
	} else {
		out, err = json.Marshal(rep)
	}
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(out, '\n'))
	return err
}
