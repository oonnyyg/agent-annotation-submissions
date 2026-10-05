package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		manifestPath = flag.String("manifest", "build.json", "清单文件路径")
		cacheDir     = flag.String("cache-dir", "", "缓存目录（默认 <工作根>/.buildcache）")
		dryRun       = flag.Bool("dry-run", false, "只预览执行计划，不运行命令、不改缓存")
	)
	flag.Parse()
	want := flag.Args()
	if err := run(*manifestPath, *cacheDir, *dryRun, want); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(manifestPath, cacheDir string, dryRun bool, want []string) error {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	if cacheDir == "" {
		cacheDir = filepath.Join(m.Root, ".buildcache")
	}
	// 默认构建全部目标；指定目标时仅取其依赖闭包。
	if len(want) == 0 {
		want = m.sortedNames()
	}
	set, err := m.closure(want)
	if err != nil {
		return err
	}
	// 先校验完整依赖图，发现环直接拒绝，不执行任何命令。
	order, err := m.topoOrder(set)
	if err != nil {
		return err
	}
	store, err := loadCache(cacheDir)
	if err != nil {
		return err
	}
	rep := newRunner(m, store, dryRun).run(order)
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	if rep.Failed {
		return fmt.Errorf("存在失败目标")
	}
	return nil
}
