package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/udp-nat-replay/internal/natreplay"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config file")
	eventsPath := flag.String("events", "", "path to JSONL events file")
	flag.Parse()
	if *configPath == "" || *eventsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: natreplay -config config.json -events events.jsonl")
		os.Exit(2)
	}
	if err := run(*configPath, *eventsPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(configPath, eventsPath string) error {
	cfgBytes, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg natreplay.Config
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	table, err := natreplay.NewTable(cfg)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	f, err := os.Open(eventsPath)
	if err != nil {
		return err
	}
	defer f.Close()

	rep := natreplay.NewReplayer(table)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var lastTime int64
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev natreplay.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("events line %d: %w", lineNo, err)
		}
		if lineNo > 1 && ev.Time < lastTime {
			return fmt.Errorf("events line %d: time goes backwards", lineNo)
		}
		lastTime = ev.Time
		res, err := rep.Handle(ev)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(res)
		fmt.Fprintln(out, string(b))
	}
	if err := sc.Err(); err != nil {
		return err
	}
	sum, _ := json.Marshal(rep.Summary())
	fmt.Fprintln(out, string(sum))
	return nil
}
