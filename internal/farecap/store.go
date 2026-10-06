package farecap

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// RevocationRecord keeps revocation history for audit.
type RevocationRecord struct {
	ID       string `json:"id"`
	Card     string `json:"target_card"`
	TargetID string `json:"target_id"`
	Line     int    `json:"input_line"`
}

type storedRide struct {
	Ride Ride
	Line int
}

// State persists effective rides and revocation history.
type State struct {
	Version int                `json:"version"`
	Rides   []storedRide       `json:"rides"`
	Revokes []RevocationRecord `json:"revocations"`
}

const stateVersion = 1

// Store is the application state machine.
type Store struct {
	cfg   *Config
	clock *Clock
	state *State
	byID  map[string]storedRide
	revID map[string]bool
}

func NewStore(cfg *Config, state *State) *Store {
	if state == nil {
		state = &State{Version: stateVersion}
	}
	if state.Version == 0 {
		state.Version = stateVersion
	}
	s := &Store{cfg: cfg, clock: NewClock(cfg), state: state, byID: map[string]storedRide{}, revID: map[string]bool{}}
	for _, sr := range state.Rides {
		s.byID[sr.Ride.ID] = sr
	}
	for _, rv := range state.Revokes {
		s.revID[rv.ID] = true
	}
	return s
}

// LineResult reports how an input line was handled.
type LineResult struct {
	Line   int    `json:"line"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

const (
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusDuplicate = "duplicate"
	StatusRevoked   = "revoked"
)

// Apply ingests one decoded event at 1-based input line number.
func (s *Store) Apply(e Event, line int) LineResult {
	res := LineResult{Line: line, ID: e.ID}
	switch e.Type {
	case TypeRide:
		r, err := e.ParseRide(s.clock.loc)
		if err != nil {
			res.Status = StatusRejected
			res.Reason = err.Error()
			return res
		}
		if old, ok := s.byID[r.ID]; ok {
			if ridesEqual(old.Ride, r) {
				res.Status = StatusDuplicate
				res.Reason = "identical record already present"
				return res
			}
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("id %s already exists with different content; old record left unchanged", r.ID)
			return res
		}
		if s.revID[r.ID] {
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("id %s already used by a revocation record", r.ID)
			return res
		}
		for _, other := range s.state.Rides {
			if other.Ride.Card == r.Card && Overlaps(other.Ride, r) {
				res.Status = StatusRejected
				res.Reason = fmt.Sprintf("overlaps with ride %s on card %s", other.Ride.ID, r.Card)
				return res
			}
		}
		width := max(r.From, r.To) - min(r.From, r.To) + 1
		if _, ok := s.cfg.FareFor(width); !ok {
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("no fare configured for coverage width %d", width)
			return res
		}
		s.state.Rides = append(s.state.Rides, storedRide{Ride: r, Line: line})
		s.byID[r.ID] = storedRide{Ride: r, Line: line}
		res.Status = StatusAccepted
		return res

	case TypeRevoke:
		if s.revID[e.ID] {
			res.Status = StatusDuplicate
			res.Reason = "identical revocation already recorded"
			return res
		}
		if old, ok := s.byID[e.ID]; ok {
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("revocation id %s conflicts with existing ride; nothing changed", old.Ride.ID)
			return res
		}
		target, ok := s.byID[e.TargetID]
		if !ok {
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("target ride %s not found", e.TargetID)
			return res
		}
		if target.Ride.Card != e.Card {
			res.Status = StatusRejected
			res.Reason = fmt.Sprintf("target ride %s belongs to another card", e.TargetID)
			return res
		}
		s.state.Revokes = append(s.state.Revokes, RevocationRecord{
			ID: e.ID, Card: target.Ride.Card, TargetID: e.TargetID, Line: line,
		})
		s.revID[e.ID] = true
		delete(s.byID, e.TargetID)
		for i, sr := range s.state.Rides {
			if sr.Ride.ID == e.TargetID {
				s.state.Rides = append(s.state.Rides[:i], s.state.Rides[i+1:]...)
				break
			}
		}
		res.Status = StatusRevoked
		return res
	}
	res.Status = StatusRejected
	res.Reason = "unhandled event type"
	return res
}

// Results recomputes billing for every card from effective rides.
func (s *Store) Results() ([]CardResult, error) {
	byCard := map[string][]Ride{}
	for _, sr := range s.state.Rides {
		byCard[sr.Ride.Card] = append(byCard[sr.Ride.Card], sr.Ride)
	}
	cards := make([]string, 0, len(byCard))
	for c := range byCard {
		cards = append(cards, c)
	}
	sort.Strings(cards)
	out := make([]CardResult, 0, len(cards))
	for _, c := range cards {
		res, err := Recompute(c, byCard[c], s.cfg, s.clock)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *Store) State() *State { return s.state }

func ridesEqual(a, b Ride) bool {
	return a.ID == b.ID && a.Card == b.Card && a.From == b.From && a.To == b.To &&
		a.Board.Equal(b.Board) && a.Alight.Equal(b.Alight)
}

// SaveState writes the state as indented JSON.
func SaveState(path string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadState reads a previously saved state.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &st, nil
}
