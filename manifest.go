package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Target 是清单中的一个构建目标。
type Target struct {
	Name    string   `json:"-"`
	Deps    []string `json:"deps"`
	Command []string `json:"command"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`
}

// Manifest 是解析并校验后的构建清单。
type Manifest struct {
	Root    string
	Targets map[string]*Target
}

// LoadManifest 读取并完整校验清单；任何错误都会拒绝整个清单。
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取清单失败: %w", err)
	}
	var raw struct {
		Targets map[string]*Target `json:"targets"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("清单 JSON 解析失败: %w", err)
	}
	if len(raw.Targets) == 0 {
		return nil, fmt.Errorf("清单不包含任何目标")
	}
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	m := &Manifest{Root: root, Targets: raw.Targets}
	for name, t := range m.Targets {
		t.Name = name
		if len(t.Command) == 0 {
			return nil, fmt.Errorf("目标 %q: command 不能为空", name)
		}
		if len(t.Outputs) == 0 {
			return nil, fmt.Errorf("目标 %q: 必须声明至少一个输出", name)
		}
		for _, p := range append(append([]string{}, t.Inputs...), t.Outputs...) {
			if err := checkPath(root, p); err != nil {
				return nil, fmt.Errorf("目标 %q: %w", name, err)
			}
		}
	}
	if err := m.checkDeps(); err != nil {
		return nil, err
	}
	if err := m.checkOutputOwnership(); err != nil {
		return nil, err
	}
	return m, nil
}

// checkPath 拒绝绝对路径、越界路径与符号链接。
func checkPath(root, p string) error {
	if p == "" || filepath.IsAbs(p) {
		return fmt.Errorf("路径 %q 非法: 必须是相对路径", p)
	}
	clean := filepath.Clean(p)
	if filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return fmt.Errorf("路径 %q 非法: 必须是相对路径", p)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("路径 %q 越出工作根", p)
	}
	// 逐级检查已存在的路径组件是否为符号链接。
	cur := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("路径 %q 含有符号链接组件 %q", p, cur)
		}
	}
	return nil
}

// checkDeps 校验依赖目标存在且无重复。
func (m *Manifest) checkDeps() error {
	for name, t := range m.Targets {
		seen := map[string]bool{}
		for _, d := range t.Deps {
			if seen[d] {
				return fmt.Errorf("目标 %q: 重复依赖 %q", name, d)
			}
			seen[d] = true
			if d == name {
				return fmt.Errorf("目标 %q: 不能依赖自身", name)
			}
			if _, ok := m.Targets[d]; !ok {
				return fmt.Errorf("目标 %q: 依赖未知目标 %q", name, d)
			}
		}
	}
	return nil
}

// checkOutputOwnership 保证输出归属唯一，且读取其他目标输出必须声明依赖。
func (m *Manifest) checkOutputOwnership() error {
	owner := map[string]string{} // 规范化输出路径 -> 目标名
	for name, t := range m.Targets {
		for _, out := range t.Outputs {
			key := filepath.Clean(out)
			if prev, ok := owner[key]; ok {
				return fmt.Errorf("输出 %q 同时被目标 %q 和 %q 声明", out, prev, name)
			}
			owner[key] = name
		}
	}
	for name, t := range m.Targets {
		depSet := map[string]bool{}
		for _, d := range t.Deps {
			depSet[d] = true
		}
		for _, in := range t.Inputs {
			if o, ok := owner[filepath.Clean(in)]; ok && o != name && !depSet[o] {
				return fmt.Errorf("目标 %q: 输入 %q 是目标 %q 的输出，但未声明对其依赖", name, in, o)
			}
		}
	}
	return nil
}

// sortedNames 返回字典序目标名，保证输出稳定。
func (m *Manifest) sortedNames() []string {
	names := make([]string, 0, len(m.Targets))
	for n := range m.Targets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
