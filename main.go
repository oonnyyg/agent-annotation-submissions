package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var rulesPath, outPath, statsPath string
	var inputs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-rules":
			i++
			if i >= len(args) {
				return fmt.Errorf("-rules 缺少参数")
			}
			rulesPath = args[i]
		case "-out":
			i++
			if i >= len(args) {
				return fmt.Errorf("-out 缺少参数")
			}
			outPath = args[i]
		case "-stats":
			i++
			if i >= len(args) {
				return fmt.Errorf("-stats 缺少参数")
			}
			statsPath = args[i]
		default:
			inputs = append(inputs, args[i])
		}
	}
	if rulesPath == "" {
		return fmt.Errorf("缺少 -rules <规则.json>")
	}
	if outPath == "" {
		return fmt.Errorf("缺少 -out <输出.csv>")
	}
	if len(inputs) == 0 {
		return fmt.Errorf("缺少输入 CSV 文件列表")
	}

	rules, err := LoadRules(rulesPath)
	if err != nil {
		return err
	}

	// 临时目录放在输出文件所在目录，保证最终 rename 为同卷原子替换，
	// 且临时文件不会散落到其他任务的目录。
	outDir := filepath.Dir(outPath)
	tmpDir, err := os.MkdirTemp(outDir, ".sort-tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	// 无论成功失败，结束后清理本次的临时目录；只删除本任务创建的目录。
	defer os.RemoveAll(tmpDir)

	tmpOut := filepath.Join(tmpDir, "output.csv")
	s := &sorter{rules: rules, cmp: &comparator{keys: rules.Keys}, tmpDir: tmpDir}
	stats, err := s.run(inputs, tmpOut)
	if err != nil {
		return err // 失败：不触碰旧输出，临时目录由 defer 清理
	}

	// 成功后才替换目标文件。
	if err := os.Rename(tmpOut, outPath); err != nil {
		return fmt.Errorf("替换输出文件失败: %w", err)
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	if statsPath != "" {
		if err := os.WriteFile(statsPath, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("写入统计文件失败: %w", err)
		}
	} else {
		fmt.Println(string(data))
	}
	return nil
}
