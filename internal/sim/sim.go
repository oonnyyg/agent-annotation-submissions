package sim

import "sort"

type jobState struct {
	job      *Job
	opIndex  int // index of the op currently waiting / processing / blocked
	enqueue  int // time the part started waiting for the current station
	start    int // processing start of the current op
	end      int // processing end of the current op
	finished bool
	recIdx   int
}

type stationState struct {
	station *Station
	queue   []*jobState // buffer, ordered by enqueue time then job id
	machine *jobState   // part on the machine, nil if free
	blocked bool        // machine part finished but cannot leave
}

// Simulator replays the workshop as a discrete-event simulation.
type Simulator struct {
	in       *Input
	stations map[string]*stationState
	jobs     []*jobState
	external []*jobState // released, not yet accepted at first station
	pending  []*jobState // not yet released, sorted by release then id
	time     int
	records  []OpRecord
	stats    map[string]*JobStats
}

// New builds a Simulator from a validated input.
func New(in *Input) *Simulator {
	s := &Simulator{
		in:       in,
		stations: map[string]*stationState{},
		stats:    map[string]*JobStats{},
	}
	for i := range in.Stations {
		st := &in.Stations[i]
		s.stations[st.ID] = &stationState{station: st}
	}
	for i := range in.Jobs {
		j := &in.Jobs[i]
		s.jobs = append(s.jobs, &jobState{job: j})
		s.stats[j.ID] = &JobStats{JobID: j.ID}
	}
	s.pending = append([]*jobState(nil), s.jobs...)
	sort.Slice(s.pending, func(a, b int) bool {
		if s.pending[a].job.Release != s.pending[b].job.Release {
			return s.pending[a].job.Release < s.pending[b].job.Release
		}
		return s.pending[a].job.ID < s.pending[b].job.ID
	})
	return s
}

// canReceive reports whether the station can accept a part now.
func (st *stationState) canReceive() bool {
	if len(st.queue) < st.station.Buffer {
		return true
	}
	return st.station.Buffer == 0 && st.machine == nil
}

// receive places a part into the buffer, or directly onto an idle
// machine when the buffer capacity is zero.
func (st *stationState) receive(js *jobState, t int) {
	if st.station.Buffer == 0 && st.machine == nil {
		st.machine = js
		st.startProcessing(t)
		return
	}
	st.queue = append(st.queue, js)
}

// startProcessing begins the op of the part on the machine at time t.
func (st *stationState) startProcessing(t int) {
	js := st.machine
	js.start = t
	op := js.job.Route[js.opIndex]
	js.end = t + op.Duration
	st.blocked = false
}
