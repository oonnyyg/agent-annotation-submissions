package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"
)

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// validateEvents rejects malformed records and dangling references before replay.
func validateEvents(cfg *Config, ef *EventFile) error {
	creds := map[string]bool{}
	for _, c := range cfg.Credentials {
		creds[c.ID] = true
	}
	readers := map[string]bool{}
	for _, r := range cfg.Readers {
		readers[r.ID] = true
	}
	zones := map[string]bool{}
	for _, z := range cfg.Zones {
		zones[z.ID] = true
	}
	persons := map[string]bool{}
	for _, p := range cfg.Persons {
		persons[p.ID] = true
	}
	for i, rec := range ef.Events {
		switch rec.Kind {
		case "swipe":
			if rec.Swipe == nil || rec.Correction != nil {
				return fmt.Errorf("event %d: swipe record malformed", i)
			}
			if !creds[rec.Swipe.Credential] {
				return fmt.Errorf("event %d: unknown credential %q", i, rec.Swipe.Credential)
			}
			if !readers[rec.Swipe.Reader] {
				return fmt.Errorf("event %d: unknown reader %q", i, rec.Swipe.Reader)
			}
		case "correction":
			if rec.Correction == nil || rec.Swipe != nil {
				return fmt.Errorf("event %d: correction record malformed", i)
			}
			if !persons[rec.Correction.Person] {
				return fmt.Errorf("event %d: unknown person %q", i, rec.Correction.Person)
			}
			if !zones[rec.Correction.To] {
				return fmt.Errorf("event %d: unknown zone %q", i, rec.Correction.To)
			}
			if rec.Correction.Reason == "" {
				return fmt.Errorf("event %d: correction requires a reason", i)
			}
		default:
			return fmt.Errorf("event %d: unknown kind %q", i, rec.Kind)
		}
	}
	return nil
}

func main() {
	configPath := flag.String("config", "config.json", "site configuration file")
	eventsPath := flag.String("events", "events.json", "event file")
	at := flag.String("at", "", "query roster after all events at or before this UTC time (RFC3339)")
	flag.Parse()

	var cfg Config
	if err := loadJSON(*configPath, &cfg); err != nil {
		fatal("load config: %v", err)
	}
	if err := cfg.validate(); err != nil {
		fatal("invalid config: %v", err)
	}
	var ef EventFile
	if err := loadJSON(*eventsPath, &ef); err != nil {
		fatal("load events: %v", err)
	}
	if err := validateEvents(&cfg, &ef); err != nil {
		fatal("invalid events: %v", err)
	}

	var cutoff *time.Time
	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			fatal("invalid -at time: %v", err)
		}
		cutoff = &t
	}

	eng := NewEngine(&cfg)
	eng.Replay(ef.Events)

	for _, d := range eng.Results {
		status := "DENY "
		if d.OK {
			status = "GRANT"
		}
		fmt.Printf("#%02d %s %-10s %s person=%s %s->%s %s\n",
			d.Seq, d.Time.UTC().Format(time.RFC3339), d.Kind, status, d.Person, d.FromZone, d.ToZone, d.Reason)
	}

	if cutoff != nil {
		snap := NewEngine(&cfg)
		// Re-apply only events at or before cutoff for the roster query.
		var recs []EventRecord
		for _, r := range ef.Events {
			if !r.time().After(*cutoff) {
				recs = append(recs, r)
			}
		}
		snap.Replay(recs)
		fmt.Printf("\nRoster at %s:\n", cutoff.UTC().Format(time.RFC3339))
		printRoster(snap.Roster())
	}

	fmt.Println("\nFinal occupancy:")
	occ := eng.Occupancy()
	keys := make([]string, 0, len(occ))
	for z := range occ {
		keys = append(keys, z)
	}
	sort.Strings(keys)
	for _, z := range keys {
		fmt.Printf("  %s: %d\n", z, occ[z])
	}
}

func printRoster(r map[string][]string) {
	keys := make([]string, 0, len(r))
	for z := range r {
		keys = append(keys, z)
	}
	sort.Strings(keys)
	for _, z := range keys {
		fmt.Printf("  %s (%d): %v\n", z, len(r[z]), r[z])
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
