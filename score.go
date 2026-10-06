package bowling

// FrameView is the score-sheet view of one frame.
type FrameView struct {
	Rolls []int `json:"rolls"`
	// Score is the frame score once determinable, else nil (pending).
	Score *int `json:"score"`
	// Cumulative is published only when every earlier frame is determined.
	Cumulative *int `json:"cumulative"`
}

// ScoreView is the full score-sheet of a game.
type ScoreView struct {
	GameID   string      `json:"game_id"`
	Player   string      `json:"player"`
	Frames   []FrameView `json:"frames"`
	Complete bool        `json:"complete"`
	// Total is published only for a completed game.
	Total       *int         `json:"total"`
	NextFrame   int          `json:"next_frame"` // 1-based, 0 when complete
	NextRoll    int          `json:"next_roll"`  // 1-based roll within frame, 0 when complete
	Corrections []Correction `json:"corrections,omitempty"`
}

// Score computes the current score-sheet. Pending frame scores are never
// zero-filled; cumulative totals appear only when all prior frames are
// determined, and Total only when the game is complete.
func (g *Game) Score() (*ScoreView, error) {
	frames, complete, err := parseFrames(g.Rolls)
	if err != nil {
		return nil, err
	}
	view := &ScoreView{
		GameID:      g.ID,
		Player:      g.Player,
		Complete:    complete,
		Corrections: g.Corrections,
	}
	cum := 0
	cumValid := true
	for idx, fr := range frames {
		fv := FrameView{Rolls: rollValues(g.Rolls, fr)}
		if s, ok := frameScore(g.Rolls, fr, idx == 9); ok {
			sc := s
			fv.Score = &sc
			if cumValid {
				cum += s
				c := cum
				fv.Cumulative = &c
			}
		} else {
			cumValid = false
		}
		view.Frames = append(view.Frames, fv)
	}
	if complete {
		t := cum
		view.Total = &t
	} else {
		nf, nr := nextPosition(frames, g.Rolls)
		view.NextFrame = nf
		view.NextRoll = nr
	}
	return view, nil
}

func rollValues(rolls []int, fr frame) []int {
	out := make([]int, 0, len(fr.rolls))
	for _, ri := range fr.rolls {
		out = append(out, rolls[ri])
	}
	return out
}

// frameScore returns the score of a frame and whether it is determined.
func frameScore(rolls []int, fr frame, isTenth bool) (int, bool) {
	idx := fr.rolls
	sum := 0
	for _, ri := range idx {
		sum += rolls[ri]
	}
	if isTenth {
		if frameComplete(fr, rolls) {
			return sum, true
		}
		return 0, false
	}
	if rolls[idx[0]] == 10 { // strike: needs next two rolls
		if len(rolls) > idx[0]+2 {
			return sum + rolls[idx[0]+1] + rolls[idx[0]+2], true
		}
		return 0, false
	}
	if len(idx) == 2 && rolls[idx[0]]+rolls[idx[1]] == 10 { // spare
		if len(rolls) > idx[1]+1 {
			return sum + rolls[idx[1]+1], true
		}
		return 0, false
	}
	if len(idx) == 2 {
		return sum, true
	}
	return 0, false // open frame with only one roll so far
}

// nextPosition returns the 1-based frame and roll-within-frame to throw next.
func nextPosition(frames []frame, rolls []int) (int, int) {
	if len(frames) == 0 {
		return 1, 1
	}
	last := frames[len(frames)-1]
	lastDone := false
	if len(frames) == 10 {
		lastDone = frameComplete(last, rolls)
	} else {
		first := rolls[last.rolls[0]]
		lastDone = first == 10 || len(last.rolls) == 2
	}
	if lastDone {
		return len(frames) + 1, 1
	}
	return len(frames), len(last.rolls) + 1
}
