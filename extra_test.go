package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmptyFieldPosition 空字段按独立位置排序，不参与数值转换。
func TestEmptyFieldPosition(t *testing.T) {
	dir := t.TempDir()
	header := []string{"n", "v"}
	in := writeCSV(t, dir, "in.csv", [][]string{
		header,
		{"5", "a"},
		{"", "empty1"},
		{"3", "b"},
		{"", "empty2"},
	})
	// empty=first：空值排最前，且保持原始相对次序。
	rules := &Rules{
		Keys:          []KeyRule{{Column: "n", Type: "decimal", Order: "asc", Empty: "first"}},
		MemoryRecords: 2,
		MergeFanIn:    2,
	}
	got, _ := runCLI(t, dir, rules, []string{in})
	want := [][]string{header, {"", "empty1"}, {"", "empty2"}, {"3", "b"}, {"5", "a"}}
	assertRows(t, got, want)

	// empty=last + desc：空值仍排最后。
	rules.Keys[0].Empty = "last"
	rules.Keys[0].Order = "desc"
	got, _ = runCLI(t, dir, rules, []string{in})
	want = [][]string{header, {"5", "a"}, {"3", "b"}, {"", "empty1"}, {"", "empty2"}}
	assertRows(t, got, want)
}

// TestDecimalNotString 十进制按数值比较，不退化为字符串排序。
func TestDecimalNotString(t *testing.T) {
	dir := t.TempDir()
	header := []string{"n"}
	in := writeCSV(t, dir, "in.csv", [][]string{
		header, {"10"}, {"9"}, {"2.5"}, {"-1"}, {"0.10"}, {"1e2"},
	})
	rules := &Rules{
		Keys:          []KeyRule{{Column: "n", Type: "decimal", Order: "asc", Empty: "first"}},
		MemoryRecords: 3,
		MergeFanIn:    2,
	}
	got, _ := runCLI(t, dir, rules, []string{in})
	want := [][]string{header, {"-1"}, {"0.10"}, {"2.5"}, {"9"}, {"10"}, {"1e2"}}
	assertRows(t, got, want)
}

// TestDedup 按完整排序键去重，保留原始次序最早的一条。
func TestDedup(t *testing.T) {
	dir := t.TempDir()
	header := []string{"k", "v"}
	f1 := writeCSV(t, dir, "a.csv", [][]string{
		header, {"x", "first"}, {"y", "y1"}, {"x", "second"},
	})
	f2 := writeCSV(t, dir, "b.csv", [][]string{
		header, {"x", "third"}, {"y", "y2"},
	})
	rules := &Rules{
		Keys:          []KeyRule{{Column: "k", Type: "text", Order: "asc", Empty: "first"}},
		Dedup:         true,
		MemoryRecords: 2,
		MergeFanIn:    2,
	}
	got, stats := runCLI(t, dir, rules, []string{f1, f2})
	want := [][]string{header, {"x", "first"}, {"y", "y1"}}
	assertRows(t, got, want)
	if stats.Read != 5 || stats.Kept != 2 || stats.Dropped != 3 {
		t.Fatalf("统计不符: %+v", stats)
	}
}

// TestDedupMultiKey 多列键全等才去重。
func TestDedupMultiKey(t *testing.T) {
	dir := t.TempDir()
	header := []string{"a", "b", "v"}
	in := writeCSV(t, dir, "in.csv", [][]string{
		header,
		{"1", "x", "keep"},
		{"1", "y", "diff-second-key"},
		{"1.0", "x", "dup-numeric-equal"},
	})
	rules := &Rules{
		Keys: []KeyRule{
			{Column: "a", Type: "decimal", Order: "asc", Empty: "first"},
			{Column: "b", Type: "text", Order: "asc", Empty: "first"},
		},
		Dedup:         true,
		MemoryRecords: 10,
		MergeFanIn:    2,
	}
	got, stats := runCLI(t, dir, rules, []string{in})
	want := [][]string{header, {"1", "x", "keep"}, {"1", "y", "diff-second-key"}}
	assertRows(t, got, want)
	if stats.Dropped != 1 {
		t.Fatalf("统计不符: %+v", stats)
	}
}

func assertRows(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("行数不符: got %d want %d\ngot: %v", len(got), len(want), got)
	}
	for i := range want {
		if strings.Join(got[i], ",") != strings.Join(want[i], ",") {
			t.Fatalf("第 %d 行不符: got %v want %v", i, got[i], want[i])
		}
	}
}

// TestFailureKeepsOldOutput 失败时保留旧输出并清理临时文件。
func TestFailureKeepsOldOutput(t *testing.T) {
	dir := t.TempDir()
	header := []string{"n"}
	good := writeCSV(t, dir, "good.csv", [][]string{header, {"1"}})
	bad := writeCSV(t, dir, "bad.csv", [][]string{header, {"abc"}})
	out := filepath.Join(dir, "out.csv")
	if err := os.WriteFile(out, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := &Rules{
		Keys:          []KeyRule{{Column: "n", Type: "decimal", Order: "asc", Empty: "first"}},
		MemoryRecords: 10,
		MergeFanIn:    2,
	}
	rulesPath := writeRules(t, dir, rules)
	err := run([]string{"-rules", rulesPath, "-out", out, good, bad})
	if err == nil || !strings.Contains(err.Error(), "bad.csv") || !strings.Contains(err.Error(), "第 1 条记录") {
		t.Fatalf("错误应定位文件与记录号: %v", err)
	}
	data, _ := os.ReadFile(out)
	if string(data) != "old\n" {
		t.Fatalf("失败后旧输出被修改: %q", data)
	}
	// 临时目录应已清理。
	matches, _ := filepath.Glob(filepath.Join(dir, ".sort-tmp-*"))
	if len(matches) != 0 {
		t.Fatalf("临时目录未清理: %v", matches)
	}
}
