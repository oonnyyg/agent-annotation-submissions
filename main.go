package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config JSON")
	eventsPath := flag.String("events", "events.json", "path to events JSON")
	at := flag.String("at", "", "RFC3339 UTC time: print zone rosters after all events up to this time")
	summary := flag.Bool("summary", false, "print final occupancy summary")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	events, err := loadEvents(*eventsPath)
	if err != nil {
		fatal(err)
	}

	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			fatal(fmt.Errorf("invalid -at time: %w", err))
		}
		printRoster(cfg, events, t.UTC())
		return
	}

	decisions := Replay(cfg, events)
	for _, d := range decisions {
		printDecision(d)
	}
	if *summary {
		printSummary(cfg, decisions)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func printDecision(d Decision) {
	status := "DENY "
	if d.Allowed {
		status = "ALLOW"
	}
	ev := d.Event
	desc := ""
	switch ev.Type {
	case "swipe":
		desc = fmt.Sprintf("swipe cred=%s reader=%s", ev.Credential, ev.Reader)
	case "correct":
		desc = fmt.Sprintf("correct person=%s to=%s", ev.Person, ev.To)
	default:
		desc = ev.Type
	}
	line := fmt.Sprintf("#%d %s %s %-45s %s", d.Seq, ev.Time.UTC().Format(time.RFC3339), status, desc, d.Reason)
	if d.Allowed {
		line += fmt.Sprintf("  [%s -> %s]", d.FromZone, d.ToZone)
	}
	fmt.Println(line)
	fmt.Printf("    occupancy: %s\n", formatOccupancy(d.Occupancy))
}

func formatOccupancy(occ map[string]int) string {
	keys := make([]string, 0, len(occ))
	for k := range occ {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, occ[k]))
	}
	return strings.Join(parts, " ")
}

func printSummary(cfg *Config, decisions []Decision) {
	var occ map[string]int
	if len(decisions) > 0 {
		occ = decisions[len(decisions)-1].Occupancy
	} else {
		occ = NewEngine(cfg).snapshot()
	}
	fmt.Println("final occupancy:")
	for _, z := range cfg.Zones {
		cap := "unlimited"
		if !z.Outdoor {
			cap = fmt.Sprintf("%d", z.Capacity)
		}
		fmt.Printf("  %-12s %d / %s\n", z.ID, occ[z.ID], cap)
	}
}

func printRoster(cfg *Config, events []Event, t time.Time) {
	roster, occ := RosterAt(cfg, events, t)
	fmt.Printf("roster after events up to %s:\n", t.Format(time.RFC3339))
	for _, z := range cfg.Zones {
		people := strings.Join(roster[z.ID], ", ")
		if people == "" {
			people = "(empty)"
		}
		fmt.Printf("  %-12s count=%d  %s\n", z.ID, occ[z.ID], people)
	}
}
