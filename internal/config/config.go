package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NullOrder controls where empty key fields sort.
type NullOrder string

const (
	NullFirst NullOrder = "first"
	NullLast  NullOrder = "last"
)

// KeyType selects comparison semantics for one key column.
type KeyType string

const (
	KeyText KeyType = "text"
	// KeyDecimal compares finite decimal values without float conversion.
	KeyDecimal KeyType = "decimal"
)

// Direction is ascending or descending.
type Direction string

const (
	Asc  Direction = "asc"
	Desc Direction = "desc"
)

// KeyRule describes one sort-key column.
type KeyRule struct {
	Column    string    `json:"column"`
	Type      KeyType   `json:"type"`
	Direction Direction `json:"direction"`
	// NullOrder states where empty fields sort relative to non-empty ones.
	NullOrder NullOrder `json:"null_order"`
}

// Rules is the full JSON rule document.
type Rules struct {
	Keys        []KeyRule `json:"keys"`
	ChunkSize   int       `json:"chunk_size"`
	MergeFanIn  int       `json:"merge_fan_in"`
	Deduplicate bool      `json:"deduplicate"`
}

// Load reads and validates a rules JSON file.
func Load(path string) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules: %w", err)
	}
	var r Rules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// Validate checks rule-level constraints independent of the CSV header.
func (r *Rules) Validate() error {
	if len(r.Keys) == 0 {
	return fmt.Errorf("rules: at least one sort key is required")
	}
	seen := map[string]bool{}
	for i := range r.Keys {
		k := &r.Keys[i]
		if strings.TrimSpace(k.Column) == "" {
			return fmt.Errorf("rules: key %d: column must be non-empty", i)
		}
		if seen[k.Column] {
			return fmt.Errorf("rules: duplicate key column %q", k.Column)
		}
		seen[k.Column] = true
		switch k.Type {
		case KeyText, KeyDecimal:
		case "":
			return fmt.Errorf("rules: key %q: type is required", k.Column)
		default:
			return fmt.Errorf("rules: key %q: unknown type %q", k.Column, k.Type)
		}
		switch k.Direction {
		case Asc, Desc, "":
		default:
			return fmt.Errorf("rules: key %q: unknown direction %q", k.Column, k.Direction)
		}
		switch k.NullOrder {
		case NullFirst, NullLast, "":
		default:
			return fmt.Errorf("rules: key %q: unknown null_order %q", k.Column, k.NullOrder)
		}
	}
	if r.ChunkSize <= 0 {
		return fmt.Errorf("rules: chunk_size must be a positive integer, got %d", r.ChunkSize)
	}
	if r.MergeFanIn < 2 {
		return fmt.Errorf("rules: merge_fan_in must be an integer >= 2, got %d", r.MergeFanIn)
	}
	r.applyDefaults()
	return nil
}

func (r *Rules) applyDefaults() {
	for i := range r.Keys {
		k := &r.Keys[i]
		if k.Direction == "" {
			k.Direction = Asc
		}
		if k.NullOrder == "" {
			k.NullOrder = NullLast
		}
	}
}

// Resolve maps key columns to header indexes and returns the resolved keys.
func (r *Rules) Resolve(header []string) ([]ResolvedKey, error) {
	index := make(map[string]int, len(header))
	for i, name := range header {
		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("input header: duplicate column %q", name)
		}
		index[name] = i
	}
	out := make([]ResolvedKey, len(r.Keys))
	for i, k := range r.Keys {
		idx, ok := index[k.Column]
		if !ok {
			return nil, fmt.Errorf("rules: key column %q not found in input header", k.Column)
		}
		out[i] = ResolvedKey{Index: idx, Rule: k}
	}
	return out, nil
}

// ResolvedKey binds a key rule to a concrete column index.
type ResolvedKey struct {
	Index int
	Rule  KeyRule
}
