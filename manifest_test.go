package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadErr(t *testing.T, manifest string) error {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "build.json")
	if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(p)
	return err
}

func TestManifestValidation(t *testing.T) {
	cmd := `["/bin/echo","hi"]`
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"未知依赖", `{"targets":{"a":{"command":` + cmd + `,"deps":["ghost"],"outputs":["a.txt"]}}}`, "未知目标"},
		{"依赖自身", `{"targets":{"a":{"command":` + cmd + `,"deps":["a"],"outputs":["a.txt"]}}}`, "不能依赖自身"},
		{"绝对路径", `{"targets":{"a":{"command":` + cmd + `,"outputs":["/tmp/a.txt"]}}}`, "相对路径"},
		{"越界路径", `{"targets":{"a":{"command":` + cmd + `,"inputs":["../secret.txt"],"outputs":["a.txt"]}}}`, "越出工作根"},
		{"输出冲突", `{"targets":{
			"a":{"command":` + cmd + `,"outputs":["x.txt"]},
			"b":{"command":` + cmd + `,"outputs":["x.txt"]}}}`, "同时被"},
		{"未声明依赖就读取他人输出", `{"targets":{
			"a":{"command":` + cmd + `,"outputs":["x.txt"]},
			"b":{"command":` + cmd + `,"inputs":["x.txt"],"outputs":["b.txt"]}}}`, "未声明对其依赖"},
		{"空命令", `{"targets":{"a":{"command":[],"outputs":["a.txt"]}}}`, "command 不能为空"},
		{"无输出", `{"targets":{"a":{"command":` + cmd + `}}}`, "至少一个输出"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := loadErr(t, c.manifest)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("期望错误包含 %q，得到 %v", c.want, err)
			}
		})
	}
}

func TestSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("s"), 0o644)
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("无法创建符号链接: %v", err)
	}
	manifest := `{"targets":{"a":{"command":["/bin/echo"],"inputs":["link.txt"],"outputs":["a.txt"]}}}`
	os.WriteFile(filepath.Join(root, "build.json"), []byte(manifest), 0o644)
	if _, err := LoadManifest(filepath.Join(root, "build.json")); err == nil ||
		!strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("应拒绝符号链接输入, 得到 %v", err)
	}
}

func TestCycleRejectedBeforeExecution(t *testing.T) {
	manifest := `{"targets":{
		"a":{"command":["/bin/echo"],"deps":["b"],"outputs":["a.txt"]},
		"b":{"command":["/bin/echo"],"deps":["a"],"outputs":["b.txt"]}}}`
	root := t.TempDir()
	p := filepath.Join(root, "build.json")
	os.WriteFile(p, []byte(manifest), 0o644)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.closure([]string{"a"})
	if _, err := m.topoOrder(set); err == nil || !strings.Contains(err.Error(), "环") {
		t.Fatalf("应检测出环, 得到 %v", err)
	}
}

func TestTopoOrderLexicographic(t *testing.T) {
	manifest := `{"targets":{
		"z":{"command":["/bin/echo"],"outputs":["z.txt"]},
		"a":{"command":["/bin/echo"],"outputs":["a.txt"]},
		"m":{"command":["/bin/echo"],"deps":["z"],"outputs":["m.txt"]}}}`
	root := t.TempDir()
	p := filepath.Join(root, "build.json")
	os.WriteFile(p, []byte(manifest), 0o644)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.closure([]string{"a", "m", "z"})
	order, err := m.topoOrder(set)
	if err != nil {
		t.Fatal(err)
	}
	// a 与 z 无依赖关系，字典序 a 先；m 依赖 z，必须在 z 后。
	want := []string{"a", "z", "m"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("顺序应为 %v, 得到 %v", want, order)
	}
}

func TestClosureOnly(t *testing.T) {
	manifest := `{"targets":{
		"a":{"command":["/bin/echo"],"outputs":["a.txt"]},
		"b":{"command":["/bin/echo"],"deps":["a"],"outputs":["b.txt"]},
		"unrelated":{"command":["/bin/echo"],"outputs":["u.txt"]}}}`
	root := t.TempDir()
	p := filepath.Join(root, "build.json")
	os.WriteFile(p, []byte(manifest), 0o644)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := m.closure([]string{"b"})
	if len(set) != 2 || !set["a"] || !set["b"] || set["unrelated"] {
		t.Fatalf("闭包应只含 a,b: %v", set)
	}
}
