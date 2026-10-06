package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// Command 是一行 JSONL 请求。
type Command struct {
	Op     string `json:"op"`              // new | roll | correct | query
	Player string `json:"player"`          // 玩家名
	Game   string `json:"game"`            // 球局 ID
	Pins   *int   `json:"pins,omitempty"`  // roll/correct 的击倒数
	Index  *int   `json:"index,omitempty"` // correct 的投球下标（0 起）
}

type response struct {
	OK    bool       `json:"ok"`
	Error string     `json:"error,omitempty"`
	Game  *Scorecard `json:"game,omitempty"`
}

func main() {
	path := os.Getenv("TENPIN_DATA")
	if path == "" {
		path = "games.jsonl"
	}
	store, err := LoadStore(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load store:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd Command
		if err := json.Unmarshal(line, &cmd); err != nil {
			enc.Encode(response{OK: false, Error: "bad json: " + err.Error()})
			continue
		}
		enc.Encode(handle(store, &cmd))
	}
}

func handle(s *Store, c *Command) response {
	switch c.Op {
	case "new":
		g, err := s.Create(c.Player, c.Game)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := s.Save(); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		sc := g.Score()
		return response{OK: true, Game: &sc}
	case "roll":
		g, err := s.Get(c.Player, c.Game)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if c.Pins == nil {
			return response{OK: false, Error: "pins required"}
		}
		if err := g.AddRoll(*c.Pins); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := s.Save(); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		sc := g.Score()
		return response{OK: true, Game: &sc}
	case "correct":
		g, err := s.Get(c.Player, c.Game)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if c.Pins == nil || c.Index == nil {
			return response{OK: false, Error: "index and pins required"}
		}
		if err := g.Correct(*c.Index, *c.Pins); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		if err := s.Save(); err != nil {
			return response{OK: false, Error: err.Error()}
		}
		sc := g.Score()
		return response{OK: true, Game: &sc}
	case "query":
		g, err := s.Get(c.Player, c.Game)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		sc := g.Score()
		return response{OK: true, Game: &sc}
	default:
		return response{OK: false, Error: "unknown op: " + c.Op}
	}
}
