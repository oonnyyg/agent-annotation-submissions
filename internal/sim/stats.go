package sim

// recordOp books a finished operation: queue and processing time are
// final, Leave is updated later if the part blocks the machine.
func (s *Simulator) recordOp(js *jobState, t int) {
	op := js.job.Route[js.opIndex]
	st := s.stats[js.job.ID]
	st.Queue += js.start - js.enqueue
	st.Processing += op.Duration
	s.records = append(s.records, OpRecord{
		JobID:     js.job.ID,
		OpIndex:   js.opIndex,
		StationID: op.Station,
		Enqueue:   js.enqueue,
		Start:     js.start,
		End:       js.end,
		Leave:     t,
	})
	js.recIdx = len(s.records) - 1
}

// recordBlocked finalizes the blocked interval when the part finally
// leaves the machine at time t.
func (s *Simulator) recordBlocked(js *jobState, t int) {
	s.records[js.recIdx].Leave = t
	s.stats[js.job.ID].Blocked += t - js.end
}

func (s *Simulator) finishJob(js *jobState, t int) {
	st := s.stats[js.job.ID]
	st.Finished = true
	st.Completion = t
}

func (s *Simulator) buildStall() *StallInfo {
	info := &StallInfo{Time: s.time}
	for _, id := range sortedKeys(s.stations) {
		st := s.stations[id]
		if st.machine != nil && st.blocked {
			js := st.machine
			occ := Occupancy{
				StationID: id,
				JobID:     js.job.ID,
				Next:      js.job.Route[js.opIndex+1].Station,
			}
			for _, q := range st.queue {
				occ.Queued = append(occ.Queued, q.job.ID)
			}
			info.Occupancy = append(info.Occupancy, occ)
		}
	}
	for _, js := range s.external {
		info.Waiting = append(info.Waiting, js.job.ID)
	}
	return info
}

func (s *Simulator) summary(stall *StallInfo) *Summary {
	sum := &Summary{Makespan: s.time, Stall: stall}
	if stall == nil {
		sum.Status = "completed"
	} else {
		sum.Status = "stalled"
	}
	for _, js := range s.jobs {
		sum.Jobs = append(sum.Jobs, *s.stats[js.job.ID])
	}
	return sum
}

// Records returns the executed operations in completion order.
func (s *Simulator) Records() []OpRecord {
	return s.records
}
