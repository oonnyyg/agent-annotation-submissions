package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	input := flag.String("input", "", "项目网络 JSON 文件路径 (必填)")
	csvPrefix := flag.String("csv-prefix", "", "若提供, 输出 <prefix>_baseline.csv 与 <prefix>_plan.csv")
	jsonOut := flag.String("json", "", "若提供, 将选择理由 JSON 写入该文件, 否则写到标准输出")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "用法: crash-planner -input project.json [-csv-prefix out] [-json report.json]")
		os.Exit(2)
	}
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开输入失败:", err)
		os.Exit(1)
	}
	defer f.Close()
	p, err := loadProject(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "输入无效:", err)
		os.Exit(1)
	}

	base := computeCPM(p, baseDurations(p))
	basePath := criticalPath(p, base)
	plan := searchPlans(p)

	writeSummary(os.Stdout, p, base, basePath, plan)

	if *csvPrefix != "" {
		if err := writeCSVFile(*csvPrefix+"_baseline.csv", p, base, map[string]bool{}, baseDurations(p)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := writeCSVFile(*csvPrefix+"_plan.csv", p, plan.Schedule, selectedSet(plan.Selected), durationsFor(p, selectedSet(plan.Selected))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var jw *os.File
	if *jsonOut != "" {
		if dir := filepath.Dir(*jsonOut); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintln(os.Stderr, "创建目录失败:", err)
				os.Exit(1)
			}
		}
		jw, err = os.Create(*jsonOut)
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建 JSON 输出失败:", err)
			os.Exit(1)
		}
		defer jw.Close()
	} else {
		jw = os.Stdout
	}
	writeJSON(jw, p, base, basePath, plan)
}

func writeCSVFile(path string, p *Project, s *Schedule, selected map[string]bool, dur map[string]int) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建 CSV 失败: %w", err)
	}
	defer f.Close()
	writeCSV(f, p, s, selected, dur)
	return nil
}
