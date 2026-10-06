package sim

import "sort"

// Run executes the simulation to completion or stall.
func (s *Simulator) Run() *Summary {
	for {
		s.advance()
		s.settle()
		if s.allDone() {
			return s.summary(nil)
		}
		if !s.hasFutureEvent() {
			return s.summary(s.buildStall())
		}
	}
}

// advance moves time to the next event and registers every release
// and completion happening at that instant.
func (s *Simulator) advance() {
	next := -1
	if len(s.pending) > 0 {
		next = s.pending[0].job.Release
	}
	for _, st := range s.stations {
		if st.machine != nil && !st.blocked {
			if next < 0 || st.machine.end < next {
				next = st.machine.end
			}
		}
	}
	if next < 0 {
		return // no events; stall handled by caller
	}
	s.time = next

	// Register releases at this instant.
	for len(s.pending) > 0 && s.pending[0].job.Release == s.time {
		js := s.pending[0]
		js.enqueue = s.time // external wait counts as queue time
		s.pending = s.pending[1:]
		s.external = append(s.external, js)
	}
	// Register completions at this instant.
	for _, id := range sortedKeys(s.stations) {
		st := s.stations[id]
		if st.machine == nil || st.blocked || st.machine.end != s.time {
			continue
		}
		js := st.machine
		s.recordOp(js, s.time)
		if js.opIndex == len(js.job.Route)-1 {
			// Last operation: the job leaves the system immediately.
			js.finished = true
			st.machine = nil
			s.finishJob(js, s.time)
		} else {
			st.blocked = true
		}
	}
}

// settle repeatedly applies transfers (by job id) then machine starts
// (by station id) until nothing changes.
func (s *Simulator) settle() {
	for {
		changed := s.transfers()
		if s.starts() {
			changed = true
		}
		if !changed {
			return
		}
	}
}

// transfers performs one round of feasible moves, ordered by job id.
func (s *Simulator) transfers() bool {
	type move struct {
		js   *jobState
		from *stationState // nil for external jobs
	}
	var moves []move
	for _, js := range s.external {
		moves = append(moves, move{js: js})
	}
	for _, id := range sortedKeys(s.stations) {
		st := s.stations[id]
		if st.machine != nil && st.blocked {
			moves = append(moves, move{js: st.machine, from: st})
		}
	}
	sort.Slice(moves, func(a, b int) bool { return moves[a].js.job.ID < moves[b].js.job.ID })

	changed := false
	for _, m := range moves {
		js := m.js
		var dest *stationState
		if m.from == nil {
			if !containsJob(s.external, js) {
				continue // already moved this round
			}
			dest = s.stations[js.job.Route[0].Station]
		} else {
			if m.from.machine != js || !m.from.blocked {
				continue
			}
			dest = s.stations[js.job.Route[js.opIndex+1].Station]
		}
		if !dest.canReceive() {
			continue // failed transfer never drops the part
		}
		if m.from == nil {
			s.external = removeJob(s.external, js)
		} else {
			s.recordBlocked(js, s.time)
			m.from.machine = nil
			js.enqueue = s.time
			m.from.blocked = false
			js.opIndex++
		}
		dest.receive(js, s.time)
		changed = true
	}
	return changed
}

// starts begins processing on every idle machine with a queued part,
// ordered by station id.
func (s *Simulator) starts() bool {
	changed := false
	for _, id := range sortedKeys(s.stations) {
		st := s.stations[id]
		if st.machine != nil || len(st.queue) == 0 {
			continue
		}
		st.machine = st.queue[0]
		st.queue = st.queue[1:]
		st.startProcessing(s.time)
		changed = true
	}
	return changed
}

func (s *Simulator) hasFutureEvent() bool {
	if len(s.pending) > 0 {
		return true
	}
	for _, st := range s.stations {
		if st.machine != nil && !st.blocked {
			return true
		}
	}
	return false
}

func (s *Simulator) allDone() bool {
	for _, js := range s.jobs {
		if !js.finished {
			return false
		}
	}
	return true
}

func containsJob(list []*jobState, js *jobState) bool {
	for _, x := range list {
		if x == js {
			return true
		}
	}
	return false
}

func removeJob(list []*jobState, js *jobState) []*jobState {
	for i, x := range list {
		if x == js {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}
