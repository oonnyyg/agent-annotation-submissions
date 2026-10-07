package main

import (
	"fmt"
	"sort"
)

// Sim replays one collective-control elevator offline.
type Sim struct {
	cfg     Config
	time    int
	floor   int
	dir     int
	onboard []*Passenger
	waiting map[int][]*Passenger // per floor, in registration order
	all     []*Passenger         // sorted by (Arrive, Index)
	next    int                  // index into all of the next unregistered arrival
	logs    []string
	maxLoad int
}

func NewSim(in *Input) *Sim {
	s := &Sim{
		cfg:     in.Config,
		floor:   in.StartFloor,
		dir:     dirIdle,
		waiting: map[int][]*Passenger{},
	}
	for i, pi := range in.Passengers {
		s.all = append(s.all, &Passenger{PassengerInput: pi, Index: i, Board: -1, Alight: -1})
	}
	sort.SliceStable(s.all, func(i, j int) bool {
		if s.all[i].Arrive != s.all[j].Arrive {
			return s.all[i].Arrive < s.all[j].Arrive
		}
		return s.all[i].Index < s.all[j].Index
	})
	return s
}

func (s *Sim) logf(format string, args ...any) {
	s.logs = append(s.logs, fmt.Sprintf("t=%d ", s.time)+fmt.Sprintf(format, args...))
}

// register moves all arrivals with Arrive <= t into the waiting queues.
// Same-time arrivals are registered before any door processing at that time.
func (s *Sim) register(t int) {
	for s.next < len(s.all) && s.all[s.next].Arrive <= t {
		p := s.all[s.next]
		s.waiting[p.From] = append(s.waiting[p.From], p)
		s.logs = append(s.logs, fmt.Sprintf("t=%d ARRIVE id=%s from=%d to=%d dir=%s",
			p.Arrive, p.ID, p.From, p.To, dirName(p.dir())))
		s.next++
	}
}

func (s *Sim) anyWaiting() bool {
	for _, q := range s.waiting {
		if len(q) > 0 {
			return true
		}
	}
	return false
}

// Run drives the simulation until every passenger has alighted.
func (s *Sim) Run() {
	for {
		s.register(s.time)
		if s.dir == dirIdle {
			if !s.anyWaiting() {
				if s.next < len(s.all) {
					s.time = s.all[s.next].Arrive
					continue
				}
				return
			}
			s.dispatchIdle()
			continue
		}
		s.stepMoving()
	}
}

// dispatchIdle sends the idle car empty to the earliest waiting passenger's
// origin, adopts that passenger's direction and opens the doors there.
func (s *Sim) dispatchIdle() {
	var first *Passenger
	for _, q := range s.waiting {
		for _, p := range q {
			if first == nil || p.Arrive < first.Arrive ||
				(p.Arrive == first.Arrive && p.Index < first.Index) {
				first = p
			}
		}
	}
	if first.From != s.floor {
		s.logf("REPOSITION from=%d to=%d (empty, pickup id=%s)", s.floor, first.From, first.ID)
		s.time += abs(first.From-s.floor) * s.cfg.TravelTime
		s.floor = first.From
		s.register(s.time)
	}
	s.dir = first.dir()
	s.logf("DISPATCH floor=%d dir=%s id=%s", s.floor, dirName(s.dir), first.ID)
	s.serviceFloor()
}

// stepMoving advances the car one floor in the current direction, then
// decides whether to stop and whether to keep, reverse or drop the direction.
func (s *Sim) stepMoving() {
	next := s.floor + s.dir
	if next < s.cfg.MinFloor || next > s.cfg.MaxFloor {
		// Endpoint guard: let serviceFloor reverse or go idle.
		s.serviceFloor()
		return
	}
	s.time += s.cfg.TravelTime
	s.floor = next
	s.register(s.time)
	s.serviceFloor()
}

// waitingAtFloorDir reports whether anyone waits at floor f heading d.
func (s *Sim) waitingAtFloorDir(f, d int) bool {
	for _, p := range s.waiting[f] {
		if p.dir() == d {
			return true
		}
	}
	return false
}

// originAhead reports whether any waiting origin lies strictly beyond the
// current floor in direction d.
func (s *Sim) originAhead(d int) bool {
	for f, q := range s.waiting {
		if len(q) == 0 {
			continue
		}
		if (d == dirUp && f > s.floor) || (d == dirDown && f < s.floor) {
			return true
		}
	}
	return false
}

// destAhead reports whether any onboard destination lies strictly beyond the
// current floor in direction d.
func (s *Sim) destAhead(d int) bool {
	for _, p := range s.onboard {
		if (d == dirUp && p.To > s.floor) || (d == dirDown && p.To < s.floor) {
			return true
		}
	}
	return false
}

// serviceFloor handles the car at the current floor: choose the direction
// for departure, alight, then board same-direction waiters in queue order.
// Boarding happens only at the door-open instant.
func (s *Sim) serviceFloor() {
	d := s.dir
	if d == dirIdle {
		return
	}
	keep := s.destAhead(d) || s.originAhead(d) || s.waitingAtFloorDir(s.floor, d)
	if !keep {
		opp := -d
		if s.waitingAtFloorDir(s.floor, opp) || s.originAhead(opp) {
			s.logf("REVERSE floor=%d dir=%s", s.floor, dirName(opp))
			d = opp
		} else {
			d = dirIdle
		}
	}

	var alight []*Passenger
	var rest []*Passenger
	for _, p := range s.onboard {
		if p.To == s.floor {
			alight = append(alight, p)
		} else {
			rest = append(rest, p)
		}
	}
	canBoard := d != dirIdle && s.waitingAtFloorDir(s.floor, d) &&
		len(rest) < s.cfg.Capacity

	if len(alight) == 0 && !canBoard {
		s.dir = d
		return
	}

	s.logf("STOP floor=%d dir=%s load=%d", s.floor, dirName(d), len(s.onboard))
	s.onboard = rest
	for _, p := range alight {
		p.Alight = s.time
		s.logf("ALIGHT id=%s floor=%d", p.ID, s.floor)
	}
	if canBoard {
		var remain []*Passenger
		for _, p := range s.waiting[s.floor] {
			if p.dir() == d && len(s.onboard) < s.cfg.Capacity {
				p.Board = s.time
				s.onboard = append(s.onboard, p)
				s.logf("BOARD id=%s floor=%d dir=%s load=%d", p.ID, s.floor, dirName(d), len(s.onboard))
			} else {
				remain = append(remain, p)
			}
		}
		s.waiting[s.floor] = remain
	}
	if len(s.onboard) > s.maxLoad {
		s.maxLoad = len(s.onboard)
	}
	s.dir = d
	s.time += s.cfg.DwellTime
	s.logf("DEPART floor=%d dir=%s load=%d", s.floor, dirName(s.dir), len(s.onboard))
}
