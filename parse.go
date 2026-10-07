package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// LoadConfig reads the JSON replay configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

// parseTime accepts non-negative decimal seconds, exactly.
func parseTime(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid time %q", s)
	}
	if r.Sign() < 0 {
		return nil, fmt.Errorf("negative time %q", s)
	}
	return r, nil
}

// LoadArrivals reads CSV events with header: time,stream,packet,length.
func LoadArrivals(path string, cfg *Config) ([]Arrival, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseArrivals(f, cfg)
}

// ParseArrivals parses arrival CSV from any reader.
func ParseArrivals(r io.Reader, cfg *Config) ([]Arrival, error) {
	known := map[string]bool{}
	for _, s := range cfg.Streams {
		known[s.ID] = true
	}
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}
	want := []string{"time", "stream", "packet", "length"}
	if len(header) != len(want) {
		return nil, fmt.Errorf("csv header must be %v, got %v", want, header)
	}
	for i, w := range want {
		if strings.ToLower(strings.TrimSpace(header[i])) != w {
			return nil, fmt.Errorf("csv column %d must be %q, got %q", i+1, w, header[i])
		}
	}
	seenPkt := map[string]bool{}
	var out []Arrival
	line := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("csv line %d: %w", line, err)
		}
		t, err := parseTime(rec[0])
		if err != nil {
			return nil, fmt.Errorf("csv line %d: %w", line, err)
		}
		stream := strings.TrimSpace(rec[1])
		if !known[stream] {
			return nil, fmt.Errorf("csv line %d: unknown stream %q", line, stream)
		}
		pkt := strings.TrimSpace(rec[2])
		if pkt == "" {
			return nil, fmt.Errorf("csv line %d: empty packet id", line)
		}
		if seenPkt[pkt] {
			return nil, fmt.Errorf("csv line %d: duplicate packet id %q", line, pkt)
		}
		seenPkt[pkt] = true
		length, err := strconv.ParseInt(strings.TrimSpace(rec[3]), 10, 64)
		if err != nil || length <= 0 {
			return nil, fmt.Errorf("csv line %d: length must be a positive integer, got %q", line, rec[3])
		}
		out = append(out, Arrival{Time: t, Stream: stream, Packet: pkt, Length: length, Seq: len(out)})
	}
	sortArrivals(out)
	return out, nil
}
