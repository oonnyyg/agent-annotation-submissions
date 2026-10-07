package main

import (
	"fmt"
	"sort"
)

// FlowConfig describes one DRR flow: a positive quantum (bytes per visit)
// and a positive waiting-byte limit for packets queued but not yet sent.
type FlowConfig struct {
	ID        string
	Quantum   int64
	WaitLimit int64
}

// Arrival is one packet arrival event read from the CSV input.
type Arrival struct {
	Time   float64 // seconds
	Flow   string
	Packet string
	Length int64
	Order  int // input order, used for stable sorting at equal times
}

// Packet is an admitted packet waiting in a flow queue.
type Packet struct {
	ID      string
	Flow    string
	Length  int64
	Arrival float64
}

// SendRecord describes one finished, non-preemptive transmission.
type SendRecord struct {
	Packet string
	Flow   string
	Length int64
	Start  float64
	End    float64
}

// DropRecord describes a newly arrived packet rejected by the wait limit.
type DropRecord struct {
	Time   float64
	Packet string
	Flow   string
	Length int64
	Reason string
}

// FlowStat aggregates per-flow waiting-time and byte accounting.
type FlowStat struct {
	Flow        string
	Arrived     int
	ArrivedByte int64
	Admitted    int
	AdmitBytes  int64
	Sent        int
	SentBytes   int64
	Dropped     int
	DropBytes   int64
	TotalWait   float64
	MaxWait     float64
}

type flowState struct {
	cfg         FlowConfig
	queue       []Packet
	waitingByte int64
	deficit     int64
}

// Simulator replays byte-based deficit round robin on a single output link.
type Simulator struct {
	Rate  float64 // link rate, bytes per second, must be > 0
	flows map[string]*flowState
	order []string // flow IDs in lexicographic order
	next  int      // index into order for the next visit
	now   float64
	sends []SendRecord
	drops []DropRecord
	stats map[string]*FlowStat
}

// NewSimulator builds a simulator from flow configs. Flows are visited in
// lexicographic ID order; the link rate must be positive.
func NewSimulator(rate float64, cfgs []FlowConfig) (*Simulator, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("link rate must be positive, got %v", rate)
	}
	s := &Simulator{
		Rate:  rate,
		flows: make(map[string]*flowState),
		stats: make(map[string]*FlowStat),
	}
	seen := make(map[string]bool)
	for _, c := range cfgs {
		if c.ID == "" {
			return nil, fmt.Errorf("flow ID must not be empty")
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("duplicate flow ID %q", c.ID)
		}
		seen[c.ID] = true
		if c.Quantum <= 0 {
			return nil, fmt.Errorf("flow %q: quantum must be positive, got %d", c.ID, c.Quantum)
		}
		if c.WaitLimit <= 0 {
			return nil, fmt.Errorf("flow %q: waiting byte limit must be positive, got %d", c.ID, c.WaitLimit)
		}
		s.flows[c.ID] = &flowState{cfg: c}
		s.order = append(s.order, c.ID)
		s.stats[c.ID] = &FlowStat{Flow: c.ID}
	}
	sort.Strings(s.order)
	return s, nil
}
