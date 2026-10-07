package main

import (
	"fmt"
	"sort"
)

// Run executes the replay over the given arrival events. Events are sorted
// by arrival time; equal times keep input order. After the input is
// exhausted, admitted queues are drained until empty.
func (s *Simulator) Run(events []Arrival) error {
	if err := validateEvents(events); err != nil {
		return err
	}
	sorted := make([]Arrival, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Time < sorted[j].Time
	})

	i := 0
	// admitDue admits every arrival due at the current time. It runs before
	// any scheduling decision and right after each transmission completes,
	// so same-instant arrivals join queues before the visit continues.
	admitDue := func() {
		for i < len(sorted) && sorted[i].Time <= s.now {
			s.admit(sorted[i])
			i++
		}
	}
	for {
		admitDue()
		if s.allIdle() {
			if i == len(sorted) {
				return nil // input exhausted and all queues drained
			}
			// Link idle: jump to the next arrival and restart the cycle
			// from the lexicographically smallest flow ID.
			s.now = sorted[i].Time
			s.next = 0
			continue
		}
		s.serveRound(admitDue)
	}
}

func validateEvents(events []Arrival) error {
	seen := make(map[string]bool)
	for _, e := range events {
		if e.Time < 0 {
			return fmt.Errorf("packet %q: negative arrival time %v", e.Packet, e.Time)
		}
		if e.Length <= 0 {
			return fmt.Errorf("packet %q: length must be positive, got %d", e.Packet, e.Length)
		}
		if seen[e.Packet] {
			return fmt.Errorf("duplicate packet ID %q", e.Packet)
		}
		seen[e.Packet] = true
	}
	return nil
}

// admit enqueues one arrival or drops it when the flow's waiting-byte limit
// would be exceeded. Only the new packet is dropped; the queue is untouched.
func (s *Simulator) admit(e Arrival) {
	f, ok := s.flows[e.Flow]
	st := s.stats[e.Flow]
	if st == nil {
		st = &FlowStat{Flow: e.Flow}
		s.stats[e.Flow] = st
	}
	st.Arrived++
	st.ArrivedByte += e.Length
	if !ok {
		s.drops = append(s.drops, DropRecord{
			Time: e.Time, Packet: e.Packet, Flow: e.Flow, Length: e.Length,
			Reason: "unknown flow",
		})
		st.Dropped++
		st.DropBytes += e.Length
		return
	}
	if f.waitingByte+e.Length > f.cfg.WaitLimit {
		s.drops = append(s.drops, DropRecord{
			Time: e.Time, Packet: e.Packet, Flow: e.Flow, Length: e.Length,
			Reason: fmt.Sprintf("waiting byte limit %d exceeded", f.cfg.WaitLimit),
		})
		st.Dropped++
		st.DropBytes += e.Length
		return
	}
	f.queue = append(f.queue, Packet{ID: e.Packet, Flow: e.Flow, Length: e.Length, Arrival: e.Time})
	f.waitingByte += e.Length
	st.Admitted++
	st.AdmitBytes += e.Length
}

func (s *Simulator) allIdle() bool {
	for _, id := range s.order {
		if len(s.flows[id].queue) > 0 {
			return false
		}
	}
	return true
}

// serveRound performs visits until at least one transmission starts or all
// flows are found idle. A visit to a non-empty queue adds the quantum once,
// then sends every head packet covered by the deficit. If the head is too
// large, the deficit is kept and the visit moves on; a packet in service is
// never preempted and arrivals during it are admitted at their own times.
func (s *Simulator) serveRound(admitDue func()) {
	for visited := 0; visited < len(s.order); visited++ {
		id := s.order[s.next]
		s.next = (s.next + 1) % len(s.order)
		f := s.flows[id]
		if len(f.queue) == 0 {
			f.deficit = 0 // idle flows never accumulate credit
			continue
		}
		f.deficit += f.cfg.Quantum
		for len(f.queue) > 0 && f.queue[0].Length <= f.deficit {
			s.transmit(f)
			admitDue() // same-instant arrivals join before the visit continues
		}
		if len(f.queue) == 0 {
			f.deficit = 0 // queue drained: visit ends, deficit resets
		}
		// Head too large: keep the remaining deficit for the next round.
		return
	}
}

// transmit sends the head packet non-preemptively. Arrivals occurring during
// the transmission, including exactly at its completion time, are admitted
// before the visit continues; no extra quantum is added for them.
func (s *Simulator) transmit(f *flowState) {
	p := f.queue[0]
	start := s.now
	end := start + float64(p.Length)/s.Rate
	f.deficit -= p.Length
	f.queue = f.queue[1:]
	f.waitingByte -= p.Length // in-service bytes no longer count as waiting
	s.now = end
	s.sends = append(s.sends, SendRecord{
		Packet: p.ID, Flow: p.Flow, Length: p.Length, Start: start, End: end,
	})
	st := s.stats[p.Flow]
	st.Sent++
	st.SentBytes += p.Length
	wait := start - p.Arrival
	st.TotalWait += wait
	if wait > st.MaxWait {
		st.MaxWait = wait
	}
}

// Results returns the send timeline, drop log, and per-flow statistics in
// lexicographic flow order.
func (s *Simulator) Results() ([]SendRecord, []DropRecord, []FlowStat) {
	stats := make([]FlowStat, 0, len(s.order))
	for _, id := range s.order {
		stats = append(stats, *s.stats[id])
	}
	extra := make([]string, 0)
	for id := range s.stats {
		known := false
		for _, k := range s.order {
			if k == id {
				known = true
				break
			}
		}
		if !known {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		stats = append(stats, *s.stats[id])
	}
	return s.sends, s.drops, stats
}
