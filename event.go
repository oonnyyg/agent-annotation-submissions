package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// EventType enumerates the supported record kinds.
type EventType string

const (
	EventBuy   EventType = "buy"
	EventSell  EventType = "sell"
	EventSplit EventType = "split"
)

// Allocation pins part of a sell to a specific lot (by buy event id).
type Allocation struct {
	Lot      string `json:"lot"`
	Quantity string `json:"quantity"`
}

// Event is one JSONL record. Amounts are integer cents; quantities and
// split ratios are decimal or fraction strings (e.g. "1.5", "3/2").
type Event struct {
	ID     string    `json:"id"`
	Time   string    `json:"time"`
	Type   EventType `json:"type"`
	Method string    `json:"method,omitempty"` // sell: "fifo" (default) or "specific"

	Quantity   string `json:"quantity,omitempty"`    // buy/sell
	TotalCents *int64 `json:"total_cents,omitempty"` // buy/sell gross amount
	FeeCents   *int64 `json:"fee_cents,omitempty"`   // buy/sell fee
	Ratio      string `json:"ratio,omitempty"`       // split

	Allocations []Allocation `json:"allocations,omitempty"` // sell with method=specific
}

// parseDecimalOrFraction parses "1.25", "3/2", "7" into an exact rational.
func parseDecimalOrFraction(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty numeric string")
	}
	r, ok := new(big.Rat).SetString(s) // accepts decimals and "a/b"
	if !ok {
		return nil, fmt.Errorf("invalid decimal or fraction %q", s)
	}
	return r, nil
}

// parseEvents decodes JSONL lines into events (syntactic decode only).
func parseEvents(data string) ([]Event, error) {
	var events []Event
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ev); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		events = append(events, ev)
	}
	return events, nil
}
