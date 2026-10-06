// Package spf implements an offline SPF (RFC 7208 subset) evaluator
// that resolves records from a fixed DNS snapshot instead of live DNS.
package spf

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Zone holds the resource records of a single DNS name.
// TXT records are stored as segment lists; segments of one record are
// concatenated before use, per RFC 7208 section 3.3.
type Zone struct {
	TXT  [][]string `json:"TXT,omitempty"`
	A    []string   `json:"A,omitempty"`
	AAAA []string   `json:"AAAA,omitempty"`
}

// Snapshot is a fixed, offline view of DNS.
type Snapshot struct {
	// Zones maps a fully qualified domain name (any case) to its records.
	Zones map[string]Zone `json:"zones"`
	// Errors simulates resolver failures for "TYPE:name" keys,
	// e.g. "TXT:bad.example.com": "temperror".
	Errors map[string]string `json:"errors,omitempty"`
}

// ParseSnapshot decodes a JSON snapshot document.
func ParseSnapshot(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid snapshot JSON: %w", err)
	}
	if s.Zones == nil {
		s.Zones = map[string]Zone{}
	}
	// DNS names are case-insensitive: normalise keys once.
	norm := make(map[string]Zone, len(s.Zones))
	for name, z := range s.Zones {
		norm[strings.ToLower(strings.TrimSuffix(name, "."))] = z
	}
	s.Zones = norm
	if s.Errors != nil {
		normErr := make(map[string]string, len(s.Errors))
		for k, v := range s.Errors {
			normErr[strings.ToLower(k)] = v
		}
		s.Errors = normErr
	}
	return &s, nil
}
