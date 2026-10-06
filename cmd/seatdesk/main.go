// seatdesk 是小型影厅连座分配工具，按 JSONL 接收操作并逐行返回 JSON。
//
// 用法: go run ./cmd/seatdesk -dir <状态目录>
//
// 支持的操作（每行一个 JSON 对象）:
//
//	{"op":"init","rows":2,"cols":10,"disabled":[{"row":1,"col":3}]}
//	{"op":"alloc","orders":[{"id":"A","size":3,"row":1,"col":4},...]}  // 一批至多 6 单
//	{"op":"reseat","id":"A","size":2,"row":1,"col":5}
//	{"op":"release","id":"A"}
//	{"op":"query"}
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"example.com/group-seat-allocation/internal/seatdesk"
)

type opMsg struct {
	Op       string             `json:"op"`
	Rows     int                `json:"rows"`
	Cols     int                `json:"cols"`
	Disabled []seatdesk.Seat    `json:"disabled"`
	Orders   []seatdesk.Request `json:"orders"`
	ID       string             `json:"id"`
	Size     int                `json:"size"`
	Row      int                `json:"row"`
	Col      int                `json:"col"`
}

func reply(ok bool, msg string, extra map[string]any) {
	out := map[string]any{"ok": ok}
	if msg != "" {
		out["error"] = msg
	}
	for k, v := range extra {
		out[k] = v
	}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
}

func fail(err error) { reply(false, err.Error(), nil) }

func main() {
	dir := flag.String("dir", "seatdesk-state", "保存影厅与订单状态的目录")
	flag.Parse()

	store, err := seatdesk.Load(*dir)
	if err != nil {
		fail(err)
		os.Exit(1)
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m opMsg
		if err := json.Unmarshal(line, &m); err != nil {
			fail(fmt.Errorf("JSON 解析失败: %w", err))
			continue
		}
		dirty := false
		switch m.Op {
		case "init":
			if err := store.InitHall(m.Rows, m.Cols, m.Disabled); err != nil {
				fail(err)
			} else {
				dirty = true
				reply(true, "", nil)
			}
		case "alloc":
			orders, err := store.AllocateBatch(m.Orders)
			if err != nil {
				fail(err)
			} else {
				dirty = true
				reply(true, "", map[string]any{"orders": orders})
			}
		case "reseat":
			o, err := store.Reseat(m.ID, m.Size, m.Row, m.Col)
			if err != nil {
				fail(err)
			} else {
				dirty = true
				reply(true, "", map[string]any{"order": o})
			}
		case "release":
			if err := store.Release(m.ID); err != nil {
				fail(err)
			} else {
				dirty = true
				reply(true, "", nil)
			}
		case "query":
			ids := make([]string, 0, len(store.Orders))
			for id := range store.Orders {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			orders := make([]*seatdesk.Order, 0, len(ids))
			occupied := map[string]string{}
			for _, id := range ids {
				o := store.Orders[id]
				orders = append(orders, o)
				for _, st := range o.Seats {
					occupied[fmt.Sprintf("%d,%d", st.Row, st.Col)] = id
				}
			}
			reply(true, "", map[string]any{
				"hall": store.Hall, "orders": orders, "occupied": occupied,
			})
		default:
			fail(fmt.Errorf("未知操作: %q", m.Op))
		}
		if dirty {
			if err := store.Save(); err != nil {
				fail(fmt.Errorf("状态保存失败: %w", err))
			}
		}
	}
	if err := sc.Err(); err != nil {
		fail(err)
		os.Exit(1)
	}
}
