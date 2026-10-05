package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestHelperProcess 不是真正的测试，而是被构建器当作子进程命令启动的助手。
// 用法: <test-binary> -test.run=TestHelperProcess -- <op> [args...]
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	switch args[0] {
	case "copy": // copy <in> <out>
		data, err := os.ReadFile(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := os.WriteFile(args[2], data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "fail":
		fmt.Fprintln(os.Stderr, "模拟失败")
		os.Exit(3)
	case "noop": // 成功但不产生任何输出
	}
	os.Exit(0)
}

func helperCmd(args ...string) []string {
	exe, _ := os.Executable()
	return append([]string{exe, "-test.run=TestHelperProcess", "--"}, args...)
}

// setup 创建临时工作根，写入清单与文件，返回工作根和可执行的 build 函数。
func setup(t *testing.T, manifest string, files map[string]string) (string, func(dryRun bool, targets ...string) *Report) {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mpath := filepath.Join(root, "build.json")
	if err := os.WriteFile(mpath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(dryRun bool, targets ...string) *Report {
		t.Helper()
		m, err := LoadManifest(mpath)
		if err != nil {
			t.Fatalf("清单加载失败: %v", err)
		}
		set, err := m.closure(targets)
		if err != nil {
			t.Fatalf("闭包计算失败: %v", err)
		}
		order, err := m.topoOrder(set)
		if err != nil {
			t.Fatalf("拓扑排序失败: %v", err)
		}
		store, err := loadCache(filepath.Join(root, ".buildcache"))
		if err != nil {
			t.Fatal(err)
		}
		return newRunner(m, store, dryRun).run(order)
	}
	return root, build
}

func statusOf(rep *Report, name string) TargetReport {
	for _, tr := range rep.Targets {
		if tr.Name == name {
			return tr
		}
	}
	return TargetReport{Name: name, Status: "<absent>"}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
