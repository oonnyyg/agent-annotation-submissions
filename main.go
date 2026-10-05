package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type Report struct {
	Plan    bool     `json:"plan"`
	Goals   []string `json:"goals"`
	Results []Result `json:"results"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		manifestPath = flag.String("manifest", "build.json", "path to the JSON build manifest")
		root         = flag.String("root", ".", "work root; all files and commands stay inside it")
		cacheDir     = flag.String("cache", ".buildcache", "cache directory")
		logDir       = flag.String("logs", "build-logs", "per-target log directory")
		reportPath   = flag.String("report", "report.json", "JSON report output path")
		planOnly     = flag.Bool("plan", false, "preview the plan without running commands or touching the cache")
	)
	flag.Parse()
	goals := flag.Args()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	m, err := LoadManifest(*manifestPath, absRoot)
	if err != nil {
		return err
	}
	// Validate the whole graph before running anything.
	if err := ValidateGraph(m); err != nil {
		return err
	}
	if len(goals) == 0 {
		goals = m.Order
	}
	set, err := Closure(m, goals)
	if err != nil {
		return err
	}
	order := TopoOrder(m, set)

	cache, err := LoadCache(*cacheDir)
	if err != nil {
		return err
	}
	results := Run(m, cache, absRoot, *logDir, order, *planOnly)
	if !*planOnly {
		if err := cache.Save(); err != nil {
			return fmt.Errorf("save cache: %w", err)
		}
	}
	report := Report{Plan: *planOnly, Goals: goals, Results: results}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*reportPath, data, 0o644); err != nil {
		return err
	}
	for _, r := range results {
		fmt.Printf("%-8s %-15s %s\n", r.Status, r.Name, r.Reason)
	}
	fmt.Println("report written to", *reportPath)
	for _, r := range results {
		if r.Status == "failed" {
			return fmt.Errorf("target %q failed", r.Name)
		}
	}
	return nil
}
