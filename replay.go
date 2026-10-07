package main

import (
	"fmt"
	"sort"
	"time"
)

// Decision is the per-event outcome of a replay.
type Decision struct {
	Seq       int
	Event     Event
	Allowed   bool
	Reason    string
	Person    string
	FromZone  string
	ToZone    string
	Occupancy map[string]int // snapshot after this event
}

// Engine replays events against a validated config.
type Engine struct {
	cfg       *Config
	zones     map[string]Zone
	people    map[string]Person
	creds     map[string]Credential
	readers   map[string]Reader
	location  map[string]string // person -> zone
	occupancy map[string]int    // zone -> count
}

func NewEngine(cfg *Config) *Engine {
	e := &Engine{
		cfg:       cfg,
		zones:     map[string]Zone{},
		people:    map[string]Person{},
		creds:     map[string]Credential{},
		readers:   map[string]Reader{},
		location:  map[string]string{},
		occupancy: map[string]int{},
	}
	for _, z := range cfg.Zones {
		e.zones[z.ID] = z
		e.occupancy[z.ID] = 0
	}
	for _, p := range cfg.People {
		e.people[p.ID] = p
		e.location[p.ID] = p.InitialZone
		e.occupancy[p.InitialZone]++
	}
	for _, c := range cfg.Credentials {
		e.creds[c.ID] = c
	}
	for _, r := range cfg.Readers {
		e.readers[r.ID] = r
	}
	return e
}

func (e *Engine) snapshot() map[string]int {
	s := make(map[string]int, len(e.occupancy))
	for k, v := range e.occupancy {
		s[k] = v
	}
	return s
}

// move atomically relocates a person and updates both zone counts.
func (e *Engine) move(person, from, to string) {
	e.location[person] = to
	e.occupancy[from]--
	e.occupancy[to]++
}

func (e *Engine) fits(z Zone) bool {
	return z.Outdoor || e.occupancy[z.ID] < z.Capacity
}

// apply processes one event and records the decision.
func (e *Engine) apply(seq int, ev Event) Decision {
	d := Decision{Seq: seq, Event: ev}
	switch ev.Type {
	case "swipe":
		d = e.applySwipe(d, ev)
	case "correct":
		d = e.applyCorrect(d, ev)
	default:
		d.Reason = fmt.Sprintf("unknown event type %q", ev.Type)
	}
	d.Occupancy = e.snapshot()
	return d
}

func (e *Engine) deny(d Decision, person, format string, args ...any) Decision {
	d.Allowed = false
	d.Person = person
	d.Reason = fmt.Sprintf(format, args...)
	return d
}

func (e *Engine) applySwipe(d Decision, ev Event) Decision {
	cr, ok := e.creds[ev.Credential]
	if !ok {
		return e.deny(d, "", "unknown credential %q", ev.Credential)
	}
	d.Person = cr.Person
	r, ok := e.readers[ev.Reader]
	if !ok {
		return e.deny(d, cr.Person, "unknown reader %q", ev.Reader)
	}
	if !cr.Enabled {
		return e.deny(d, cr.Person, "credential %q is disabled", cr.ID)
	}
	t := ev.Time.UTC()
	if t.Before(cr.ValidFrom) || !t.Before(cr.ValidTo) {
		return e.deny(d, cr.Person, "credential %q not valid at %s (valid [%s, %s))",
			cr.ID, t.Format(time.RFC3339), cr.ValidFrom.Format(time.RFC3339), cr.ValidTo.Format(time.RFC3339))
	}
	cur := e.location[cr.Person]
	if cur != r.From {
		return e.deny(d, cr.Person, "person %q is in %q, not at reader origin %q", cr.Person, cur, r.From)
	}
	to := e.zones[r.To]
	if !to.Outdoor && !contains(cr.Zones, r.To) {
		return e.deny(d, cr.Person, "credential %q lacks permission for zone %q", cr.ID, r.To)
	}
	if !e.fits(to) {
		return e.deny(d, cr.Person, "zone %q is at capacity %d", to.ID, to.Capacity)
	}
	e.move(cr.Person, r.From, r.To)
	d.Allowed = true
	d.FromZone = r.From
	d.ToZone = r.To
	d.Reason = "swipe accepted"
	return d
}

func (e *Engine) applyCorrect(d Decision, ev Event) Decision {
	p, ok := e.people[ev.Person]
	if !ok {
		return e.deny(d, ev.Person, "unknown person %q", ev.Person)
	}
	d.Person = p.ID
	to, ok := e.zones[ev.To]
	if !ok {
		return e.deny(d, p.ID, "unknown target zone %q", ev.To)
	}
	if ev.Reason == "" {
		return e.deny(d, p.ID, "correction requires a reason")
	}
	from := e.location[p.ID]
	if from == to.ID {
		return e.deny(d, p.ID, "person %q already in zone %q", p.ID, to.ID)
	}
	if !e.fits(to) {
		return e.deny(d, p.ID, "zone %q is at capacity %d", to.ID, to.Capacity)
	}
	e.move(p.ID, from, to.ID)
	d.Allowed = true
	d.FromZone = from
	d.ToZone = to.ID
	d.Reason = "correction: " + ev.Reason
	return d
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Replay runs all events and returns every decision.
func Replay(cfg *Config, events []Event) []Decision {
	e := NewEngine(cfg)
	out := make([]Decision, 0, len(events))
	for i, ev := range events {
		out = append(out, e.apply(i+1, ev))
	}
	return out
}

// RosterAt replays events up to and including time t and returns
// zone -> sorted person list, plus occupancy counts.
func RosterAt(cfg *Config, events []Event, t time.Time) (map[string][]string, map[string]int) {
	e := NewEngine(cfg)
	for i, ev := range events {
		if ev.Time.After(t) {
			break
		}
		e.apply(i+1, ev)
	}
	roster := map[string][]string{}
	for _, z := range cfg.Zones {
		roster[z.ID] = []string{}
	}
	for person, zone := range e.location {
		roster[zone] = append(roster[zone], person)
	}
	for z := range roster {
		sort.Strings(roster[z])
	}
	return roster, e.snapshot()
}
