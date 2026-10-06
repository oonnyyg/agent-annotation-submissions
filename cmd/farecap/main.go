package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/transit-fare-capping/internal/farecap"
)

type output struct {
	Config      string                     `json:"config"`
	Input       string                     `json:"input"`
	State       string                     `json:"state"`
	Results     []farecap.CardResult       `json:"results"`
	Events      []farecap.LineResult       `json:"events"`
	Revocations []farecap.RevocationRecord `json:"revocations"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "farecap:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("farecap", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "path to JSON rate configuration")
	inputPath := fs.String("input", "", "path to JSONL ride/revocation input (- or omitted for stdin)")
	statePath := fs.String("state", "farecap-state.json", "path to persisted state (saved after the run)")
	query := fs.Bool("query", false, "only load state/config and print current totals; do not ingest input")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := farecap.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	state := &farecap.State{}
	if data, err := os.ReadFile(*statePath); err == nil {
		if err := json.Unmarshal(data, state); err != nil {
			return fmt.Errorf("parse state: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read state: %w", err)
	}
	store := farecap.NewStore(cfg, state)

	out := output{Config: *configPath, State: *statePath}

	if !*query {
		var f *os.File
		switch {
		case *inputPath == "" || *inputPath == "-":
			f = os.Stdin
			out.Input = "stdin"
		default:
			f, err = os.Open(*inputPath)
			if err != nil {
				return fmt.Errorf("open input: %w", err)
			}
			defer f.Close()
			out.Input = *inputPath
		}
		events, err := ingest(store, f)
		if err != nil {
			return err
		}
		out.Events = events
		if err := farecap.SaveState(*statePath, store.State()); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	results, err := store.Results()
	if err != nil {
		return err
	}
	out.Results = results
	out.Revocations = store.State().Revokes

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func ingest(store *farecap.Store, f *os.File) ([]farecap.LineResult, error) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var results []farecap.LineResult
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "//") {
			continue
		}
		e, err := farecap.DecodeEventForCLI([]byte(raw))
		if err != nil {
			results = append(results, farecap.LineResult{
				Line:   line,
				Status: farecap.StatusRejected,
				Reason: err.Error(),
			})
			continue
		}
		results = append(results, store.Apply(e, line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	return results, nil
}
