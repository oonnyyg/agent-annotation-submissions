package scanlogic

import (
	"encoding/json"
	"fmt"
	"io"
)

// Condition is a boolean expression tree over contacts.
// Exactly one of Contact, Not, And, Or must be set.
type Condition struct {
	Contact string      `json:"contact,omitempty"`
	Not     *Condition  `json:"not,omitempty"`
	And     []Condition `json:"and,omitempty"`
	Or      []Condition `json:"or,omitempty"`
}

// Statement is one ordered program statement. Exactly one of
// Coil, TON, CTU must be set.
type Statement struct {
	Coil *CoilStmt `json:"coil,omitempty"`
	TON  *TONStmt  `json:"ton,omitempty"`
	CTU  *CTUStmt  `json:"ctu,omitempty"`
}

// CoilStmt assigns the condition value to a plain coil.
type CoilStmt struct {
	Target string    `json:"target"`
	Cond   Condition `json:"condition"`
}

// TONStmt is a non-retentive on-delay timer call.
type TONStmt struct {
	Timer string    `json:"timer"`
	Cond  Condition `json:"condition"`
}

// CTUStmt is an up-counter call with reset priority.
type CTUStmt struct {
	Counter string    `json:"counter"`
	Cond    Condition `json:"condition"`
	Reset   Condition `json:"reset"`
}

// TimerDecl declares a timer with a positive preset in milliseconds.
type TimerDecl struct {
	Name   string `json:"name"`
	Preset int64  `json:"preset"`
}

// CounterDecl declares a counter with a positive preset.
type CounterDecl struct {
	Name   string `json:"name"`
	Preset int64  `json:"preset"`
}

// Program is a validated scan-logic program.
type Program struct {
	Inputs     []string      `json:"inputs"`
	Coils      []string      `json:"coils"`
	Timers     []TimerDecl   `json:"timers"`
	Counters   []CounterDecl `json:"counters"`
	Statements []Statement   `json:"statements"`
}

// ScanInput is one scan record: full external input snapshot plus duration.
type ScanInput struct {
	DurationMs int64           `json:"duration_ms"`
	Inputs     map[string]bool `json:"inputs"`
}

// InputLog is the ordered list of per-scan input records.
type InputLog struct {
	Scans []ScanInput `json:"scans"`
}

// LoadProgram parses and validates a program.
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

// LoadInputLog parses and validates per-scan input records.
func LoadInputLog(r io.Reader, p *Program) (*InputLog, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var log InputLog
	if err := dec.Decode(&log); err != nil {
		return nil, fmt.Errorf("parse input log: %w", err)
	}
	for i, s := range log.Scans {
		if s.DurationMs <= 0 {
			return nil, fmt.Errorf("scan %d: duration_ms must be a positive integer, got %d", i, s.DurationMs)
		}
		for _, name := range p.Inputs {
			if _, ok := s.Inputs[name]; !ok {
				return nil, fmt.Errorf("scan %d: missing input %q (records must list all declared inputs)", i, name)
			}
		}
		for name := range s.Inputs {
			if !p.isInput(name) {
				return nil, fmt.Errorf("scan %d: undeclared input %q", i, name)
			}
		}
	}
	return &log, nil
}

func (p *Program) isInput(name string) bool {
	for _, n := range p.Inputs {
		if n == name {
			return true
		}
	}
	return false
}
