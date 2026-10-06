package domain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"example.com/stocktake-adjustment-desk/internal/store"
)

// Record is one imported count line.
type Record struct {
	ID       string
	Location string
	SKU      string
	Qty      int64
	Revoked  bool
}

// Stocktake is the aggregate reconstructed from the event log.
type Stocktake struct {
	ID        string
	Location  string
	Status    string // open, sealed, posted
	Baseline  map[string]int64
	Records   map[string]*Record
	Completed map[string]bool
	Details   []store.CountDetail
	Diff      map[string]int64
	Posted    bool
}

// State is the full materialized view of the event log.
type State struct {
	Inventory  map[string]map[string]int64 // location -> sku -> qty
	Stocktakes map[string]*Stocktake
	// activeByLocation lists stocktake ids that are still open per location.
	activeByLocation map[string]string
}

// NewState returns an empty state.
func NewState() *State {
	return &State{
		Inventory:        map[string]map[string]int64{},
		Stocktakes:       map[string]*Stocktake{},
		activeByLocation: map[string]string{},
	}
}

// Load reads every event from dir and replays it into a fresh State.
func Load(path string) (*State, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st := NewState()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e store.Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("replay: %w", err)
		}
		if err := st.Apply(&e); err != nil {
			return nil, fmt.Errorf("replay seq %d: %w", e.Seq, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *State) locQty(loc, sku string) int64 {
	if m, ok := s.Inventory[loc]; ok {
		return m[sku]
	}
	return 0
}

func (s *State) addQty(loc, sku string, delta int64) {
	m := s.Inventory[loc]
	if m == nil {
		m = map[string]int64{}
		s.Inventory[loc] = m
	}
	m[sku] += delta
	if m[sku] == 0 {
		delete(m, sku)
	}
}

// Apply folds one event into the state. It is used both during replay
// and immediately after a successful append to keep the in-memory view fresh.
func (s *State) Apply(e *store.Event) error {
	switch e.Type {
	case "inbound", "outbound":
		s.addQty(e.Location, e.SKU, signedQty(e.Type, e.Qty))
	case "stocktake_started":
		st := &Stocktake{
			ID:        e.Stocktake,
			Location:  e.Location,
			Status:    "open",
			Baseline:  cloneMap(e.Baseline),
			Records:   map[string]*Record{},
			Completed: map[string]bool{},
		}
		s.Stocktakes[e.Stocktake] = st
		s.activeByLocation[e.Location] = e.Stocktake
	case "count_recorded":
		st := mustStocktake(s, e.Stocktake)
		if _, exists := st.Records[e.RecordID]; exists {
			break // idempotent re-import of identical content
		}
		st.Records[e.RecordID] = &Record{
			ID:       e.RecordID,
			Location: e.Location,
			SKU:      e.SKU,
			Qty:      e.Qty,
		}
	case "count_revoked":
		st := mustStocktake(s, e.Stocktake)
		if r := st.Records[e.RecordID]; r != nil {
			r.Revoked = true
		}
	case "location_completed":
		st := mustStocktake(s, e.Stocktake)
		st.Completed[e.Location] = true
	case "stocktake_sealed":
		st := mustStocktake(s, e.Stocktake)
		st.Status = "sealed"
		st.Details = e.Details
		st.Diff = cloneMap(e.Diff)
		delete(s.activeByLocation, st.Location)
	case "stocktake_posted":
		st := mustStocktake(s, e.Stocktake)
		st.Status = "posted"
		st.Posted = true
		for sku, d := range e.Diff {
			s.addQty(st.Location, sku, d)
		}
	default:
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	return nil
}

func mustStocktake(s *State, id string) *Stocktake {
	st := s.Stocktakes[id]
	if st == nil {
		panic(fmt.Sprintf("missing stocktake %q during apply", id))
	}
	return st
}

func signedQty(typ string, q int64) int64 {
	if typ == "outbound" {
		return -q
	}
	return q
}

func cloneMap(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
