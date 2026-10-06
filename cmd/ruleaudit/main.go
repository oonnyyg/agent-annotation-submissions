package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"example.com/decision-table-coverage/internal/analyzer"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	path := ""
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println("用法: go run ./cmd/ruleaudit [spec.json]（缺省从标准输入读取）")
			return nil
		}
		path = a
	}
	var f *os.File
	var err error
	if path == "" || path == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
	}
	spec, err := analyzer.LoadSpec(f)
	if err != nil {
		return err
	}
	rep := analyzer.Analyze(spec)
	printReport(spec, rep)
	if len(spec.Samples) > 0 {
		printSamples(spec)
	}
	return nil
}

func printReport(spec *analyzer.Spec, rep *analyzer.Report) {
	fmt.Printf("策略: %s\n", spec.Strategy)
	if !rep.Complete {
		fmt.Printf("分析未完成: 等价单元总数超过上限 %d，无法给出无冲突/全覆盖结论。\n", rep.CellLimit)
		fmt.Println("请提高 cell_limit 或缩小字段域后重试。")
		return
	}
	fmt.Printf("等价单元总数: %d（上限 %d），分析完整。\n", rep.TotalCells, rep.CellLimit)

	fmt.Printf("\n[未覆盖区域] %d 个\n", len(rep.Uncovered))
	for _, r := range rep.Uncovered {
		fmt.Printf("  - %s  复现输入: %s\n", r.Desc, formatInput(spec, r.Input))
	}

	conflictWord := "重叠区域"
	if spec.Strategy == analyzer.StrategyUnique {
		conflictWord = "冲突区域（唯一命中策略下不允许多条命中）"
	}
	fmt.Printf("\n[%s] %d 个\n", conflictWord, len(rep.Overlaps))
	for _, r := range rep.Overlaps {
		fmt.Printf("  - %s  命中规则: %s  复现输入: %s\n",
			r.Desc, strings.Join(r.Rules, ", "), formatInput(spec, r.Input))
	}

	if spec.Strategy == analyzer.StrategyFirst {
		fmt.Printf("\n[被遮蔽规则] %d 条\n", len(rep.Shadowed))
		for _, sh := range rep.Shadowed {
			fmt.Printf("  - 规则 %s 被前面规则联合完全遮蔽: %s\n",
				sh.RuleID, strings.Join(sh.Coverers, ", "))
		}
		fmt.Printf("\n[生效规则见证] %d 条\n", len(rep.Witnesses))
		for _, w := range rep.Witnesses {
			fmt.Printf("  - 规则 %s 最早生效于: %s\n", w.RuleID, formatInput(spec, w.Input))
		}
	}

	ok := len(rep.Uncovered) == 0 && len(rep.Overlaps) == 0 && len(rep.Shadowed) == 0
	if ok {
		fmt.Println("\n结论: 全覆盖、无冲突、无遮蔽。")
	} else {
		fmt.Println("\n结论: 存在上述问题，上线前请处理。")
	}
}

func printSamples(spec *analyzer.Spec) {
	fmt.Printf("\n[样本求值] %d 条\n", len(spec.Samples))
	for i, sample := range spec.Samples {
		matched, err := analyzer.MatchedRules(spec, sample)
		if err != nil {
			fmt.Printf("  - 样本 %d 求值失败: %v\n", i, err)
			continue
		}
		ids := []string{}
		for _, ri := range matched {
			ids = append(ids, spec.Rules[ri].ID)
		}
		switch spec.Strategy {
		case analyzer.StrategyUnique:
			if len(matched) > 1 {
				fmt.Printf("  - 样本 %d %s => 冲突，命中: %s\n", i, formatInput(spec, sample), strings.Join(ids, ", "))
			} else if len(matched) == 1 {
				fmt.Printf("  - 样本 %d %s => %s（输出 %q）\n", i, formatInput(spec, sample), ids[0], spec.Rules[matched[0]].Output)
			} else {
				fmt.Printf("  - 样本 %d %s => 无规则命中\n", i, formatInput(spec, sample))
			}
		case analyzer.StrategyFirst:
			if len(matched) > 0 {
				fmt.Printf("  - 样本 %d %s => 首条命中 %s（输出 %q），全部匹配: %s\n",
					i, formatInput(spec, sample), ids[0], spec.Rules[matched[0]].Output, strings.Join(ids, ", "))
			} else {
				fmt.Printf("  - 样本 %d %s => 无规则命中\n", i, formatInput(spec, sample))
			}
		}
	}
}

// formatInput 按字段声明顺序格式化输入。
func formatInput(spec *analyzer.Spec, input map[string]any) string {
	keys := make([]string, 0, len(input))
	for _, f := range spec.Fields {
		if _, ok := input[f.Name]; ok {
			keys = append(keys, f.Name)
		}
	}
	// 输入中可能存在未声明字段（样本求值会报错，这里兜底排序输出）
	extra := []string{}
	for k := range input {
		if spec.FieldByName(k) == nil {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%v", k, input[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
