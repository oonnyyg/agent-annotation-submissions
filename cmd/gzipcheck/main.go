// Command gzipcheck validates gzip files member by member and optionally
// publishes the concatenated plaintext to a new file.
//
// Exit codes: 0 = valid (and output published if requested),
// 1 = validation failed, 2 = usage or operational error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"example.com/gzip-member-intake/internal/gzipx"
)

type config struct {
	Input            string `json:"input"`
	Output           string `json:"output"`
	CheckOnly        bool   `json:"check_only"`
	MaxHeaderBytes   int64  `json:"max_header_bytes"`
	MaxMembers       int    `json:"max_members"`
	MaxExpandedBytes int64  `json:"max_expanded_bytes"`
}

type output struct {
	gzipx.Report
	Input  string `json:"input"`
	Output string `json:"output,omitempty"`
}

func main() {
	configPath := flag.String("config", "", "path to JSON config file (required)")
	inputOverride := flag.String("input", "", "override input gzip path from config")
	checkOnly := flag.Bool("check", false, "force check-only mode (no output file)")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "gzipcheck: -config is required")
		os.Exit(2)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gzipcheck:", err)
		os.Exit(2)
	}
	if *inputOverride != "" {
		cfg.Input = *inputOverride
	}
	if *checkOnly {
		cfg.CheckOnly = true
		cfg.Output = ""
	}
	if cfg.Input == "" {
		fmt.Fprintln(os.Stderr, "gzipcheck: no input file configured")
		os.Exit(2)
	}

	rep, code := run(cfg)
	out := output{Report: rep, Input: cfg.Input}
	if !cfg.CheckOnly && cfg.Output != "" {
		out.Output = cfg.Output
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "gzipcheck: encoding report:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func loadConfig(path string) (config, error) {
	var cfg config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config: %v", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %v", err)
	}
	return cfg, nil
}

// run inspects the input and, on full success, publishes the output file.
// It returns the report and the process exit code.
func run(cfg config) (gzipx.Report, int) {
	limits := gzipx.Limits{
		MaxHeaderBytes:   cfg.MaxHeaderBytes,
		MaxMembers:       cfg.MaxMembers,
		MaxExpandedBytes: cfg.MaxExpandedBytes,
	}

	in, err := os.Open(cfg.Input)
	if err != nil {
		return gzipx.Report{Error: fmt.Sprintf("opening input: %v", err)}, 2
	}
	defer in.Close()

	if cfg.CheckOnly || cfg.Output == "" {
		rep := gzipx.Inspect(in, limits, nil)
		if !rep.OK {
			return rep, 1
		}
		return rep, 0
	}

	// Never overwrite an existing target.
	if _, err := os.Lstat(cfg.Output); err == nil {
		return gzipx.Report{Error: fmt.Sprintf("output %q already exists", cfg.Output)}, 2
	} else if !os.IsNotExist(err) {
		return gzipx.Report{Error: fmt.Sprintf("checking output: %v", err)}, 2
	}

	dir := filepath.Dir(cfg.Output)
	tmp, err := os.CreateTemp(dir, ".gzipcheck-*")
	if err != nil {
		return gzipx.Report{Error: fmt.Sprintf("creating temp file: %v", err)}, 2
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	rep := gzipx.Inspect(in, limits, tmp)
	if !rep.OK {
		cleanup()
		return rep, 1
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return gzipx.Report{Error: fmt.Sprintf("closing temp file: %v", err)}, 2
	}
	// Re-check before publishing; best effort against races.
	if _, err := os.Lstat(cfg.Output); err == nil {
		os.Remove(tmpName)
		return gzipx.Report{Error: fmt.Sprintf("output %q already exists", cfg.Output)}, 2
	}
	if err := os.Rename(tmpName, cfg.Output); err != nil {
		os.Remove(tmpName)
		return gzipx.Report{Error: fmt.Sprintf("publishing output: %v", err)}, 2
	}
	return rep, 0
}
