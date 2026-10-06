package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/elf-load-map-audit/internal/elfmap"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: elfmap <query.json>")
	fmt.Fprintln(os.Stderr, `query.json: {"elf_path": "...", "load_bias": "0x0", "reads": [{"vaddr": "0x...", "length": N}]}`)
	os.Exit(2)
}

func main() {
	if len(os.Args) != 2 {
		usage()
	}
	cfgData, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatal(err)
	}
	var cfg elfmap.Query
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		fatal(fmt.Errorf("parse query config: %w", err))
	}
	report, err := elfmap.Run(&cfg)
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "elfmap:", err)
	os.Exit(1)
}
