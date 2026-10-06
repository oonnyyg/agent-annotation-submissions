package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/udp-nat-replay/internal/nat"
)

func run() error {
	configPath := flag.String("config", "config.json", "path to JSON config")
	eventsPath := flag.String("events", "", "path to JSONL events (default stdin)")
	flag.Parse()

	cfgFile, err := os.Open(*configPath)
	if err != nil {
		return err
	}
	defer cfgFile.Close()
	var cfg nat.Config
	if err := json.NewDecoder(cfgFile).Decode(&cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	var events io.Reader = os.Stdin
	if *eventsPath != "" {
		f, err := os.Open(*eventsPath)
		if err != nil {
			return err
		}
		defer f.Close()
		events = f
	}

	table, err := nat.NewTable(cfg)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(events)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	line := 0
	var lastTime int64
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev nat.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if line > 1 && ev.Time < lastTime {
			return fmt.Errorf("line %d: event time %d goes backwards", line, ev.Time)
		}
		lastTime = ev.Time
		res, err := table.Process(ev)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return enc.Encode(table.Summary())
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "natreplay:", err)
		os.Exit(1)
	}
}
