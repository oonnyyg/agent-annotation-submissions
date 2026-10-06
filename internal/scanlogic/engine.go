package scanlogic

import "fmt"

// TimerState is the runtime state of a TON timer.
type TimerState struct {
	Acc  int64 `json:"acc"`
	Done bool  `json:"done"`
}

// CounterState is the runtime state of a CTU counter.
type CounterState struct {
	Count int64 `json:"count"`
	Done  bool  `json:"done"`
	// prevCond is the edge memory, updated every scan even during reset.
	prevCond bool
}

// State is the full mutable machine state. All booleans start false,
// all accumulators and counts start at zero.
type State struct {
	Coils    map[string]bool          `json:"coils"`
	Timers   map[string]*TimerState   `json:"timers"`
	Counters map[string]*CounterState `json:"counters"`
}

// NewState builds the initial (all-false, all-zero) state for a program.
func NewState(p *Program) *State {
	s := &State{
		Coils:    map[string]bool{},
		Timers:   map[string]*TimerState{},
		Counters: map[string]*CounterState{},
	}
	for _, c := range p.Coils {
		s.Coils[c] = false
	}
	for _, t := range p.Timers {
		s.Timers[t.Name] = &TimerState{}
	}
	for _, c := range p.Counters {
		s.Counters[c.Name] = &CounterState{}
	}
	return s
}

// StmtDiff describes the effect of one statement execution.
type StmtDiff struct {
	Index  int    `json:"index"`
	Target string `json:"target"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Changed reports whether the statement actually modified its target.
func (d StmtDiff) Changed() bool {
	return fmt.Sprint(d.Before) != fmt.Sprint(d.After)
}

// ScanResult is one JSONL output line.
type ScanResult struct {
	Scan       int             `json:"scan"`
	DurationMs int64           `json:"duration_ms"`
	Inputs     map[string]bool `json:"inputs"`
	Diffs      []StmtDiff      `json:"diffs"`
	State      *State          `json:"state"`
}

// Engine replays a program scan by scan. It never reads a real clock.
type Engine struct {
	prog    *Program
	state   *State
	inputs  map[string]bool // frozen snapshot for the current scan
	presets map[string]int64
}

// NewEngine creates an engine with the initial state.
func NewEngine(p *Program) *Engine {
	presets := map[string]int64{}
	for _, t := range p.Timers {
		presets[t.Name] = t.Preset
	}
	for _, c := range p.Counters {
		presets[c.Name] = c.Preset
	}
	return &Engine{prog: p, state: NewState(p), presets: presets}
}

// State returns the current machine state.
func (e *Engine) State() *State { return e.state }

// RunScan executes one scan: freeze the input snapshot, then run
// statements strictly in order so earlier writes are visible to later
// statements while later writes are not yet visible to earlier ones.
func (e *Engine) RunScan(scanIdx int, in ScanInput) ScanResult {
	e.inputs = make(map[string]bool, len(in.Inputs))
	for k, v := range in.Inputs {
		e.inputs[k] = v
	}
	res := ScanResult{
		Scan:       scanIdx,
		DurationMs: in.DurationMs,
		Inputs:     in.Inputs,
	}
	for i, st := range e.prog.Statements {
		res.Diffs = append(res.Diffs, e.exec(i, st, in.DurationMs))
	}
	res.State = e.state
	return res
}

func (e *Engine) exec(idx int, st Statement, dur int64) StmtDiff {
	switch {
	case st.Coil != nil:
		before := e.state.Coils[st.Coil.Target]
		after := e.eval(st.Coil.Cond)
		e.state.Coils[st.Coil.Target] = after
		return StmtDiff{Index: idx, Target: st.Coil.Target, Before: before, After: after}
	case st.TON != nil:
		ts := e.state.Timers[st.TON.Timer]
		before := *ts
		if e.eval(st.TON.Cond) {
			ts.Acc += dur
			if preset := e.presets[st.TON.Timer]; ts.Acc >= preset {
				ts.Acc = preset // cap accumulated value at preset
				ts.Done = true
			}
		} else {
			ts.Acc = 0
			ts.Done = false
		}
		return StmtDiff{Index: idx, Target: st.TON.Timer, Before: before, After: *ts}
	default:
		cs := e.state.Counters[st.CTU.Counter]
		before := *cs
		cond := e.eval(st.CTU.Cond)
		if e.eval(st.CTU.Reset) {
			// Reset has priority over counting.
			cs.Count = 0
			cs.Done = false
		} else if cond && !cs.prevCond {
			if cs.Count < e.presets[st.CTU.Counter] {
				cs.Count++
			}
			cs.Done = cs.Count >= e.presets[st.CTU.Counter]
		}
		// Edge memory updates every scan, even while reset is held,
		// so releasing reset with the condition still true cannot
		// produce a spurious extra count.
		cs.prevCond = cond
		after := *cs
		after.prevCond = false // do not leak internal memory into JSON output
		return StmtDiff{Index: idx, Target: st.CTU.Counter, Before: CounterState{Count: before.Count, Done: before.Done}, After: after}
	}
}

// eval reads the frozen input snapshot and the current internal state.
func (e *Engine) eval(c Condition) bool {
	switch {
	case c.Contact != "":
		return e.read(c.Contact)
	case c.Not != nil:
		return !e.eval(*c.Not)
	case len(c.And) > 0:
		for _, sub := range c.And {
			if !e.eval(sub) {
				return false
			}
		}
		return true
	default:
		for _, sub := range c.Or {
			if e.eval(sub) {
				return true
			}
		}
		return false
	}
}

// read resolves a contact: external input, coil, or a function block's
// done flag (timers and counters expose their completion flag).
func (e *Engine) read(name string) bool {
	if v, ok := e.inputs[name]; ok {
		return v
	}
	if v, ok := e.state.Coils[name]; ok {
		return v
	}
	if t, ok := e.state.Timers[name]; ok {
		return t.Done
	}
	if c, ok := e.state.Counters[name]; ok {
		return c.Done
	}
	return false
}
