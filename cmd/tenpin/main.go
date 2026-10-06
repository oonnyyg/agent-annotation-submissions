// Command tenpin is a JSONL CLI for the ten-pin bowling score desk.
// It reads one JSON command per line from stdin and writes one JSON
// response per line to stdout. Games are persisted locally.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	bowling "example.com/tenpin-score-desk"
)

type request struct {
	Cmd       string `json:"cmd"`
	ID        string `json:"id,omitempty"`
	Player    string `json:"player,omitempty"`
	Pins      *int   `json:"pins,omitempty"`
	RollIndex *int   `json:"roll_index,omitempty"`
}

type response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

func main() {
	dir := flag.String("dir", envOr("TENPIN_DATA_DIR", "tenpin-data"),
		"directory where games are stored")
	flag.Parse()

	store, err := bowling.NewStore(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(response{OK: false, Error: "bad json: " + err.Error()})
			continue
		}
		enc.Encode(handle(store, req))
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func handle(store *bowling.Store, req request) response {
	switch req.Cmd {
	case "new":
		if req.ID == "" || req.Player == "" {
			return fail("new requires id and player")
		}
		g := bowling.NewGame(req.ID, req.Player)
		if err := store.Create(g); err != nil {
			return fail(err.Error())
		}
		return ok(g)
	case "roll":
		if req.Pins == nil {
			return fail("roll requires pins")
		}
		return mutate(store, req.ID, func(g *bowling.Game) error {
			return g.Roll(*req.Pins)
		})
	case "correct":
		if req.Pins == nil || req.RollIndex == nil {
			return fail("correct requires roll_index and pins")
		}
		return mutate(store, req.ID, func(g *bowling.Game) error {
			return g.Correct(*req.RollIndex, *req.Pins)
		})
	case "score":
		g, err := store.Load(req.ID)
		if err != nil {
			return fail(err.Error())
		}
		v, err := g.Score()
		if err != nil {
			return fail(err.Error())
		}
		return ok(v)
	case "list":
		games, err := store.List()
		if err != nil {
			return fail(err.Error())
		}
		return ok(games)
	default:
		return fail("unknown cmd " + fmt.Sprintf("%q", req.Cmd))
	}
}

// mutate loads a game, applies fn, and saves only on success.
func mutate(store *bowling.Store, id string, fn func(*bowling.Game) error) response {
	g, err := store.Load(id)
	if err != nil {
		return fail(err.Error())
	}
	if err := fn(g); err != nil {
		return fail(err.Error())
	}
	if err := store.Save(g); err != nil {
		return fail(err.Error())
	}
	v, err := g.Score()
	if err != nil {
		return fail(err.Error())
	}
	return ok(v)
}

func ok(data interface{}) response { return response{OK: true, Data: data} }
func fail(msg string) response     { return response{OK: false, Error: msg} }
