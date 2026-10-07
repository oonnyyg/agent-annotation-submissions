package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBufferFanInEquivalence 不同内存块大小与归并扇入产生相同输出。
func TestBufferFanInEquivalence(t *testing.T) {
	dir := t.TempDir()
	header := []string{"k", "n", "v"}
	rng := rand.New(rand.NewSource(42))
	var files []string
	var total int
	for fi := 0; fi < 4; fi++ {
		rows := [][]string{header}
		for ri := 0; ri < 50; ri++ {
			rows = append(rows, []string{
				fmt.Sprintf("key%d", rng.Intn(10)),
				fmt.Sprintf("%d", rng.Intn(100)-50),
				fmt.Sprintf("f%dr%d", fi, ri),
			})
			total++
		}
		files = append(files, writeCSV(t, dir, fmt.Sprintf("in%d.csv", fi), rows))
	}

	var reference string
	for _, mem := range []int{1, 3, 7, 1000} {
		for _, fan := range []int{2, 3, 16} {
			sub := t.TempDir()
			rules := &Rules{
				Keys: []KeyRule{
					{Column: "k", Type: "text", Order: "asc", Empty: "first"},
					{Column: "n", Type: "decimal", Order: "desc", Empty: "last"},
				},
				Dedup:         false,
				MemoryRecords: mem,
				MergeFanIn:    fan,
			}
			got, stats := runCLI(t, sub, rules, files)
			if stats.Read != int64(total) || stats.Kept != int64(total) {
				t.Fatalf("mem=%d fan=%d 统计不符: %+v", mem, fan, stats)
			}
			var b strings.Builder
			for _, row := range got {
				b.WriteString(strings.Join(row, ","))
				b.WriteByte('\n')
			}
			if reference == "" {
				reference = b.String()
			} else if b.String() != reference {
				t.Fatalf("mem=%d fan=%d 输出与参考不一致", mem, fan)
			}
		}
	}
}

// TestQuotedCSV 正确读取带引号及换行的 CSV 记录。
func TestQuotedCSV(t *testing.T) {
	dir := t.TempDir()
	header := []string{"k", "note"}
	in := filepath.Join(dir, "in.csv")
	content := "k,note\n" +
		"b,\"line1\nline2\"\n" +
		"a,\"say \"\"hi\"\"\"\n" +
		"c,plain\n"
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := &Rules{
		Keys:          []KeyRule{{Column: "k", Type: "text", Order: "asc", Empty: "first"}},
		MemoryRecords: 2,
		MergeFanIn:    2,
	}
	got, stats := runCLI(t, dir, rules, []string{in})
	if stats.Read != 3 {
		t.Fatalf("应读取 3 条逻辑记录: %+v", stats)
	}
	want := [][]string{
		header,
		{"a", "say \"hi\""},
		{"b", "line1\nline2"},
		{"c", "plain"},
	}
	assertRows(t, got, want)
}

// TestHeaderMismatch 表头不一致时报错。
func TestHeaderMismatch(t *testing.T) {
	dir := t.TempDir()
	f1 := writeCSV(t, dir, "a.csv", [][]string{{"x", "y"}, {"1", "2"}})
	f2 := writeCSV(t, dir, "b.csv", [][]string{{"x", "z"}, {"1", "2"}})
	rules := &Rules{
		Keys:          []KeyRule{{Column: "x", Type: "text", Order: "asc", Empty: "first"}},
		MemoryRecords: 10,
		MergeFanIn:    2,
	}
	rulesPath := writeRules(t, dir, rules)
	err := run([]string{"-rules", rulesPath, "-out", filepath.Join(dir, "o.csv"), f1, f2})
	if err == nil || !strings.Contains(err.Error(), "表头不一致") {
		t.Fatalf("应报告表头不一致: %v", err)
	}
}

// TestRulesValidation 非法资源限制被拒绝。
func TestRulesValidation(t *testing.T) {
	base := Rules{Keys: []KeyRule{{Column: "x", Type: "text", Order: "asc", Empty: "first"}}}
	for _, r := range []Rules{
		{Keys: base.Keys, MemoryRecords: 0, MergeFanIn: 2},
		{Keys: base.Keys, MemoryRecords: 1, MergeFanIn: 1},
		{MemoryRecords: 1, MergeFanIn: 2},
		{Keys: []KeyRule{{Column: "x", Type: "int", Order: "asc", Empty: "first"}}, MemoryRecords: 1, MergeFanIn: 2},
	} {
		if err := r.validate(); err == nil {
			t.Fatalf("应拒绝非法规则: %+v", r)
		}
	}
}
