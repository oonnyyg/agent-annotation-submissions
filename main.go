package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/project-crash-planner/cpm"
)

func main() {
	input := flag.String("input", "", "项目定义 JSON 文件路径（必需）")
	csvPath := flag.String("csv", "", "CSV 对照计划输出路径（可选）")
	jsonPath := flag.String("json", "", "JSON 选择理由输出路径（可选，默认打印到标准输出）")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须通过 -input 指定项目 JSON 文件")
		flag.Usage()
		os.Exit(2)
	}

	p, err := cpm.LoadProject(*input)
	if err != nil {
		fatal(err)
	}
	result, err := cpm.EvaluateCrashing(p)
	if err != nil {
		fatal(err)
	}
	report := cpm.BuildReport(p, result)

	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		if err := report.WriteCSV(f); err != nil {
			fatal(err)
		}
	}

	printSummary(report)

	out := os.Stdout
	if *jsonPath != "" {
		f, err := os.Create(*jsonPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		out = f
	}
	if err := report.WriteJSON(out); err != nil {
		fatal(err)
	}
}

func printSummary(r *cpm.Report) {
	fmt.Printf("基准完工日: %d 天\n", r.ProjectFinishBaseline)
	fmt.Printf("最优完工日: %d 天（提前 %d 天）\n",
		r.ProjectFinishCrashed, r.DaysSaved)
	fmt.Printf("已选压缩: %v  成本: %d / 预算: %d\n",
		r.SelectedActivities, r.TotalCost, r.Budget)
	fmt.Printf("基准关键路径: %v\n", r.CriticalPathBaseline)
	fmt.Printf("压缩后关键路径: %v\n", r.CriticalPathCrashed)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "错误: %v\n", err)
	os.Exit(1)
}
