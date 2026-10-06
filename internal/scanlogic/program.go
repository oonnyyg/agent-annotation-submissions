// Package scanlogic implements an offline sequential-control replay engine.
// It interprets a declarative scan program (coils, TON timers, CTU counters)
// against a recorded sequence of per-scan input snapshots, using only the
// recorded scan durations. No real devices or clocks are involved.
package scanlogic

import (
	"encoding/json"
	"fmt"
	"io"
)

// Program is a validated scan program.
type Program struct {
	Inputs     []string      `json:"inputs"`
	Coils      []string      `json:"coils"`
	Timers     []TimerDecl   `json:"timers"`
	Counters   []CounterDecl `json:"counters"`
	Statements []Statement   `json:"statements"`
}

// TimerDecl declares a non-retentive on-delay timer (TON).
type TimerDecl struct {
	Name     string `json:"name"`
	PresetMS int64  `json:"preset_ms"`
}

// CounterDecl declares an up counter (CTU).
type CounterDecl struct {
	Name   string `json:"name"`
	Preset int64  `json:"preset"`
}

// Statement is one ordered statement: a coil write, a TON call, or a CTU call.
type Statement struct {
	Type    string     `json:"type"`    // "coil" | "ton" | "ctu"
	Target  string     `json:"target"`  // coil name (type=coil)
	Timer   string     `json:"timer"`   // timer name (type=ton)
	Counter string     `json:"counter"` // counter name (type=ctu)
	Cond    *Condition `json:"condition"`
	Reset   *Condition `json:"reset"` // ctu only
}

// Condition is a boolean expression tree over contacts and done flags.
type Condition struct {
	Op   string       `json:"op"` // "contact" | "done" | "and" | "or" | "not"
	Name string       `json:"name,omitempty"`
	Args []*Condition `json:"args,omitempty"`
}

// ScanRecord is one line of the per-scan input record (JSONL).
type ScanRecord struct {
	Scan       int             `json:"scan"`
	DurationMS int64           `json:"duration_ms"`
	Inputs     map[string]bool `json:"inputs"`
}

// LoadProgram parses and validates a program JSON document.
func LoadProgram(r io.Reader) (*Program, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var p Program
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse program: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadRecords parses the per-scan input record (JSONL, one ScanRecord per line).
func LoadRecords(r io.Reader) ([]ScanRecord, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var records []ScanRecord
	for {
		var rec ScanRecord
		err := dec.Decode(&rec)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse scan record %d: %w", len(records)+1, err)
		}
		records = append(records, rec)
	}
	return records, nil
}
