package workshop

import (
	"container/heap"
	"sort"
)

// OpRecord captures the timing of one operation for CSV output.
// Arrive is when the job started waiting for this station (release time for
// the first op, transfer-in time otherwise). Queue = Start - Arrive,
// Process = End - Start, BlockedAfter = Leave - End (machine held after
// completion because the next station could not receive).
type OpRecord struct {
	JobID        string
	OpIndex      int
	StationID    string
	Arrive       int
	Start        int
	End          int
	Leave        int
	Queue        int
	Process      int
	BlockedAfter int
}

// JobSummary aggregates per-job statistics.
type JobSummary struct {
	ID           string `json:"id"`
	Status       string `json:"status"` // "completed" or "stalled"
	CompletedOps int    `json:"completed_ops"`
	TotalOps     int    `json:"total_ops"`
	Process      int    `json:"process_time"`
	Queue        int    `json:"queue_time"`
	Blocked      int    `json:"blocked_time"`
}

// Result is the outcome of a simulation run.
type Result struct {
	Records   []OpRecord
	Jobs      []JobSummary
	Completed bool
	Makespan  int
	Stalled   bool
	StallTime int
	// Occupancy maps station id to the job id holding its machine at stall.
	Occupancy map[string]string
	// Queues maps station id to queued job ids (FIFO order) at stall.
	Queues map[string][]string
	// External lists job ids still waiting outside for their first station.
	External []string
}

type jobState struct {
	job      *Job
	opIdx    int    // index of the op the job is waiting for / running
	location string // "unreleased", "external", "queued", "machine", "blocked", "done"
	station  string // current station when queued/machine/blocked
	arrive   int    // when the job began waiting for the current op
	start    int    // when the current op started on the machine
	enqueue  int    // when the job entered its current buffer slot
	summary  *JobSummary
}

type stationState struct {
	st      *Station
	machine *jobState   // job on the machine (processing or blocked)
	queue   []*jobState // FIFO buffer, excludes the machine
}

type event struct {
	time int
	job  *jobState
}

