package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const twoLevelManifest = `{
  "targets": {
    "gen":  {"command": %s, "inputs": ["in.txt"], "outputs": ["mid.txt"]},
    "final": {"command": %s, "deps": ["gen"], "inputs": ["mid.txt"], "outputs": ["out.txt"]}
  }
}`

func twoLevel(t *testing.T) (string, func(bool, ...string) *Report) {
	t.Helper()
	q := func(args ...string) string {
		bs, _ := json.Marshal(helperCmd(args...))
		return string(bs)
	}
	manifest := fmt.Sprintf(twoLevelManifest, q("copy", "in.txt", "mid.txt"), q("copy", "mid.txt", "out.txt"))
	return setup(t, manifest, map[string]string{"in.txt": "hello"})
}

func TestContentInvalidation(t *testing.T) {
	root, build := twoLevel(t)

	rep := build(false, "final")
	if statusOf(rep, "gen").Status != StatusRebuilt || statusOf(rep, "final").Status != StatusRebuilt {
		t.Fatalf("首次构建应全部 rebuilt: %+v", rep)
	}
	// 内容不变，即使修改时间变化也不应重建。
	now := time.Now()
	in := filepath.Join(root, "in.txt")
	os.Chtimes(in, now, now)
	rep = build(false, "final")
	if statusOf(rep, "gen").Status != StatusCached || statusOf(rep, "final").Status != StatusCached {
		t.Fatalf("内容未变应命中缓存: %+v", rep)
	}
	// 修改输入内容 -> gen 重建，final 因依赖输出变化也重建。
	write(t, root, "in.txt", "changed")
	rep = build(false, "final")
	if statusOf(rep, "gen").Status != StatusRebuilt {
		t.Fatalf("输入变化后 gen 应重建: %+v", rep)
	}
	if statusOf(rep, "final").Status != StatusRebuilt {
		t.Fatalf("依赖输出变化后 final 应重建: %+v", rep)
	}
	// 改回原内容 -> 指纹恢复，但缓存记录已被覆盖，应再次重建一次后稳定。
	write(t, root, "in.txt", "hello")
	rep = build(false, "final")
	if statusOf(rep, "gen").Status != StatusRebuilt {
		t.Fatalf("内容恢复后指纹与当前缓存不同，应重建: %+v", rep)
	}
	rep = build(false, "final")
	if statusOf(rep, "final").Status != StatusCached {
		t.Fatalf("稳定后应命中缓存: %+v", rep)
	}
}

func TestOutputTamper(t *testing.T) {
	root, build := twoLevel(t)
	build(false, "final")

	// 外部篡改输出 -> 缓存不命中并重建。
	write(t, root, "out.txt", "tampered")
	rep := build(false, "final")
	tr := statusOf(rep, "final")
	if tr.Status != StatusRebuilt || !strings.Contains(tr.Reason, "外部修改") {
		t.Fatalf("输出被篡改应重建并说明原因: %+v", tr)
	}
	// 手动把输出改回正确内容 -> 与缓存记录一致，应命中。
	write(t, root, "out.txt", "hello")
	rep = build(false, "final")
	if statusOf(rep, "final").Status != StatusCached {
		t.Fatalf("输出恢复一致后应命中缓存: %+v", rep)
	}
	// 删除中间输出 -> gen 重建。
	os.Remove(filepath.Join(root, "mid.txt"))
	rep = build(false, "final")
	if statusOf(rep, "gen").Status != StatusRebuilt {
		t.Fatalf("输出丢失应重建: %+v", rep)
	}
}

func TestBranchFailure(t *testing.T) {
	q := func(args ...string) string {
		bs, _ := json.Marshal(helperCmd(args...))
		return string(bs)
	}
	manifest := `{
	  "targets": {
	    "bad":   {"command": ` + q("fail") + `, "inputs": ["in.txt"], "outputs": ["bad.txt"]},
	    "child": {"command": ` + q("copy", "bad.txt", "child.txt") + `, "deps": ["bad"], "inputs": ["bad.txt"], "outputs": ["child.txt"]},
	    "other": {"command": ` + q("copy", "in.txt", "other.txt") + `, "inputs": ["in.txt"], "outputs": ["other.txt"]}
	  }
	}`
	_, build := setup(t, manifest, map[string]string{"in.txt": "x"})
	rep := build(false, "bad", "child", "other")
	if !rep.Failed {
		t.Fatal("报告应标记整体失败")
	}
	if statusOf(rep, "bad").Status != StatusFailed {
		t.Fatalf("bad 应失败: %+v", rep)
	}
	tr := statusOf(rep, "child")
	if tr.Status != StatusSkipped || !strings.Contains(tr.Reason, "bad") {
		t.Fatalf("child 应因依赖失败被跳过: %+v", tr)
	}
	if statusOf(rep, "other").Status != StatusRebuilt {
		t.Fatalf("无关分支 other 应正常执行: %+v", rep)
	}
	// 失败目标不得写入缓存。
	rep = build(false, "bad")
	if statusOf(rep, "bad").Status != StatusFailed {
		t.Fatalf("失败目标不应出现缓存命中: %+v", rep)
	}
}

func TestMissingDeclaredOutput(t *testing.T) {
	q := func(args ...string) string {
		bs, _ := json.Marshal(helperCmd(args...))
		return string(bs)
	}
	manifest := `{
	  "targets": {
	    "noout": {"command": ` + q("noop") + `, "inputs": ["in.txt"], "outputs": ["none.txt"]}
	  }
	}`
	_, build := setup(t, manifest, map[string]string{"in.txt": "x"})
	rep := build(false, "noout")
	tr := statusOf(rep, "noout")
	if tr.Status != StatusFailed || !strings.Contains(tr.Reason, "none.txt") {
		t.Fatalf("缺少声明输出应失败: %+v", tr)
	}
}

func TestDryRun(t *testing.T) {
	root, build := twoLevel(t)
	build(false, "final")
	cachePath := filepath.Join(root, ".buildcache", "cache.json")
	before, _ := os.ReadFile(cachePath)

	// 修改输入后预览：gen 计划重建，final 应等待上游结果。
	write(t, root, "in.txt", "new")
	rep := build(true, "final")
	if !rep.DryRun {
		t.Fatal("报告应标记 dry_run")
	}
	if statusOf(rep, "gen").Status != StatusPlanRun {
		t.Fatalf("gen 应为 would-run: %+v", rep)
	}
	tr := statusOf(rep, "final")
	if tr.Status != StatusPlanWait || !strings.Contains(tr.Reason, "待定") {
		t.Fatalf("final 应为 pending-upstream: %+v", tr)
	}
	// 干跑不得修改缓存，也不得产生新输出。
	after, _ := os.ReadFile(cachePath)
	if string(before) != string(after) {
		t.Fatal("干跑不应修改缓存文件")
	}
	data, _ := os.ReadFile(filepath.Join(root, "out.txt"))
	if string(data) != "hello" {
		t.Fatal("干跑不应运行命令")
	}
	// 恢复输入内容（指纹回到缓存记录），干跑应全部命中。
	write(t, root, "in.txt", "hello")
	rep = build(true, "final")
	if statusOf(rep, "gen").Status != StatusPlanHit {
		t.Fatalf("无变化时应为 would-cache: %+v", rep)
	}
}
