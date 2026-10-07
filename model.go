package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Trip 表示一个必须执行的班次。
type Trip struct {
	ID     string
	From   string
	To     string
	Depart time.Time
	Arrive time.Time
}

// DeadheadArc 表示站点间的有向空驶信息。
type DeadheadArc struct {
	From     string `json:"from"`
	To       string `json:"to"`
	TimeMin  int64  `json:"time_min"`
	Distance int64  `json:"distance_km"`
}

// Network 是 JSON 输入：统一整备时长与空驶弧表。
type Network struct {
	TurnaroundMin int64         `json:"turnaround_min"`
	Arcs          []DeadheadArc `json:"arcs"`
}

// Problem 是解析并校验后的完整输入。
type Problem struct {
	Trips      []Trip // 按 ID 排序，保证与输入顺序无关
	Turnaround time.Duration
	TravelTime map[[2]string]time.Duration
	TravelDist map[[2]string]int64
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

// parseTripsCSV 读取班次 CSV，表头：id,from,to,depart,arrive（UTC RFC3339）。
func parseTripsCSV(r io.Reader) ([]Trip, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("读取班次 CSV 失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("班次 CSV 为空")
	}
	header := rows[0]
	want := []string{"id", "from", "to", "depart", "arrive"}
	if len(header) != len(want) {
		return nil, fmt.Errorf("CSV 表头应为 %v，实际为 %v", want, header)
	}
	for i := range want {
		if strings.ToLower(strings.TrimSpace(header[i])) != want[i] {
			return nil, fmt.Errorf("CSV 表头第 %d 列应为 %q，实际为 %q", i+1, want[i], header[i])
		}
	}
	seen := map[string]bool{}
	var trips []Trip
	for line, row := range rows[1:] {
		if len(row) != len(want) {
			return nil, fmt.Errorf("第 %d 行列数错误", line+2)
		}
		id := strings.TrimSpace(row[0])
		if id == "" {
			return nil, fmt.Errorf("第 %d 行班次编号为空", line+2)
		}
		if seen[id] {
			return nil, fmt.Errorf("班次编号 %q 重复", id)
		}
		seen[id] = true
		dep, err := time.Parse(timeLayout, strings.TrimSpace(row[3]))
		if err != nil {
			return nil, fmt.Errorf("班次 %q 出发时刻非法: %w", id, err)
		}
		arr, err := time.Parse(timeLayout, strings.TrimSpace(row[4]))
		if err != nil {
			return nil, fmt.Errorf("班次 %q 到达时刻非法: %w", id, err)
		}
		if !arr.After(dep) {
			return nil, fmt.Errorf("班次 %q 持续时间必须为正（到达必须晚于出发）", id)
		}
		from, to := strings.TrimSpace(row[1]), strings.TrimSpace(row[2])
		if from == "" || to == "" {
			return nil, fmt.Errorf("班次 %q 起终站不能为空", id)
		}
		trips = append(trips, Trip{ID: id, From: from, To: to, Depart: dep.UTC(), Arrive: arr.UTC()})
	}
	if len(trips) == 0 {
		return nil, fmt.Errorf("班次 CSV 不含任何班次")
	}
	if len(trips) > 120 {
		return nil, fmt.Errorf("班次数 %d 超过上限 120", len(trips))
	}
	// 固定按编号排序，保证输入重排后结果一致。
	sort.Slice(trips, func(i, j int) bool { return trips[i].ID < trips[j].ID })
	return trips, nil
}

// parseNetworkJSON 读取空驶网络 JSON。
func parseNetworkJSON(r io.Reader) (*Network, error) {
	var n Network
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("解析空驶 JSON 失败: %w", err)
	}
	if n.TurnaroundMin < 0 {
		return nil, fmt.Errorf("最短整备时长不能为负")
	}
	seen := map[[2]string]bool{}
	for _, a := range n.Arcs {
		if a.From == "" || a.To == "" {
			return nil, fmt.Errorf("空驶弧站点不能为空")
		}
		if a.From == a.To {
			return nil, fmt.Errorf("空驶弧 %q->%q 起终站相同（同站空驶恒为零，无需提供）", a.From, a.To)
		}
		if a.TimeMin < 0 || a.Distance < 0 {
			return nil, fmt.Errorf("空驶弧 %q->%q 的时间与里程必须非负", a.From, a.To)
		}
		key := [2]string{a.From, a.To}
		if seen[key] {
			return nil, fmt.Errorf("空驶弧 %q->%q 重复定义", a.From, a.To)
		}
		seen[key] = true
	}
	return &n, nil
}

// buildProblem 合并校验后的输入。
func buildProblem(trips []Trip, net *Network) *Problem {
	// 固定按编号排序，保证输入重排后结果一致。
	sort.Slice(trips, func(i, j int) bool { return trips[i].ID < trips[j].ID })
	p := &Problem{
		Trips:      trips,
		Turnaround: time.Duration(net.TurnaroundMin) * time.Minute,
		TravelTime: map[[2]string]time.Duration{},
		TravelDist: map[[2]string]int64{},
	}
	for _, a := range net.Arcs {
		key := [2]string{a.From, a.To}
		p.TravelTime[key] = time.Duration(a.TimeMin) * time.Minute
		p.TravelDist[key] = a.Distance
	}
	return p
}

// deadhead 返回 from->to 的空驶时间与里程；同站为零，未提供则不可达。
func (p *Problem) deadhead(from, to string) (time.Duration, int64, bool) {
	if from == to {
		return 0, 0, true
	}
	key := [2]string{from, to}
	t, okT := p.TravelTime[key]
	d, okD := p.TravelDist[key]
	if !okT || !okD {
		return 0, 0, false
	}
	return t, d, true
}

// formatDuration 以 "XhYYm" 形式输出时长。
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "-" + formatDuration(-d)
	}
	total := int64(d / time.Minute)
	return fmt.Sprintf("%dh%02dm", total/60, total%60)
}
