// Command seatdesk reads JSONL operations from stdin and writes one JSON
// response per line to stdout. State persists in a directory (-dir flag or
// SEATDESK_DIR, default ./seatdesk-data).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/group-seat-allocation/internal/hall"
)

type request struct {
	Op     string      `json:"op"`
	Rows   int         `json:"rows"`
	Cols   int         `json:"cols"`
	Seats  []hall.Seat `json:"seats"`
	ID     string      `json:"id"`
	Size   int         `json:"size"`
	Pref   hall.Seat   `json:"pref"`
	Orders []orderReq  `json:"orders"`
}

type orderReq struct {
	ID   string    `json:"id"`
	Size int       `json:"size"`
	Pref hall.Seat `json:"pref"`
}

func reply(v any) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func fail(msg string) {
	reply(map[string]any{"ok": false, "error": msg})
}

func main() {
	dir := flag.String("dir", os.Getenv("SEATDESK_DIR"), "state directory")
	flag.Parse()
	if *dir == "" {
		*dir = "seatdesk-data"
	}
	h, err := hall.Load(*dir)
	if err != nil {
		fail("load state: " + err.Error())
		return
	}
	save := func() {
		if err := h.Save(*dir); err != nil {
			fail("save state: " + err.Error())
		}
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			fail("bad json: " + err.Error())
			continue
		}
		switch req.Op {
		case "create":
			nh, err := hall.NewHall(req.Rows, req.Cols)
			if err != nil {
				fail(err.Error())
				continue
			}
			h = nh
			save()
			reply(map[string]any{"ok": true, "rows": h.Rows, "cols": h.Cols})
		case "disable", "enable":
			if h == nil {
				fail("hall not created")
				continue
			}
			if err := h.SetDisabled(req.Seats, req.Op == "disable"); err != nil {
				fail(err.Error())
				continue
			}
			save()
			reply(map[string]any{"ok": true})
		case "book":
			if h == nil {
				fail("hall not created")
				continue
			}
			reqs := make([]hall.OrderRequest, len(req.Orders))
			for i, o := range req.Orders {
				reqs[i] = hall.OrderRequest{ID: o.ID, Size: o.Size, Pref: o.Pref}
			}
			orders, err := h.Book(reqs)
			if err != nil {
				fail(err.Error())
				continue
			}
			save()
			reply(map[string]any{"ok": true, "orders": orders})
		case "change":
			if h == nil {
				fail("hall not created")
				continue
			}
			o, err := h.Change(req.ID, req.Size, req.Pref)
			if err != nil {
				fail(err.Error())
				continue
			}
			save()
			reply(map[string]any{"ok": true, "order": o})
		case "release":
			if h == nil {
				fail("hall not created")
				continue
			}
			o, err := h.Release(req.ID)
			if err != nil {
				fail(err.Error())
				continue
			}
			save()
			reply(map[string]any{"ok": true, "released": o})
		case "query":
			if h == nil {
				fail("hall not created")
				continue
			}
			orders := h.SortedOrders()
			occ := map[string]string{}
			for _, o := range orders {
				for _, s := range o.Seats {
					occ[fmt.Sprintf("%d,%d", s.Row, s.Col)] = o.ID
				}
			}
			disabled := []hall.Seat{}
			for r := 1; r <= h.Rows; r++ {
				for c := 1; c <= h.Cols; c++ {
					if h.Disabled[fmt.Sprintf("%d,%d", r, c)] {
						disabled = append(disabled, hall.Seat{Row: r, Col: c})
					}
				}
			}
			reply(map[string]any{
				"ok": true, "rows": h.Rows, "cols": h.Cols,
				"orders": orders, "occupied": occ, "disabled": disabled,
			})
		default:
			fail("unknown op: " + req.Op)
		}
	}
	if err := sc.Err(); err != nil {
		fail("read stdin: " + err.Error())
	}
}
