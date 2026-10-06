package main

import "errors"

// Game 表示一局十瓶保龄球，Rolls 按时间顺序保存原始投球。
type Game struct {
	Player  string       `json:"player"`
	ID      string       `json:"id"`
	Rolls   []int        `json:"rolls"`
	History []Correction `json:"history,omitempty"`
}

// Correction 记录一次更正：被改投球下标、原值、新值。
type Correction struct {
	Index int `json:"index"`
	Old   int `json:"old"`
	New   int `json:"new"`
}

var (
	ErrGameComplete = errors.New("game already complete")
	ErrInvalidPins  = errors.New("pins out of range for current position")
	ErrBadIndex     = errors.New("roll index out of range")
)

// frameInfo 描述解析到某一投球位置时的局面。
type frameInfo struct {
	Frame        int  `json:"frame"`
	RollInFrame  int  `json:"rollInFrame"`
	PinsStanding int  `json:"pinsStanding"`
	Complete     bool `json:"complete"`
}

// position 根据合法投球序列推导当前应投位置。
func position(rolls []int) frameInfo {
	i := 0
	for frame := 1; frame <= 9; frame++ {
		if i >= len(rolls) {
			return frameInfo{Frame: frame, RollInFrame: 1, PinsStanding: 10}
		}
		if rolls[i] == 10 {
			i++
			continue
		}
		if i+1 >= len(rolls) {
			return frameInfo{Frame: frame, RollInFrame: 2, PinsStanding: 10 - rolls[i]}
		}
		i += 2
	}
	tenth := rolls[i:]
	if len(tenth) == 0 {
		return frameInfo{Frame: 10, RollInFrame: 1, PinsStanding: 10}
	}
	if len(tenth) == 1 {
		if tenth[0] == 10 {
			return frameInfo{Frame: 10, RollInFrame: 2, PinsStanding: 10}
		}
		return frameInfo{Frame: 10, RollInFrame: 2, PinsStanding: 10 - tenth[0]}
	}
	if len(tenth) == 2 {
		first, second := tenth[0], tenth[1]
		if first == 10 {
			if second == 10 {
				return frameInfo{Frame: 10, RollInFrame: 3, PinsStanding: 10}
			}
			return frameInfo{Frame: 10, RollInFrame: 3, PinsStanding: 10 - second}
		}
		if first+second == 10 {
			return frameInfo{Frame: 10, RollInFrame: 3, PinsStanding: 10}
		}
		return frameInfo{Frame: 10, Complete: true}
	}
	return frameInfo{Frame: 10, Complete: true}
}

// validate 检查整个投球序列是否合法（允许未完成的序列）。
func validate(rolls []int) error {
	for i, p := range rolls {
		if p < 0 || p > 10 {
			return ErrInvalidPins
		}
		pos := position(rolls[:i])
		if pos.Complete {
			return ErrGameComplete
		}
		if p > pos.PinsStanding {
			return ErrInvalidPins
		}
	}
	return nil
}

// AddRoll 追加一次投球。
func (g *Game) AddRoll(pins int) error {
	pos := position(g.Rolls)
	if pos.Complete {
		return ErrGameComplete
	}
	if pins < 0 || pins > pos.PinsStanding {
		return ErrInvalidPins
	}
	g.Rolls = append(g.Rolls, pins)
	return nil
}

// Correct 更正第 index 次投球，重新验证整个序列；失败则保留原局。
func (g *Game) Correct(index, pins int) error {
	if index < 0 || index >= len(g.Rolls) {
		return ErrBadIndex
	}
	if pins < 0 || pins > 10 {
		return ErrInvalidPins
	}
	candidate := make([]int, len(g.Rolls))
	copy(candidate, g.Rolls)
	candidate[index] = pins
	if err := validate(candidate); err != nil {
		return err
	}
	old := g.Rolls[index]
	g.Rolls = candidate
	g.History = append(g.History, Correction{Index: index, Old: old, New: pins})
	return nil
}

// FrameScore 一轮的展示数据。
type FrameScore struct {
	Rolls      []int `json:"rolls"`
	Score      *int  `json:"score,omitempty"`
	Cumulative *int  `json:"cumulative,omitempty"`
}

// Scorecard 是查询返回的完整记分单。
type Scorecard struct {
	Player   string       `json:"player"`
	Game     string       `json:"game"`
	Frames   []FrameScore `json:"frames"`
	Position frameInfo    `json:"position"`
	Complete bool         `json:"complete"`
	Total    *int         `json:"total,omitempty"`
	History  []Correction `json:"history,omitempty"`
}

// Score 计算记分单。轮分待定时不填，累计分只公布到已确定前缀。
func (g *Game) Score() Scorecard {
	rolls := g.Rolls
	var frames []FrameScore
	i := 0
	cum := 0
	cumValid := true
	for f := 1; f <= 10 && i < len(rolls); f++ {
		var fs FrameScore
		var frameEnd, bonusNeed int
		if f < 10 {
			if rolls[i] == 10 {
				fs.Rolls = rolls[i : i+1]
				frameEnd = i + 1
				bonusNeed = 2
			} else if i+1 < len(rolls) {
				fs.Rolls = rolls[i : i+2]
				frameEnd = i + 2
				if rolls[i]+rolls[i+1] == 10 {
					bonusNeed = 1
				}
			} else {
				fs.Rolls = rolls[i:]
				frameEnd = len(rolls)
				bonusNeed = -1
			}
		} else {
			fs.Rolls = rolls[i:]
			pos := position(rolls)
			if !pos.Complete {
				bonusNeed = -1
			}
			frameEnd = len(rolls)
		}
		if bonusNeed >= 0 && len(rolls)-frameEnd >= bonusNeed {
			s := sum(fs.Rolls) + sum(rolls[frameEnd:frameEnd+bonusNeed])
			fs.Score = &s
		}
		if fs.Score != nil && cumValid {
			cum += *fs.Score
			c := cum
			fs.Cumulative = &c
		} else {
			cumValid = false
		}
		frames = append(frames, fs)
		i = frameEnd
	}
	pos := position(rolls)
	sc := Scorecard{
		Player:   g.Player,
		Game:     g.ID,
		Frames:   frames,
		Position: pos,
		Complete: pos.Complete,
		History:  g.History,
	}
	if pos.Complete && cumValid {
		t := cum
		sc.Total = &t
	}
	return sc
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}
