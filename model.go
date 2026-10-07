package main

import (
	"fmt"
	"time"
)

// Zone describes an area. Outdoor zones have unlimited capacity.
type Zone struct {
	ID       string `json:"id"`
	Outdoor  bool   `json:"outdoor"`
	Capacity int    `json:"capacity"` // ignored when Outdoor is true
}

// Credential grants its holder access. Validity is half-open [From, To).
type Credential struct {
	ID     string    `json:"id"`
	Person string    `json:"person"`
	Active bool      `json:"active"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Zones  []string  `json:"zones"` // indoor zones this credential may enter
}

// Person is a tracked individual with an initial zone.
type Person struct {
	ID      string `json:"id"`
	Initial string `json:"initial"`
}

// Reader connects an origin zone to a destination zone.
type Reader struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Config is the full site configuration loaded before replay.
type Config struct {
	Zones       []Zone       `json:"zones"`
	Persons     []Person     `json:"persons"`
	Credentials []Credential `json:"credentials"`
	Readers     []Reader     `json:"readers"`
}

// Event is a card swipe at a reader.
type Event struct {
	Time       time.Time `json:"time"`
	Credential string    `json:"credential"`
	Reader     string    `json:"reader"`
}

// Correction manually relocates a person with a recorded reason.
type Correction struct {
	Time   time.Time `json:"time"`
	Person string    `json:"person"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
}

// EventRecord is a tagged union; exactly one of Swipe/Correction is set.
type EventRecord struct {
	Kind       string      `json:"kind"`
	Swipe      *Event      `json:"swipe,omitempty"`
	Correction *Correction `json:"correction,omitempty"`
	order      int
}

// EventFile is the replay input.
type EventFile struct {
	Events []EventRecord `json:"events"`
}

func (c *Config) validate() error {
	zones := map[string]bool{}
	for _, z := range c.Zones {
		if z.ID == "" {
			return fmt.Errorf("zone with empty id")
		}
		if zones[z.ID] {
			return fmt.Errorf("duplicate zone %q", z.ID)
		}
		zones[z.ID] = true
		if !z.Outdoor && z.Capacity < 0 {
			return fmt.Errorf("zone %q has negative capacity", z.ID)
		}
	}
	persons := map[string]bool{}
	for _, p := range c.Persons {
		if p.ID == "" {
			return fmt.Errorf("person with empty id")
		}
		if persons[p.ID] {
			return fmt.Errorf("duplicate person %q", p.ID)
		}
		persons[p.ID] = true
		if !zones[p.Initial] {
			return fmt.Errorf("person %q initial zone %q not defined", p.ID, p.Initial)
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
		creds[cr.ID] = true
		if !persons[cr.Person] {
			return fmt.Errorf("credential %q references unknown person %q", cr.ID, cr.Person)
		}
		if !cr.From.Before(cr.To) {
			return fmt.Errorf("credential %q validity [%s,%s) is empty", cr.ID, cr.From, cr.To)
		}
		for _, z := range cr.Zones {
			if !zones[z] {
				return fmt.Errorf("credential %q references unknown zone %q", cr.ID, z)
			}
		}
	}
	readers := map[string]bool{}
	for _, r := range c.Readers {
		if r.ID == "" {
			return fmt.Errorf("reader with empty id")
		}
		if readers[r.ID] {
			return fmt.Errorf("duplicate reader %q", r.ID)
		}
		readers[r.ID] = true
		if !zones[r.From] || !zones[r.To] {
			return fmt.Errorf("reader %q references unknown zone (%q -> %q)", r.ID, r.From, r.To)
		}
		if r.From == r.To {
			return fmt.Errorf("reader %q connects zone %q to itself", r.ID, r.From)
		}
	}
	return nil
}
