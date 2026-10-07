package main

import (
	"fmt"
	"math/big"
	"sort"
)

// StreamConfig describes one flow in the replay.
type StreamConfig struct {
	ID              string `json:"id"`
	Quantum         int64  `json:"quantum"`
	MaxWaitingBytes int64  `json:"maxWaitingBytes"`
}

// Config is the whole replay setup.
type Config struct {
	LinkRateBytesPerSec int64          `json:"linkRateBytesPerSec"`
	Streams             []StreamConfig `json:"streams"`
}

func (c *Config) validate() error {
	if c.LinkRateBytesPerSec <= 0 {
		return fmt.Errorf("link rate must be positive, got %d", c.LinkRateBytesPerSec)
	}
	seen := map[string]bool{}
	for _, s := range c.Streams {
		if s.ID == "" {
			return fmt.Errorf("stream id must not be empty")
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate stream id %q", s.ID)
		}
		seen[s.ID] = true
		if s.Quantum <= 0 {
			return fmt.Errorf("stream %q: quantum must be positive, got %d", s.ID, s.Quantum)
		}
		if s.MaxWaitingBytes <= 0 {
			return fmt.Errorf("stream %q: maxWaitingBytes must be positive, got %d", s.ID, s.MaxWaitingBytes)
		}
	}
	return nil
}

// Arrival is one packet-arrival event parsed from CSV.
type Arrival struct {
	Time   *big.Rat
	Stream string
	Packet string
	Length int64
	Seq    int
}

// sortArrivals orders by time, keeping input order for equal times.
func sortArrivals(ev []Arrival) {
	sort.SliceStable(ev, func(i, j int) bool {
		return ev[i].Time.Cmp(ev[j].Time) < 0
	})
}

// Packet is the runtime state of one admitted packet.
type Packet struct {
	ID     string
	Stream string
	Length int64
	Arrive *big.Rat
}

// SentRecord describes one completed transmission.
type SentRecord struct {
	Packet string
	Stream string
	Length int64
	Arrive *big.Rat
	Start  *big.Rat
	Finish *big.Rat
}

// DropRecord describes one dropped arrival.
type DropRecord struct {
	Packet string
	Stream string
	Length int64
	Time   *big.Rat
	Reason string
}

// StreamStats aggregates per-stream results.
type StreamStats struct {
	ID           string
	Admitted     int
	AdmittedByte int64
	Sent         int
	SentBytes    int64
	Dropped      int
	DroppedBytes int64
	TotalWait    *big.Rat
	MaxWait      *big.Rat
}

// Result is the full replay outcome.
type Result struct {
	Sent     []SentRecord
	Dropped  []DropRecord
	Stats    map[string]*StreamStats
	Order    []string
	Finished *big.Rat
}
