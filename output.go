package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// rowJSON is one activity line in the JSON rationale report.
type rowJSON struct {
	ID       string `json:"id"`
	Duration int    `json:"duration"`
	Crashed  bool   `json:"crashed"`
	ES       int    `json:"es"`
	EF       int    `json:"ef"`
	LS       int    `json:"ls"`
	LF       int    `json:"lf"`
	Float    int    `json:"float"`
	Critical bool   `json:"critical"`
}

type scenarioJSON struct {
	Finish       int       `json:"finish"`
	Cost         int       `json:"cost"`
	Selected     []string  `json:"selected"`
	CriticalPath []string  `json:"criticalPath"`
	Rows         []rowJSON `json:"rows"`
}

type reportJSON struct {
	Budget    int          `json:"budget"`
	Baseline  scenarioJSON `json:"baseline"`
	Plan      scenarioJSON `json:"plan"`
	Rationale string       `json:"rationale"`
}

func buildRows(p *Project, s *Schedule, selected map[string]bool, dur map[string]int) []rowJSON {
	ids := p.sortedIDs()
	rows := make([]rowJSON, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, rowJSON{
			ID:       id,
			Duration: dur[id],
			Crashed:  selected[id],
			ES:       s.ES[id],
			EF:       s.EF[id],
			LS:       s.LS[id],
			LF:       s.LF[id],
			Float:    s.Float[id],
			Critical: s.Critical(id),
		})
	}
	return rows
}

func selectedSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// writeCSV writes the schedule of one scenario as CSV.
func writeCSV(w io.Writer, p *Project, s *Schedule, selected map[string]bool, dur map[string]int) {
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "duration", "crashed", "es", "ef", "ls", "lf", "float", "critical"})
	for _, r := range buildRows(p, s, selected, dur) {
		cw.Write([]string{
			r.ID,
			fmt.Sprintf("%d", r.Duration),
			fmt.Sprintf("%t", r.Crashed),
			fmt.Sprintf("%d", r.ES),
			fmt.Sprintf("%d", r.EF),
			fmt.Sprintf("%d", r.LS),
			fmt.Sprintf("%d", r.LF),
			fmt.Sprintf("%d", r.Float),
			fmt.Sprintf("%t", r.Critical),
		})
	}
	cw.Flush()
}

// writeJSON writes the full baseline-vs-plan rationale report.
func writeJSON(w io.Writer, p *Project, base *Schedule, basePath []string, plan *Plan) {
	baseDur := baseDurations(p)
	planDur := durationsFor(p, selectedSet(plan.Selected))
	sel := plan.Selected
	if sel == nil {
		sel = []string{}
	}
	report := reportJSON{
		Budget: p.Budget,
		Baseline: scenarioJSON{
			Finish:       base.Finish,
			Cost:         0,
			Selected:     []string{},
			CriticalPath: basePath,
			Rows:         buildRows(p, base, map[string]bool{}, baseDur),
		},
		Plan: scenarioJSON{
			Finish:       plan.Finish,
			Cost:         plan.Cost,
			Selected:     sel,
			CriticalPath: plan.Path,
			Rows:         buildRows(p, plan.Schedule, selectedSet(plan.Selected), planDur),
		},
		Rationale: rationaleText(p, base, plan),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(report)
}

func rationaleText(p *Project, base *Schedule, plan *Plan) string {
	var b strings.Builder
	if len(plan.Selected) == 0 {
		fmt.Fprintf(&b, "预算 %d 下无可行压缩能提前完工, 保持基准完工日 %d 天, 成本 0。",
			p.Budget, base.Finish)
		return b.String()
	}
	fmt.Fprintf(&b, "在预算 %d 内穷举全部压缩组合, 选择 %s (总成本 %d), 完工日由 %d 天提前到 %d 天。",
		p.Budget, strings.Join(plan.Selected, ", "), plan.Cost, base.Finish, plan.Finish)
	fmt.Fprintf(&b, " 同完工日组合中本方案成本最低; 若仍并列, 已选标识序列字典序最小。")
	if !equalIDs(plan.Path, nil) {
		fmt.Fprintf(&b, " 压缩后关键路径为 %s。", strings.Join(plan.Path, " -> "))
	}
	return b.String()
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeSummary prints a human-readable CLI summary.
func writeSummary(w io.Writer, p *Project, base *Schedule, basePath []string, plan *Plan) {
	fmt.Fprintf(w, "基准完工日: %d 天\n", base.Finish)
	fmt.Fprintf(w, "基准关键路径: %s\n", strings.Join(basePath, " -> "))
	fmt.Fprintf(w, "预算: %d, 方案成本: %d, 压缩后完工日: %d 天\n", p.Budget, plan.Cost, plan.Finish)
	if len(plan.Selected) == 0 {
		fmt.Fprintln(w, "未选择任何压缩项")
	} else {
		fmt.Fprintf(w, "选择压缩: %s\n", strings.Join(plan.Selected, ", "))
	}
	fmt.Fprintf(w, "压缩后关键路径: %s\n", strings.Join(plan.Path, " -> "))
	var crit []string
	for id := range plan.Schedule.Float {
		if plan.Schedule.Critical(id) {
			crit = append(crit, id)
		}
	}
	sort.Strings(crit)
	fmt.Fprintf(w, "压缩后关键活动: %s\n", strings.Join(crit, ", "))
}
