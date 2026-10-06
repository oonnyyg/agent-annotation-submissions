package scanlogic

import "fmt"

// TimerState is the runtime state of a TON timer.
type TimerState struct {
	AccumulatedMS int64 `json:"accumulated_ms"`
	PresetMS      int64 `json:"preset_ms"`
	Done          bool  `json:"done"`
}

// CounterState is the runtime state of a CTU counter.
type CounterState struct {
	Value  int64 `json:"value"`
	Preset int64 `json:"preset"`
	Done   bool  `json:"done"`
}

// State is the full mutable machine state between scans.
type State struct {
	Coils    map[string]bool          `json:"coils"`
	Timers   map[string]*TimerState   `json:"timers"`
	Counters map[string]*CounterState `json:"counters"`

	inputs   map[string]bool // snapshot of the current scan
	condPrev map[string]bool // ctu edge memory, persists across scans
}

// NewState builds the initial state: all booleans false, accumulators zero.
func (p *Program) NewState() *State {
	s := &State{
		Coils:    map[string]bool{},
		Timers:   map[string]*TimerState{},
		Counters: map[string]*CounterState{},
		condPrev: map[string]bool{},
	}
	for _, c := range p.Coils {
		s.Coils[c] = false
	}
	for _, t := range p.Timers {
		s.Timers[t.Name] = &TimerState{PresetMS: t.PresetMS}
	}
	for _, c := range p.Counters {
		s.Counters[c.Name] = &CounterState{Preset: c.Preset}
	}
	return s
}

// Change describes one statement's effect during a scan.
type Change struct {
	Statement int    `json:"statement"`
	Target    string `json:"target"`
	Before    any    `json:"before"`
	After     any    `json:"after"`
}

// ScanResult is one output line of the replay.
type ScanResult struct {
	Scan       int             `json:"scan"`
	DurationMS int64           `json:"duration_ms"`
	Inputs     map[string]bool `json:"inputs"`
	Changes    []Change        `json:"changes"`
	State      *State          `json:"state"`
}

// validateRecord checks a scan record against the program: duration must be
// a positive integer of milliseconds and inputs must be complete and exact.
func (p *Program) validateRecord(rec ScanRecord) error {
	if rec.DurationMS <= 0 {
		return fmt.Errorf("scan %d: duration_ms must be a positive integer", rec.Scan)
	}
	if rec.Inputs == nil {
		return fmt.Errorf("scan %d: inputs must be provided", rec.Scan)
	}
	for _, name := range p.Inputs {
		if _, ok := rec.Inputs[name]; !ok {
			return fmt.Errorf("scan %d: missing input %q", rec.Scan, name)
		}
	}
	for name := range rec.Inputs {
		known := false
		for _, decl := range p.Inputs {
			if decl == name {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("scan %d: undeclared input %q", rec.Scan, name)
		}
	}
	return nil
}

// Run replays the whole record sequence and returns one result per scan.
func (p *Program) Run(records []ScanRecord) ([]*ScanResult, error) {
	state := p.NewState()
	results := make([]*ScanResult, 0, len(records))
	for i, rec := range records {
		if err := p.validateRecord(rec); err != nil {
			return nil, err
		}
		res := p.runScan(state, rec, i)
		res.State = state.snapshot()
		results = append(results, res)
	}
	return results, nil
}

// snapshot deep-copies the externally visible state so earlier scan results
// are not mutated by later scans.
func (s *State) snapshot() *State {
	cp := &State{
		Coils:    map[string]bool{},
		Timers:   map[string]*TimerState{},
		Counters: map[string]*CounterState{},
	}
	for k, v := range s.Coils {
		cp.Coils[k] = v
	}
	for k, v := range s.Timers {
		t := *v
		cp.Timers[k] = &t
	}
	for k, v := range s.Counters {
		c := *v
		cp.Counters[k] = &c
	}
	return cp
}

// runScan executes one scan: fix the input snapshot, then run statements
// strictly in order. Writes are visible to later statements immediately;
// earlier statements keep the old values they already read.
func (p *Program) runScan(s *State, rec ScanRecord, index int) *ScanResult {
	s.inputs = rec.Inputs
	res := &ScanResult{
		Scan:       rec.Scan,
		DurationMS: rec.DurationMS,
		Inputs:     rec.Inputs,
	}
	for i := range p.Statements {
		st := &p.Statements[i]
		switch st.Type {
		case "coil":
			before := s.Coils[st.Target]
			after := s.eval(st.Cond)
			s.Coils[st.Target] = after
			res.Changes = append(res.Changes, Change{
				Statement: i, Target: "coil:" + st.Target,
				Before: before, After: after,
			})
		case "ton":
			ts := s.Timers[st.Timer]
			before := *ts
			if s.eval(st.Cond) {
				ts.AccumulatedMS += rec.DurationMS
				if ts.AccumulatedMS >= ts.PresetMS {
					ts.AccumulatedMS = ts.PresetMS
					ts.Done = true
				}
			} else {
				ts.AccumulatedMS = 0
				ts.Done = false
			}
			res.Changes = append(res.Changes, Change{
				Statement: i, Target: "timer:" + st.Timer,
				Before: before, After: *ts,
			})
		case "ctu":
			cs := s.Counters[st.Counter]
			before := *cs
			cond := s.eval(st.Cond)
			reset := s.eval(st.Reset)
			if reset {
				cs.Value = 0
				cs.Done = false
			} else if cond && !s.condPrev[st.Counter] {
				cs.Value++
				if cs.Value >= cs.Preset {
					cs.Value = cs.Preset
					cs.Done = true
				}
			}
			// Edge memory updates even while reset is true, so releasing
			// reset with the condition still true does not count again.
			s.condPrev[st.Counter] = cond
			res.Changes = append(res.Changes, Change{
				Statement: i, Target: "counter:" + st.Counter,
				Before: before, After: *cs,
			})
		}
	}
	return res
}

// eval evaluates a condition against the current state (input snapshot plus
// whatever internal writes have already happened this scan).
func (s *State) eval(c *Condition) bool {
	switch c.Op {
	case "contact":
		if v, ok := s.inputs[c.Name]; ok {
			return v
		}
		return s.Coils[c.Name]
	case "done":
		if t, ok := s.Timers[c.Name]; ok {
			return t.Done
		}
		return s.Counters[c.Name].Done
	case "and":
		for _, a := range c.Args {
			if !s.eval(a) {
				return false
			}
		}
		return true
	case "or":
		for _, a := range c.Args {
			if s.eval(a) {
				return true
			}
		}
		return false
	case "not":
		return !s.eval(c.Args[0])
	}
	return false
}
