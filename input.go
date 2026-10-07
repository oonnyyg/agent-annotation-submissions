package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const timeLayout = time.RFC3339

type deadheadJSON struct {
	From             string `json:"from"`
	To               string `json:"to"`
	DurationMinutes  int    `json:"duration_minutes"`
	DistanceKm       int    `json:"distance_km"`
}

type configJSON struct {
	MinPrepMinutes int             `json:"min_prep_minutes"`
	Deadheads      []deadheadJSON  `json:"deadheads"`
}

// loadInput 读取并校验 CSV 班次与 JSON 空驶配置。
func loadInput(csvPath, jsonPath string) (*Input, error) {
	trips, err := loadTrips(csvPath)
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(jsonPath)
	if err != nil {
	return nil, err
	}

	if cfg.MinPrepMinutes < 0 {
		return nil, fmt.Errorf("min_prep_minutes 不能为负: %d", cfg.MinPrepMinutes)
	}
	dh := make(map[[2]string]DeadheadLeg, len(cfg.Deadheads))
	for i, d := range cfg.Deadheads {
		if strings.TrimSpace(d.From) == "" || strings.TrimSpace(d.To) == "" {
			return nil, fmt.Errorf("空驶记录 #%d 缺少站点", i+1)
		}
		if d.DurationMinutes < 0 {
			return nil, fmt.Errorf("空驶 %s->%s 时长不能为负", d.From, d.To)
		}
		if d.DistanceKm < 0 {
			return nil, fmt.Errorf("空驶 %s->%s 里程不能为负", d.From, d.To)
		}
		key := [2]string{d.From, d.To}
		if _, dup := dh[key]; dup {
			return nil, fmt.Errorf("空驶记录重复: %s->%s", d.From, d.To)
		}
		dh[key] = DeadheadLeg{
			From:     d.From,
			To:       d.To,
			Duration: time.Duration(d.DurationMinutes) * time.Minute,
			Distance: d.DistanceKm,
		}
	}

	return &Input{
		Trips:    trips,
		Deadhead: dh,
		MinPrep:  time.Duration(cfg.MinPrepMinutes) * time.Minute,
	}, nil
}

func loadTrips(path string) ([]Trip, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开班次 CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("读取班次 CSV 表头: %w", err)
	}
	cols, err := mapHeader(header, []string{"trip_id", "from", "to", "start", "end"})
	if err != nil {
		return nil, err
	}

	var trips []Trip
	seenID := make(map[string]bool)
	lineNo := 1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		if err != nil {
			return nil, fmt.Errorf("班次 CSV 第 %d 行: %w", lineNo, err)
		}
		if len(rec) != len(cols) {
			return nil, fmt.Errorf("班次 CSV 第 %d 行: 列数不匹配", lineNo)
		}
		id := strings.TrimSpace(rec[cols["trip_id"]])
		if id == "" {
			return nil, fmt.Errorf("班次 CSV 第 %d 行: trip_id 为空", lineNo)
		}
		if seenID[id] {
			return nil, fmt.Errorf("班次 ID 重复: %s", id)
		}
		from := strings.TrimSpace(rec[cols["from"]])
		to := strings.TrimSpace(rec[cols["to"]])
		if from == "" || to == "" {
			return nil, fmt.Errorf("班次 CSV 第 %d 行: 起终站为空", lineNo)
		}
		start, err := parseUTC(rec[cols["start"]])
		if err != nil {
			return nil, fmt.Errorf("班次 %s 的 start: %w", id, err)
		}
		end, err := parseUTC(rec[cols["end"]])
		if err != nil {
			return nil, fmt.Errorf("班次 %s 的 end: %w", id, err)
		}
		if !end.After(start) {
			return nil, fmt.Errorf("班次 %s 持续时间必须为正", id)
		}
		seenID[id] = true
		trips = append(trips, Trip{
			ID:    id,
			From:  from,
			To:    to,
			Start: start,
			End:   end,
			Index: len(trips),
		})
	}
	if len(trips) == 0 {
		return nil, fmt.Errorf("班次 CSV 不包含任何班次")
	}
	return trips, nil
}

func loadConfig(path string) (*configJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 JSON: %w", err)
	}
	var cfg configJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置 JSON: %w", err)
	}
	return &cfg, nil
}

func parseUTC(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func mapHeader(header []string, want []string) (map[string]int, error) {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	for _, w := range want {
		if _, ok := m[w]; !ok {
			return nil, fmt.Errorf("CSV 缺少列: %s（需要 %s）", w, strings.Join(want, ", "))
		}
	}
	return m, nil
}

// sortedTripIDs 返回按输入稳定顺序排列的班次 ID（输入顺序本身即稳定序号）。
func (in *Input) sortedTripIDs() []string {
	ids := make([]string, len(in.Trips))
	for i := range in.Trips {
		ids[i] = in.Trips[i].ID
	}
	sort.SliceStable(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}

// itoa 避免不必要的 strconv 引用，仅用于潜在调试。
var _ = strconv.Itoa
