package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Zone describes an area. Outdoor zones have unlimited capacity.
type Zone struct {
	ID       string `json:"id"`
	Capacity int    `json:"capacity"`
	Outdoor  bool   `json:"outdoor"`
}

// Person has a single confirmed location at any time.
type Person struct {
	ID          string `json:"id"`
	InitialZone string `json:"initial_zone"`
}

// Credential is bound to one person. Validity is half-open: [ValidFrom, ValidTo).
type Credential struct {
	ID        string    `json:"id"`
	Person    string    `json:"person"`
	Enabled   bool      `json:"enabled"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
	Zones     []string  `json:"zones"` // indoor zones this credential may enter
}

// Reader connects exactly one origin zone to one destination zone.
type Reader struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Config is the full static configuration for a replay.
type Config struct {
	Zones       []Zone       `json:"zones"`
	People      []Person     `json:"people"`
	Credentials []Credential `json:"credentials"`
	Readers     []Reader     `json:"readers"`
}

// Event is one replay input. Type is "swipe" or "correct".
type Event struct {
	Time       time.Time `json:"time"`
	Type       string    `json:"type"`
	Credential string    `json:"credential,omitempty"`
	Reader     string    `json:"reader,omitempty"`
	Person     string    `json:"person,omitempty"`
	To         string    `json:"to,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func loadEvents(path string) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("parse events: %w", err)
	}
	// Sort by time; stable so equal timestamps keep input order.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	return events, nil
}

func (c *Config) validate() error {
	zones := map[string]Zone{}
	outdoorCount := 0
	for _, z := range c.Zones {
		if z.ID == "" {
			return fmt.Errorf("zone with empty id")
		}
		if _, dup := zones[z.ID]; dup {
			return fmt.Errorf("duplicate zone %q", z.ID)
		}
		if !z.Outdoor && z.Capacity < 0 {
			return fmt.Errorf("zone %q has negative capacity", z.ID)
		}
		if z.Outdoor {
			outdoorCount++
		}
		zones[z.ID] = z
	}
	if outdoorCount == 0 {
		return fmt.Errorf("no outdoor zone defined")
	}

	people := map[string]bool{}
	occupancy := map[string]int{}
	for _, p := range c.People {
		if p.ID == "" {
			return fmt.Errorf("person with empty id")
		}
		if people[p.ID] {
			return fmt.Errorf("duplicate person %q", p.ID)
		}
		z, ok := zones[p.InitialZone]
		if !ok {
			return fmt.Errorf("person %q initial zone %q does not exist", p.ID, p.InitialZone)
		}
		people[p.ID] = true
		occupancy[z.ID]++
		if !z.Outdoor && occupancy[z.ID] > z.Capacity {
			return fmt.Errorf("initial positions exceed capacity of zone %q", z.ID)
		}
	}

	creds := map[string]bool{}
	for _, cr := range c.Credentials {
		if cr.ID == "" {
			return fmt.Errorf("credential with empty id")
		}
		if creds[cr.ID] {
			return fmt.Errorf("duplicate credential %q", cr.ID)
		}
		if !people[cr.Person] {
			return fmt.Errorf("credential %q references unknown person %q", cr.ID, cr.Person)
		}
		if !cr.ValidFrom.Before(cr.ValidTo) {
			return fmt.Errorf("credential %q has empty validity window", cr.ID)
		}
		for _, zid := range cr.Zones {
			z, ok := zones[zid]
			if !ok {
				return fmt.Errorf("credential %q references unknown zone %q", cr.ID, zid)
			}
			if z.Outdoor {
				return fmt.Errorf("credential %q lists outdoor zone %q; exit needs no permission", cr.ID, zid)
			}
		}
		creds[cr.ID] = true
	}

	readers := map[string]bool{}
	for _, r := range c.Readers {
		if r.ID == "" {
			return fmt.Errorf("reader with empty id")
		}
		if readers[r.ID] {
			return fmt.Errorf("duplicate reader %q", r.ID)
		}
		if _, ok := zones[r.From]; !ok {
			return fmt.Errorf("reader %q references unknown from-zone %q", r.ID, r.From)
		}
		if _, ok := zones[r.To]; !ok {
			return fmt.Errorf("reader %q references unknown to-zone %q", r.ID, r.To)
		}
		if r.From == r.To {
			return fmt.Errorf("reader %q has identical from/to zone %q", r.ID, r.From)
		}
		readers[r.ID] = true
	}
	return nil
}
