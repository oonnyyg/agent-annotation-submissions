package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Event is one JSONL input record.
type Event struct {
	Op       string `json:"op"` // begin | commit | rollback | lock | status
	Tx       int    `json:"tx,omitempty"`
	Resource string `json:"resource,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

// Result is one JSONL output record, emitted per input event.
type Result struct {
	Event    int              `json:"event"`
	Op       string           `json:"op"`
	OK       bool             `json:"ok"`
	Tx       int              `json:"tx,omitempty"`
	Grant    *Grant           `json:"grant,omitempty"`
	Waiting  bool             `json:"waiting,omitempty"`
	Granted  []Grant          `json:"granted,omitempty"`
	Deadlock []DeadlockReport `json:"deadlock,omitempty"`
	Snapshot *Snapshot        `json:"snapshot,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// Replay processes JSONL events from r and writes one JSON result per event
// to w. Replaying the same input always yields the same output.
func Replay(r io.Reader, w io.Writer) error {
	m := NewLockManager()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	enc := json.NewEncoder(w)
	eventNo := 0
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		eventNo++
		var ev Event
		res := Result{Event: eventNo}
		if err := json.Unmarshal(line, &ev); err != nil {
			res.Op = "invalid"
			res.Error = fmt.Sprintf("invalid JSON: %v", err)
			if err := enc.Encode(res); err != nil {
				return err
			}
			continue
		}
		res.Op = ev.Op
		process(m, &ev, &res)
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return sc.Err()
}

func process(m *LockManager, ev *Event, res *Result) {
	switch ev.Op {
	case "begin":
		res.Tx = m.Begin()
		res.OK = true
	case "commit", "rollback":
		res.Tx = ev.Tx
		granted, err := m.Finish(ev.Tx)
		if err != nil {
			res.Error = err.Error()
			return
		}
		res.OK = true
		res.Granted = granted
	case "lock":
		res.Tx = ev.Tx
		g, err := m.RequestLock(ev.Tx, ev.Resource, LockMode(ev.Mode))
		if err != nil {
			res.Error = err.Error()
			return
		}
		res.OK = true
		if g != nil {
			res.Grant = g
		} else {
			res.Waiting = true
		}
	case "status":
		res.OK = true
		snap := m.Snapshot()
		res.Snapshot = &snap
	default:
		res.Error = fmt.Sprintf("unknown op %q", ev.Op)
		return
	}
	// After every state change, detect and resolve deadlocks.
	if ev.Op != "status" && res.OK {
		if reports := m.ResolveDeadlocks(); len(reports) > 0 {
			res.Deadlock = reports
		}
	}
}
