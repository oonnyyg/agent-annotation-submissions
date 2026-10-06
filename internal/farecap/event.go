package farecap

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	TypeRide   = "ride"
	TypeRevoke = "revoke"
)

// Event is one JSONL input line: either a ride record or a revocation.
type Event struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Card     string `json:"card"`
	Board    string `json:"board,omitempty"`
	Alight   string `json:"alight,omitempty"`
	FromZone int    `json:"from_zone,omitempty"`
	ToZone   int    `json:"to_zone,omitempty"`
	TargetID string `json:"target_id,omitempty"`
}

// Ride is a validated ride record with parsed times (UTC instants).
type Ride struct {
	ID     string
	Card   string
	Board  time.Time
	Alight time.Time
	From   int
	To     int
}

func (e *Event) validateRaw() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("missing id")
	}
	if strings.TrimSpace(e.Card) == "" {
		return fmt.Errorf("missing card")
	}
	switch e.Type {
	case TypeRide:
	case TypeRevoke:
		return nil
	default:
		return fmt.Errorf("unknown type %q", e.Type)
	}
	return nil
}

// ParseRide validates the event and converts it into a Ride.
// Naive timestamps (no zone designator) are interpreted at loc.
func (e *Event) ParseRide(loc *time.Location) (Ride, error) {
	if err := e.validateRaw(); err != nil {
		return Ride{}, err
	}
	if e.Type != TypeRide {
		return Ride{}, fmt.Errorf("not a ride event")
	}
	board, err := parseTime(e.Board, loc)
	if err != nil {
		return Ride{}, fmt.Errorf("board: %w", err)
	}
	alight, err := parseTime(e.Alight, loc)
	if err != nil {
		return Ride{}, fmt.Errorf("alight: %w", err)
	}
	if alight.Before(board) {
		return Ride{}, fmt.Errorf("alight %s before board %s", e.Alight, e.Board)
	}
	if e.FromZone <= 0 || e.ToZone <= 0 {
		return Ride{}, fmt.Errorf("zones must be positive integers, got from=%d to=%d", e.FromZone, e.ToZone)
	}
	return Ride{ID: e.ID, Card: e.Card, Board: board, Alight: alight, From: e.FromZone, To: e.ToZone}, nil
}

func parseTime(s string, loc *time.Location) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, fmt.Errorf("missing timestamp")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	// Naive local timestamp: interpret at the configured fixed UTC offset.
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unparsable timestamp %q (use RFC3339 or 2006-01-02T15:04:05)", s)
}

func decodeEvent(raw []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return Event{}, err
	}
	if err := e.validateRaw(); err != nil {
		return Event{}, err
	}
	if e.Type == TypeRevoke && strings.TrimSpace(e.TargetID) == "" {
		return Event{}, fmt.Errorf("revoke event %s missing target_id", e.ID)
	}
	return e, nil
}

// DecodeEventForCLI parses one JSONL line into an Event.
func DecodeEventForCLI(raw []byte) (Event, error) { return decodeEvent(raw) }
