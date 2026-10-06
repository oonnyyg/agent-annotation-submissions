package seatdesk

import (
	"fmt"
	"sort"
)

// Request 是一笔待分配订单的请求参数。
type Request struct {
	ID   string `json:"id"`
	Size int    `json:"size"`
	Row  int    `json:"row"`
	Col  int    `json:"col"`
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// costOf 计算一组座位（已按坐标升序）到首选坐标的曼哈顿距离之和。
func costOf(seats []Seat, row, col int) int {
	c := 0
	for _, st := range seats {
		c += abs(st.Row-row) + abs(st.Col-col)
	}
	return c
}

// candidates 枚举某订单在当前占用下的全部可行连座方案，
// 返回的方案按坐标序列字典序升序排列。
func (s *Store) candidates(size int, occ map[Seat]string) [][]Seat {
	disabled := s.disabledSet()
	var out [][]Seat
	for r := 1; r <= s.Hall.Rows; r++ {
		for start := 1; start+size-1 <= s.Hall.Cols; start++ {
			run := make([]Seat, 0, size)
			ok := true
			for c := start; c < start+size; c++ {
				st := Seat{Row: r, Col: c}
				if disabled[st] {
					ok = false
					break
				}
				if _, taken := occ[st]; taken {
					ok = false
					break
				}
				run = append(run, st)
			}
			if ok {
				out = append(out, run)
			}
		}
	}
	return out
}

// flatten 把分配结果按订单编号字典序、单内座位坐标升序拼接成坐标序列。
func flatten(ids []string, assign map[string][]Seat) []int {
	var seq []int
	for _, id := range ids {
		for _, st := range assign[id] {
			seq = append(seq, st.Row, st.Col)
		}
	}
	return seq
}

func lessSeq(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// solve 在给定初始占用下为一批订单求总代价最小的分配，
// 同代价时取拼接坐标序列字典序最小者。无解时返回 nil。
func (s *Store) solve(reqs []Request, occ map[Seat]string) map[string][]Seat {
	ids := make([]string, len(reqs))
	cands := make(map[string][][]Seat, len(reqs))
	for i, r := range reqs {
		ids[i] = r.ID
		cs := s.candidates(r.Size, occ)
		if len(cs) == 0 {
			return nil
		}
		cands[r.ID] = cs
	}
	sort.Strings(ids)

	reqByID := map[string]Request{}
	for _, r := range reqs {
		reqByID[r.ID] = r
	}

	bestCost := -1
	var bestSeq []int
	var best map[string][]Seat

	assign := map[string][]Seat{}
	used := map[Seat]bool{}
	var dfs func(idx, curCost int)
	dfs = func(idx, curCost int) {
		if bestCost >= 0 && curCost > bestCost {
			return
		}
		if idx == len(ids) {
			seq := flatten(ids, assign)
			if bestCost < 0 || curCost < bestCost || (curCost == bestCost && lessSeq(seq, bestSeq)) {
				bestCost = curCost
				bestSeq = seq
				best = map[string][]Seat{}
				for id, sts := range assign {
					cp := make([]Seat, len(sts))
					copy(cp, sts)
					best[id] = cp
				}
			}
			return
		}
		id := ids[idx]
		req := reqByID[id]
		for _, run := range cands[id] {
			clash := false
			for _, st := range run {
				if used[st] {
					clash = true
					break
				}
			}
			if clash {
				continue
			}
			for _, st := range run {
				used[st] = true
			}
			assign[id] = run
			dfs(idx+1, curCost+costOf(run, req.Row, req.Col))
			delete(assign, id)
			for _, st := range run {
				delete(used, st)
			}
		}
	}
	dfs(0, 0)
	return best
}

// AllocateBatch 原子地分配一批订单；任何失败都不留下部分占座。
func (s *Store) AllocateBatch(reqs []Request) ([]*Order, error) {
	if s.Hall == nil {
		return nil, fmt.Errorf("影厅尚未初始化")
	}
	if len(reqs) == 0 || len(reqs) > 6 {
		return nil, fmt.Errorf("一批订单数量须为 1..6")
	}
	seen := map[string]bool{}
	for _, r := range reqs {
		if r.ID == "" {
			return nil, fmt.Errorf("订单编号不能为空")
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("批内订单编号冲突: %s", r.ID)
		}
		seen[r.ID] = true
		if _, exists := s.Orders[r.ID]; exists {
			return nil, fmt.Errorf("订单编号已存在: %s", r.ID)
		}
		if r.Size < 1 || r.Size > s.Hall.Cols {
			return nil, fmt.Errorf("订单 %s 人数非法: %d", r.ID, r.Size)
		}
	}
	assign := s.solve(reqs, s.occupied(""))
	if assign == nil {
		return nil, fmt.Errorf("整批无可行分配，已全部拒绝")
	}
	var out []*Order
	for _, r := range reqs {
		seats := assign[r.ID]
		o := &Order{ID: r.ID, Size: r.Size, Row: r.Row, Col: r.Col,
			Seats: seats, Cost: costOf(seats, r.Row, r.Col)}
		s.Orders[o.ID] = o
		out = append(out, o)
	}
	return out, nil
}

// Release 释放整单占座。
func (s *Store) Release(id string) error {
	if _, ok := s.Orders[id]; !ok {
		return fmt.Errorf("订单不存在: %s", id)
	}
	delete(s.Orders, id)
	return nil
}

// Reseat 整单换座：先求新分配，再一次替换；失败时原订单保持不变。
func (s *Store) Reseat(id string, size, row, col int) (*Order, error) {
	o, ok := s.Orders[id]
	if !ok {
		return nil, fmt.Errorf("订单不存在: %s", id)
	}
	if size < 1 || size > s.Hall.Cols {
		return nil, fmt.Errorf("人数非法: %d", size)
	}
	req := Request{ID: id, Size: size, Row: row, Col: col}
	assign := s.solve([]Request{req}, s.occupied(id))
	if assign == nil {
		return nil, fmt.Errorf("换座无可行方案，原座位保持不变")
	}
	seats := assign[id]
	o.Size = size
	o.Row = row
	o.Col = col
	o.Seats = seats
	o.Cost = costOf(seats, row, col)
	return o, nil
}
