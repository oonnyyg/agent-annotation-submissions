package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// cacheEntry 记录一个目标上次成功构建的指纹与输出内容哈希。
type cacheEntry struct {
	Fingerprint  string            `json:"fingerprint"`
	OutputHashes map[string]string `json:"output_hashes"`
}

// cacheStore 是落盘的缓存，重启后可复用。
type cacheStore struct {
	dir     string
	Entries map[string]*cacheEntry `json:"entries"`
}

func loadCache(dir string) (*cacheStore, error) {
	s := &cacheStore{dir: dir, Entries: map[string]*cacheEntry{}}
	data, err := os.ReadFile(filepath.Join(dir, "cache.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("缓存文件损坏: %w", err)
	}
	if s.Entries == nil {
		s.Entries = map[string]*cacheEntry{}
	}
	s.dir = dir
	return s, nil
}

func (s *cacheStore) save() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "cache.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "cache.json"))
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// fingerprint 计算目标指纹：命令参数 + 输入内容 + 直接依赖输出内容。
// 只看内容，不看修改时间。
func (m *Manifest) fingerprint(t *Target) (string, error) {
	h := sha256.New()
	for _, arg := range t.Command {
		fmt.Fprintf(h, "arg\x00%s\x00", arg)
	}
	inputs := append([]string{}, t.Inputs...)
	sort.Strings(inputs)
	for _, in := range inputs {
		sum, err := hashFile(filepath.Join(m.Root, in))
		if err != nil {
			return "", fmt.Errorf("读取输入 %q 失败: %w", in, err)
		}
		fmt.Fprintf(h, "input\x00%s\x00%s\x00", filepath.Clean(in), sum)
	}
	deps := append([]string{}, t.Deps...)
	sort.Strings(deps)
	for _, d := range deps {
		for _, out := range m.Targets[d].Outputs {
			sum, err := hashFile(filepath.Join(m.Root, out))
			if err != nil {
				return "", fmt.Errorf("读取依赖 %q 的输出 %q 失败: %w", d, out, err)
			}
			fmt.Fprintf(h, "dep\x00%s\x00%s\x00%s\x00", d, filepath.Clean(out), sum)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashOutputs 计算目标当前各输出的内容哈希。
func (m *Manifest) hashOutputs(t *Target) (map[string]string, error) {
	hashes := map[string]string{}
	for _, out := range t.Outputs {
		sum, err := hashFile(filepath.Join(m.Root, out))
		if err != nil {
			return nil, fmt.Errorf("输出 %q 缺失或不可读: %w", out, err)
		}
		hashes[filepath.Clean(out)] = sum
	}
	return hashes, nil
}

// checkHit 判断缓存是否命中，并给出未命中的明确原因。
func (m *Manifest) checkHit(s *cacheStore, t *Target, fp string) (bool, string) {
	e, ok := s.Entries[t.Name]
	if !ok {
		return false, "无缓存记录（首次构建）"
	}
	if e.Fingerprint != fp {
		return false, "指纹变化（命令、输入或依赖输出内容改变）"
	}
	for _, out := range t.Outputs {
		key := filepath.Clean(out)
		sum, err := hashFile(filepath.Join(m.Root, out))
		if err != nil {
			return false, fmt.Sprintf("声明输出 %q 不存在", out)
		}
		if e.OutputHashes[key] != sum {
			return false, fmt.Sprintf("输出 %q 被外部修改，与缓存记录不符", out)
		}
	}
	return true, "指纹一致且输出未被篡改"
}