type eventHeap []event

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return h[i].time < h[j].time }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)        { *h = append(*h, x.(event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

// Simulate replays the workshop as a discrete-event simulation.
func Simulate(in *Input) *Result {
	stations := map[string]*stationState{}
	for i := range in.Stations {
		s := &in.Stations[i]
		stations[s.ID] = &stationState{st: s}
	}
	jobs := map[string]*jobState{}
	var events eventHeap
	for i := range in.Jobs {
		j := &in.Jobs[i]
		sum := &JobSummary{ID: j.ID, Status: "stalled", TotalOps: len(j.Route)}
		jobs[j.ID] = &jobState{job: j, location: "unreleased", arrive: j.Release, summary: sum}
		heap.Push(&events, event{time: j.Release, job: jobs[j.ID]})
	}

	res := &Result{Occupancy: map[string]string{}, Queues: map[string][]string{}}
	jobIDs := in.SortedJobIDs()
	stationIDs := in.SortedStationIDs()
	now := 0

	for len(events) > 0 {
		now = events[0].time
		// Register every release and completion at this timestamp first.
		for len(events) > 0 && events[0].time == now {
			e := heap.Pop(&events).(event)
			js := e.job
			switch js.location {
			case "unreleased":
				js.location = "external"
			case "machine":
				sum := js.summary
				sum.CompletedOps++
				sum.Process += js.job.Route[js.opIdx].Duration
				if js.opIdx == len(js.job.Route)-1 {
					// Last op: the job leaves immediately.
					res.Records = append(res.Records, finishRecord(js, now, now))
					stations[js.station].machine = nil
					js.location = "done"
					js.station = ""
					sum.Status = "completed"
				} else {
					js.location = "blocked"
				}
			}
		}

		// Repeat transfer and start rounds until nothing changes.
		for {
			changed := false
			// Transfers, in lexicographic job order.
			for _, id := range jobIDs {
				js := jobs[id]
				switch js.location {
				case "external":
					if tryEnter(js, stations, now, &events) {
						changed = true
					}
				case "blocked":
					next := js.job.Route[js.opIdx+1].Station
					if canAccept(stations[next]) {
						end := js.start + js.job.Route[js.opIdx].Duration
						res.Records = append(res.Records, finishRecord(js, end, now))
						js.summary.Blocked += now - end
						stations[js.station].machine = nil
						js.opIdx++
						js.arrive = now
						enter(js, stations[next], now, &events)
						changed = true
					}
				}
			}
			// Starts, in lexicographic station order.
			for _, id := range stationIDs {
				st := stations[id]
				if st.machine == nil && len(st.queue) > 0 {
					js := st.queue[0]
					st.queue = st.queue[1:]
					startOnMachine(js, st, now, &events)
					changed = true
				}
			}
			if !changed {
				break
			}
		}
	}

	// Collect final state.
	for _, r := range res.Records {
		if r.Leave > res.Makespan {
			res.Makespan = r.Leave
		}
	}
	unfinished := false
	for _, id := range jobIDs {
		js := jobs[id]
		res.Jobs = append(res.Jobs, *js.summary)
		if js.location != "done" {
			unfinished = true
		}
	}
	res.Completed = !unfinished
	if unfinished {
		res.Stalled = true
		res.StallTime = now
		// Settle records for jobs stuck holding a machine after completion.
		for _, id := range jobIDs {
			js := jobs[id]
			if js.location == "blocked" {
				end := js.start + js.job.Route[js.opIdx].Duration
				res.Records = append(res.Records, finishRecord(js, end, now))
				js.summary.Blocked += now - end
			}
		}
		if now > res.Makespan {
			res.Makespan = now
		}
		for _, id := range stationIDs {
			st := stations[id]
			if st.machine != nil {
				res.Occupancy[id] = st.machine.job.ID
			}
			var q []string
			for _, js := range st.queue {
				q = append(q, js.job.ID)
			}
			if len(q) > 0 {
				res.Queues[id] = q
			}
		}
		for _, id := range jobIDs {
			if jobs[id].location == "external" {
				res.External = append(res.External, id)
			}
		}
	}
	return res
}

// canAccept reports whether a station can receive a job right now: either a
// buffer slot is free, or the machine is idle for a direct handoff.
func canAccept(st *stationState) bool {
	return len(st.queue) < st.st.Buffer || st.machine == nil
}

// tryEnter moves an externally waiting job into its first station if possible.
func tryEnter(js *jobState, stations map[string]*stationState, now int, events *eventHeap) bool {
	st := stations[js.job.Route[0].Station]
	if !canAccept(st) {
		return false
	}
	js.arrive = js.job.Release
	enter(js, st, now, events)
	return true
}

// enter places a job into a station: directly onto an idle machine when the
// queue is empty (the only option for zero-capacity buffers), otherwise FIFO.
func enter(js *jobState, st *stationState, now int, events *eventHeap) {
	js.station = st.st.ID
	if st.machine == nil && len(st.queue) == 0 {
		startOnMachine(js, st, now, events)
		return
	}
	js.location = "queued"
	js.enqueue = now
	// Keep the queue ordered by enqueue time, then job id.
	i := sort.Search(len(st.queue), func(i int) bool {
		q := st.queue[i]
		if q.enqueue != js.enqueue {
			return q.enqueue > js.enqueue
		}
		return q.job.ID > js.job.ID
	})
	st.queue = append(st.queue, nil)
	copy(st.queue[i+1:], st.queue[i:])
	st.queue[i] = js
}

// startOnMachine begins processing a job on an idle machine.
func startOnMachine(js *jobState, st *stationState, now int, events *eventHeap) {
	js.location = "machine"
	js.station = st.st.ID
	js.start = now
	js.summary.Queue += now - js.arrive
	st.machine = js
	heap.Push(events, event{time: now + js.job.Route[js.opIdx].Duration, job: js})
}

// finishRecord builds the CSV record for a completed op.
func finishRecord(js *jobState, end, leave int) OpRecord {
	op := js.job.Route[js.opIdx]
	return OpRecord{
		JobID:        js.job.ID,
		OpIndex:      js.opIdx,
		StationID:    op.Station,
		Arrive:       js.arrive,
		Start:        js.start,
		End:          end,
		Leave:        leave,
		Queue:        js.start - js.arrive,
		Process:      end - js.start,
		BlockedAfter: leave - end,
	}
}
