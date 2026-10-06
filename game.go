// Package bowling implements ten-pin bowling score keeping for a practice
// desk: multiple players, multiple independent games, correction of
// historical rolls with full re-validation and re-scoring.
package bowling

import "errors"

var (
	ErrGameComplete   = errors.New("game is already complete")
	ErrInvalidPins    = errors.New("pins out of range for remaining pins")
	ErrRollOutOfRange = errors.New("roll index out of range")
)

// Correction records one successful correction operation.
type Correction struct {
	RollIndex int `json:"roll_index"`
	OldPins   int `json:"old_pins"`
	NewPins   int `json:"new_pins"`
}

// Game is a single ten-pin bowling game for one player.
type Game struct {
	ID          string       `json:"id"`
	Player      string       `json:"player"`
	Rolls       []int        `json:"rolls"`
	Corrections []Correction `json:"corrections,omitempty"`
}

// NewGame creates an empty game.
func NewGame(id, player string) *Game {
	return &Game{ID: id, Player: player, Rolls: []int{}}
}

// frame describes one parsed frame as a slice of roll indices.
type frame struct {
	rolls []int // indices into Game.Rolls
}

// parseFrames splits rolls into frames and validates every roll against the
// pins remaining at the moment it was thrown. It returns the frames and
// whether the game is complete.
func parseFrames(rolls []int) ([]frame, bool, error) {
	frames := make([]frame, 0, 10)
	i := 0
	for f := 0; f < 10 && i < len(rolls); f++ {
		if f < 9 {
			first := rolls[i]
			if first < 0 || first > 10 {
				return nil, false, ErrInvalidPins
			}
			if first == 10 {
				frames = append(frames, frame{rolls: []int{i}})
				i++
				continue
			}
			if i+1 >= len(rolls) {
				frames = append(frames, frame{rolls: []int{i}})
				i++
				continue
			}
			second := rolls[i+1]
			if second < 0 || second > 10-first {
				return nil, false, ErrInvalidPins
			}
			frames = append(frames, frame{rolls: []int{i, i + 1}})
			i += 2
			continue
		}
		// Tenth frame: up to three rolls with rack resets.
		start := i
		pinsLeft := 10
		bonus := 0 // extra rolls earned
		thrown := 0
		for i < len(rolls) && thrown < 2+bonus {
			p := rolls[i]
			if p < 0 || p > pinsLeft {
				return nil, false, ErrInvalidPins
			}
			thrown++
			i++
			if pinsLeft == 10 && p == 10 {
				// Strike: reset rack, earn bonus rolls.
				if thrown == 1 {
					bonus = 2
				}
				pinsLeft = 10
			} else {
				pinsLeft -= p
				if thrown == 2 && pinsLeft == 0 {
					// Spare: one bonus roll, fresh rack.
					bonus = 1
					pinsLeft = 10
				}
			}
		}
		frames = append(frames, frame{rolls: rollsIndices(start, i)})
	}
	complete := len(frames) == 10 && frameComplete(frames[9], rolls)
	return frames, complete, nil
}

func rollsIndices(start, end int) []int {
	out := make([]int, 0, end-start)
	for k := start; k < end; k++ {
		out = append(out, k)
	}
	return out
}

// frameComplete reports whether the tenth frame has all its rolls.
func frameComplete(fr frame, rolls []int) bool {
	n := len(fr.rolls)
	if n == 0 {
		return false
	}
	first := rolls[fr.rolls[0]]
	if first == 10 {
		return n == 3
	}
	if n >= 2 && first+rolls[fr.rolls[1]] == 10 {
		return n == 3
	}
	return n == 2
}

// Roll appends a throw after validating it against the current state.
func (g *Game) Roll(pins int) error {
	_, complete, err := parseFrames(g.Rolls)
	if err != nil {
		return err
	}
	if complete {
		return ErrGameComplete
	}
	candidate := append(append([]int{}, g.Rolls...), pins)
	if _, _, err := parseFrames(candidate); err != nil {
		return err
	}
	g.Rolls = candidate
	return nil
}

// Correct changes the pins of the roll at rollIndex. The full roll sequence
// is re-validated; if any later roll becomes illegal the whole correction is
// rejected and the game is left untouched.
func (g *Game) Correct(rollIndex, pins int) error {
	if rollIndex < 0 || rollIndex >= len(g.Rolls) {
		return ErrRollOutOfRange
	}
	candidate := append([]int{}, g.Rolls...)
	old := candidate[rollIndex]
	candidate[rollIndex] = pins
	if _, _, err := parseFrames(candidate); err != nil {
		return err
	}
	g.Rolls = candidate
	g.Corrections = append(g.Corrections, Correction{
		RollIndex: rollIndex, OldPins: old, NewPins: pins,
	})
	return nil
}
