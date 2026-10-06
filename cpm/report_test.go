package cpm

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func sampleProject() *Project {
	return &Project{
		Budget: 12,
		Activities: []Activity{
			{ID: "start", Duration: 0},
			{ID: "design", Duration: 4, Predecessors: []string{"start"},
				Crash: &CrashOption{ShorterDuration: 2, Cost: 6}},
			{ID: "build", Duration: 6, Predecessors: []string{"design"},
				Crash: &CrashOption{ShorterDuration: 4, Cost: 6}},
			{ID: "docs", Duration: 3, Predecessors: []string{"start"},
				Crash: &CrashOption{ShorterDuration: 1, Cost: 2}},
			{ID: "done", Duration: 0, Predecessors: []string{"build", "docs"}},
		},
	}
}

func TestReportJSONAndCSV(t *testing.T) {
	p := sampleProject()
	r, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildReport(p, r)

	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON 输出不可解析: %v", err)
	}
	if decoded["project_finish_baseline"].(float64) != 10 {
		t.Fatalf("基准完工日应为 10, 实际 %v", decoded["project_finish_baseline"])
	}
	// 预算 12 可同时压 design+build：4+6=10 → 2+4=6，docs 分支为 3，故完工 6。
	if decoded["project_finish_crashed"].(float64) != 6 {
		t.Fatalf("压缩完工日应为 6, 实际 %v", decoded["project_finish_crashed"])
	}

	var cb bytes.Buffer
	if err := rep.WriteCSV(&cb); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(
		strings.TrimPrefix(cb.String(), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(p.Activities)+1 {
		t.Fatalf("CSV 行数 = %d, 期望 %d", len(rows), len(p.Activities)+1)
	}
	if rows[0][0] != "id" {
		t.Fatalf("CSV 表头首列 = %q, 期望 id", rows[0][0])
	}
}

// 18 项可压缩活动的枚举规模测试（2^18 组合）。
func TestEighteenCrashableActivities(t *testing.T) {
	acts := []Activity{{ID: "A0", Duration: 1,
		Crash: &CrashOption{ShorterDuration: 0, Cost: 1}}}
	for i := 1; i < 18; i++ {
		id := string(rune('A' + i))
		if i >= 26 {
			id = "A" + string(rune('0'+i-26))
		}
		acts = append(acts, Activity{ID: id, Duration: 1,
			Predecessors: []string{acts[i-1].ID},
			Crash:        &CrashOption{ShorterDuration: 0, Cost: 1}})
	}
	p := &Project{Budget: 18, Activities: acts}
	r, err := EvaluateCrashing(p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Combinations != 1<<18 {
		t.Fatalf("枚举组合数 = %d, 期望 %d", r.Combinations, 1<<18)
	}
	if r.CrashedFinish != 0 {
		t.Fatalf("全部压缩后为零工期链, 完工日应为 0, 实际 %d", r.CrashedFinish)
	}
}
