package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCSV 把行写入临时 CSV 文件。
func writeCSV(t *testing.T, dir, name string, rows [][]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeRules 把规则写入临时 JSON 文件。
func writeRules(t *testing.T, dir string, r *Rules) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readCSV 读取 CSV 全部行。
func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// runCLI 执行一次完整排序，返回输出内容与统计。
func runCLI(t *testing.T, dir string, rules *Rules, inputs []string) ([][]string, Stats) {
	t.Helper()
	rulesPath := writeRules(t, dir, rules)
	out := filepath.Join(dir, "out.csv")
	statsPath := filepath.Join(dir, "stats.json")
	args := []string{"-rules", rulesPath, "-out", out, "-stats", statsPath}
	args = append(args, inputs...)
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	var stats Stats
	data, err := os.ReadFile(statsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		t.Fatal(err)
	}
	return readCSV(t, out), stats
}

// TestCrossChunkEqualKeys 相等键跨文件、跨分块时保持原始次序。
func TestCrossChunkEqualKeys(t *testing.T) {
	dir := t.TempDir()
	header := []string{"key", "payload"}
	// 三个文件，key 大量重复；payload 记录原始位置。
	var files []string
	var want []string
	for fi := 0; fi < 3; fi++ {
		rows := [][]string{header}
		for ri := 0; ri < 7; ri++ {
			key := fmt.Sprintf("k%d", (fi+ri)%3) // 只有 3 种键
			payload := fmt.Sprintf("f%dr%d", fi, ri)
			rows = append(rows, []string{key, payload})
			want = append(want, key+","+payload)
		}
		files = append(files, writeCSV(t, dir, fmt.Sprintf("in%d.csv", fi), rows))
	}
	// 期望：按 key 稳定排序后的原始次序。
	sortWant := stableSortLines(want)

	rules := &Rules{
		Keys:          []KeyRule{{Column: "key", Type: "text", Order: "asc", Empty: "last"}},
		MemoryRecords: 2, // 强制大量分块
		MergeFanIn:    2, // 强制多轮归并
	}
	got, stats := runCLI(t, dir, rules, files)
	if stats.Read != 21 || stats.Kept != 21 || stats.Dropped != 0 {
		t.Fatalf("统计不符: %+v", stats)
	}
	var lines []string
	for _, row := range got[1:] {
		lines = append(lines, row[0]+","+row[1])
	}
	if strings.Join(lines, ";") != strings.Join(sortWant, ";") {
		t.Fatalf("跨块稳定性失败:\n got: %v\nwant: %v", lines, sortWant)
	}
}

// stableSortLines 用 Go 稳定排序生成期望结果。
func stableSortLines(lines []string) []string {
	out := append([]string(nil), lines...)
	// 简单插入排序保证稳定。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
