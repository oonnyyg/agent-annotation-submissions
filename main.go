package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "anonymize":
		err = cmdAnonymize(os.Args[2:])
	case "check":
		err = cmdCheck(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `用法:
  anonymizer anonymize -csv 数据.csv -hier 层级.json -config 配置.json -public 公开.csv -report 报告.json
  anonymizer check     -csv 数据.csv -hier 层级.json -config 配置.json -steps 1,0,... [-report 报告.json]`)
}

func loadInputs(csvPath, hierPath, configPath string) (*Dataset, map[string]*Hierarchy, *Config, error) {
	hier, err := LoadHierarchies(hierPath)
	if err != nil {
		return nil, nil, nil, err
	}
	ds, err := LoadDataset(csvPath, hier)
	if err != nil {
		return nil, nil, nil, err
	}
	cfg, err := LoadConfig(configPath, ds.QICols)
	if err != nil {
		return nil, nil, nil, err
	}
	return ds, hier, cfg, nil
}

func cmdAnonymize(args []string) error {
	fs := flag.NewFlagSet("anonymize", flag.ContinueOnError)
	csvPath := fs.String("csv", "", "输入 CSV 文件")
	hierPath := fs.String("hier", "", "层级 JSON 文件")
	configPath := fs.String("config", "", "配置 JSON 文件")
	publicPath := fs.String("public", "", "公开 CSV 输出路径")
	reportPath := fs.String("report", "", "内部核查报告 JSON 输出路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *csvPath == "" || *hierPath == "" || *configPath == "" || *publicPath == "" || *reportPath == "" {
		return fmt.Errorf("anonymize 需要 -csv -hier -config -public -report 全部参数")
	}
	ds, hier, cfg, err := loadInputs(*csvPath, *hierPath, *configPath)
	if err != nil {
		return err
	}
	best, err := SearchBest(ds, hier, cfg)
	if err != nil {
		return err
	}
	if err := writePublicCSV(*publicPath, ds, hier, best); err != nil {
		return err
	}
	if err := writeReport(*reportPath, "anonymize", ds, cfg, best); err != nil {
		return err
	}
	fmt.Printf("最优步数 %v，抑制 %d 条，总成本 %d\n", best.Steps, best.Suppressed, best.TotalCost)
	return nil
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	csvPath := fs.String("csv", "", "输入 CSV 文件")
	hierPath := fs.String("hier", "", "层级 JSON 文件")
	configPath := fs.String("config", "", "配置 JSON 文件")
	stepsStr := fs.String("steps", "", "步数向量，如 1,0,2（按 CSV 准标识列顺序）")
	reportPath := fs.String("report", "", "可选：核查报告 JSON 输出路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *csvPath == "" || *hierPath == "" || *configPath == "" || *stepsStr == "" {
		return fmt.Errorf("check 需要 -csv -hier -config -steps 参数")
	}
	ds, hier, cfg, err := loadInputs(*csvPath, *hierPath, *configPath)
	if err != nil {
		return err
	}
	parts := strings.Split(*stepsStr, ",")
	steps := make([]int, len(parts))
	for i, p := range parts {
		s, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return fmt.Errorf("步数 %q 不是整数", p)
		}
		steps[i] = s
	}
	ev, err := Evaluate(ds, hier, cfg, steps)
	if err != nil {
		return err
	}
	if *reportPath != "" {
		if err := writeReport(*reportPath, "check", ds, cfg, ev); err != nil {
			return err
		}
	}
	if ev.Feasible {
		fmt.Printf("方案可行：步数 %v，抑制 %d 条，总成本 %d\n", ev.Steps, ev.Suppressed, ev.TotalCost)
		return nil
	}
	return fmt.Errorf("方案不可行: %s", ev.Reason)
}

// writePublicCSV 输出公开数据：仅泛化后的准标识列与敏感列，不含标识与抑制记录。
func writePublicCSV(path string, ds *Dataset, hier map[string]*Hierarchy, ev *Evaluation) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建公开 CSV 失败: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := append(append([]string{}, ds.QICols...), ds.SensitiveCol)
	if err := w.Write(header); err != nil {
		return err
	}
	suppressed := map[string]bool{}
	for _, r := range ev.Rows {
		if r.Suppressed {
			suppressed[r.ID] = true
		}
	}
	for _, rec := range ds.Records {
		if suppressed[rec.ID] {
			continue
		}
		row := make([]string, 0, len(ds.QICols)+1)
		for j, v := range rec.QI {
			row = append(row, hier[ds.QICols[j]].Generalize(v, ev.Steps[j]))
		}
		row = append(row, rec.Sensitive)
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

type reportFile struct {
	Purpose string      `json:"purpose"` // 文件用途说明
	Mode    string      `json:"mode"`
	QICols  []string    `json:"qiColumns"`
	Config  *Config     `json:"config"`
	Result  *Evaluation `json:"result"`
}

// writeReport 输出内部核查报告：分组检查、逐行去留与成本，仅供内部审计，不得公开。
func writeReport(path, mode string, ds *Dataset, cfg *Config, ev *Evaluation) error {
	rf := reportFile{
		Purpose: "内部核查报告：记录分组检查、逐行去留与成本，仅供内部审计，不得对外发布",
		Mode:    mode,
		QICols:  ds.QICols,
		Config:  cfg,
		Result:  ev,
	}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入报告失败: %w", err)
	}
	return nil
}
