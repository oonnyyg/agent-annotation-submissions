package main

import (
	"fmt"
	"sort"
	"time"
)

// Decision records the outcome of processing one event record.
type Decision struct {
	Seq      int
	Time     time.Time
	Kind     string // "swipe" or "correction"
	OK       bool
	Reason   string
	Person   string
	FromZone string
	ToZone   string
}

// Engine replays events against a validated config.
type Engine struct {
	cfg     *Config
	zones   map[string]*Zone
	readers map[string]*Reader
	creds   map[string]*Credential
	loc     map[string]string // person -> zone
	occ     map[string]int    // zone -> headcount
	Results []Decision
}

func NewEngine(cfg *Config) *Engine {
	e := &Engine{
		cfg:     cfg,
		zones:   map[string]*Zone{},
		readers: map[string]*Reader{},
		creds:   map[string]*Credential{},
		loc:     map[string]string{},
		occ:     map[string]int{},
	}
	for i := range cfg.Zones {
		z := &cfg.Zones[i]
		e.zones[z.ID] = z
	}
	for i := range cfg.Readers {
		r := &cfg.Readers[i]
		e.readers[r.ID] = r
	}
	for i := range cfg.Credentials {
		c := &cfg.Credentials[i]
		e.creds[c.ID] = c
	}
	for _, p := range cfg.Persons {
		e.loc[p.ID] = p.Initial
		e.occ[p.Initial]++
	}
	return e
}

// Replay processes records in time order, stable for equal timestamps.
func (e *Engine) Replay(recs []EventRecord) {
	sort.SliceStable(recs, func(i, j int) bool {
		return recs[i].time().Before(recs[j].time())
	})
	for i := range recs {
		recs[i].order = i
		switch {
		case recs[i].Swipe != nil:
			e.Results = append(e.Results, e.applySwipe(i, recs[i].Swipe))
		case recs[i].Correction != nil:
			e.Results = append(e.Results, e.applyCorrection(i, recs[i].Correction))
		}
	}
}

func (r EventRecord) time() time.Time {
	if r.Swipe != nil {
		return r.Swipe.Time
	}
	return r.Correction.Time
}

func (e *Engine) applySwipe(seq int, ev *Event) Decision {
	d := Decision{Seq: seq, Time: ev.Time, Kind: "swipe"}
	cr, ok := e.creds[ev.Credential]
	if !ok {
		d.Reason = fmt.Sprintf("unknown credential %q", ev.Credential)
		return d
	}
	d.Person = cr.Person
	r, ok := e.readers[ev.Reader]
	if !ok {
		d.Reason = fmt.Sprintf("unknown reader %q", ev.Reader)
		return d
	}
	d.FromZone, d.ToZone = r.From, r.To
	if !cr.Active {
		d.Reason = "credential disabled"
		return d
	}
	if ev.Time.Before(cr.From) || !ev.Time.Before(cr.To) {
		d.Reason = fmt.Sprintf("credential not valid at %s (valid [%s,%s))", ev.Time, cr.From, cr.To)
		return d
	}
	dst := e.zones[r.To]
	if !dst.Outdoor && !contains(cr.Zones, r.To) {
		d.Reason = fmt.Sprintf("credential lacks permission for zone %q", r.To)
		return d
	}
	cur := e.loc[cr.Person]
	if cur != r.From {
		d.Reason = fmt.Sprintf("person %q is in %q, not at reader origin %q", cr.Person, cur, r.From)
		return d
	}
	if !dst.Outdoor && e.occ[r.To] >= dst.Capacity {
		d.Reason = fmt.Sprintf("zone %q is full (%d/%d)", r.To, e.occ[r.To], dst.Capacity)
		return d
	}
	e.loc[cr.Person] = r.To
	e.occ[r.From]--
	e.occ[r.To]++
	d.OK = true
	d.Reason = "granted"
	return d
}

func (e *Engine) applyCorrection(seq int, c *Correction) Decision {
	d := Decision{Seq: seq, Time: c.Time, Kind: "correction", Person: c.Person, ToZone: c.To}
	if c.Reason == "" {
		d.Reason = "correction requires a reason"
		return d
	}
	cur, ok := e.loc[c.Person]
	if !ok {
		d.Reason = fmt.Sprintf("unknown person %q", c.Person)
		return d
	}
	d.FromZone = cur
	dst, ok := e.zones[c.To]
	if !ok {
		d.Reason = fmt.Sprintf("unknown zone %q", c.To)
		return d
	}
	if cur == c.To {
		d.Reason = fmt.Sprintf("person already in %q", c.To)
		return d
	}
	if !dst.Outdoor && e.occ[c.To] >= dst.Capacity {
		d.Reason = fmt.Sprintf("zone %q is full (%d/%d)", c.To, e.occ[c.To], dst.Capacity)
		return d
	}
	e.loc[c.Person] = c.To
	e.occ[cur]--
	e.occ[c.To]++
	d.OK = true
	d.Reason = "corrected: " + c.Reason
	return d
}

// Roster returns sorted zone -> sorted person list snapshot.
func (e *Engine) Roster() map[string][]string {
	out := map[string][]string{}
	for id := range e.zones {
		out[id] = []string{}
	}
	for p, z := range e.loc {
		out[z] = append(out[z], p)
	}
	for z := range out {
		sort.Strings(out[z])
	}
	return out
}

// Occupancy returns zone -> headcount.
func (e *Engine) Occupancy() map[string]int {
	out := map[string]int{}
	for id := range e.zones {
		out[id] = e.occ[id]
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
