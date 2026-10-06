package cpm

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// ActivityRow 是 CSV 计划中单个活动的对照行。
type ActivityRow struct {
	ID           string   `json:"id"`
	Predecessors []string `json:"predecessors"`
	BaseDuration int      `json:"base_duration"`
	NewDuration  int      `json:"new_duration"`
	BaseES       int      `json:"base_es"`
	BaseEF       int      `json:"base_ef"`
	BaseLS       int      `json:"base_ls"`
	BaseLF       int      `json:"base_lf"`
	BaseFloat    int      `json:"base_total_float"`
	BaseCritical bool     `json:"base_critical"`
	NewES        int      `json:"new_es"`
	NewEF        int      `json:"new_ef"`
	NewLS        int      `json:"new_ls"`
	NewLF        int      `json:"new_lf"`
	NewFloat     int      `json:"new_total_float"`
	NewCritical  bool     `json:"new_critical"`
	Crashed      bool     `json:"crashed"`
}

// Report 是 JSON“选择理由”输出结构。
type Report struct {
	Budget                int           `json:"budget"`
	SelectedActivities    []string      `json:"selected_activities"`
	TotalCost             int           `json:"total_cost"`
	ProjectFinishBaseline int           `json:"project_finish_baseline"`
	ProjectFinishCrashed  int           `json:"project_finish_crashed"`
	DaysSaved             int           `json:"days_saved"`
	CriticalPathBaseline  []string      `json:"critical_path_baseline"`
	CriticalPathCrashed   []string      `json:"critical_path_crashed"`
	CombinationsTotal     int           `json:"combinations_total"`
	CombinationsFeasible  int           `json:"combinations_feasible_within_budget"`
	SelectionRule         string        `json:"selection_rule"`
	Activities            []ActivityRow `json:"activities"`
}

const selectionRule = "在预算内枚举全部压缩组合，优先整体完工日最早；" +
	"同完工日取总成本最低；再相同时取排序后已选活动标识序列字典序最小"

// BuildReport 汇总基准与最优方案的对照数据。
func BuildReport(p *Project, r *CrashResult) *Report {
	selectedSet := make(map[string]bool, len(r.Selected))
	for _, id := range r.Selected {
		selectedSet[id] = true
	}
	rows := make([]ActivityRow, 0, len(p.Activities))
	for _, a := range p.Activities {
		b := r.BaseSchedule.Results[a.ID]
		c := r.CrashedSchedule.Results[a.ID]
		preds := append([]string{}, a.Predecessors...)
		rows = append(rows, ActivityRow{
			ID:           a.ID,
			Predecessors: preds,
			BaseDuration: b.Duration,
			NewDuration:  c.Duration,
			BaseES:       b.EarlyStart,
			BaseEF:       b.EarlyFinish,
			BaseLS:       b.LateStart,
			BaseLF:       b.LateFinish,
			BaseFloat:    b.TotalFloat,
			BaseCritical: b.Critical,
			NewES:        c.EarlyStart,
			NewEF:        c.EarlyFinish,
			NewLS:        c.LateStart,
			NewLF:        c.LateFinish,
			NewFloat:     c.TotalFloat,
			NewCritical:  c.Critical,
			Crashed:      selectedSet[a.ID],
		})
	}
	return &Report{
		Budget:                p.Budget,
		SelectedActivities:    append([]string{}, r.Selected...),
		TotalCost:             r.TotalCost,
		ProjectFinishBaseline: r.BaseFinish,
		ProjectFinishCrashed:  r.CrashedFinish,
		DaysSaved:             r.BaseFinish - r.CrashedFinish,
		CriticalPathBaseline:  append([]string{}, r.BaseSchedule.CriticalPath...),
		CriticalPathCrashed:   append([]string{}, r.CrashedSchedule.CriticalPath...),
		CombinationsTotal:     r.Combinations,
		CombinationsFeasible:  r.FeasibleChecked,
		SelectionRule:         selectionRule,
		Activities:            rows,
	}
}

// WriteJSON 将选择理由以缩进 JSON 写出。
func (rep *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(rep)
}

var csvHeader = []string{
	"id", "predecessors", "base_duration", "new_duration", "crashed",
	"base_es", "base_ef", "base_ls", "base_lf", "base_total_float", "base_critical",
	"new_es", "new_ef", "new_ls", "new_lf", "new_total_float", "new_critical",
}

// WriteCSV 将基准/方案对照计划以 CSV 写出（含 UTF-8 BOM 便于 Excel 打开）。
func (rep *Report) WriteCSV(w io.Writer) error {
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, a := range rep.Activities {
		row := []string{
			a.ID,
			joinComma(a.Predecessors),
			strconv.Itoa(a.BaseDuration),
			strconv.Itoa(a.NewDuration),
			strconv.FormatBool(a.Crashed),
			strconv.Itoa(a.BaseES),
			strconv.Itoa(a.BaseEF),
			strconv.Itoa(a.BaseLS),
			strconv.Itoa(a.BaseLF),
			strconv.Itoa(a.BaseFloat),
			strconv.FormatBool(a.BaseCritical),
			strconv.Itoa(a.NewES),
			strconv.Itoa(a.NewEF),
			strconv.Itoa(a.NewLS),
			strconv.Itoa(a.NewLF),
			strconv.Itoa(a.NewFloat),
			strconv.FormatBool(a.NewCritical),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("写出 CSV: %w", err)
	}
	return nil
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ";"
		}
		out += s
	}
	return out
}
