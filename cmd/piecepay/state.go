package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Tier 表示累进阶梯中的一档：不超过 UpTo 件的部分按 RateCents（整数分/件）计价。
// UpTo 为 nil 表示最后一档（上不封顶）。
type Tier struct {
	UpTo      *int64 `json:"up_to,omitempty"`
	RateCents int64  `json:"rate_cents"`
}

// SchemeVersion 是计价方案的一个不可变版本。
type SchemeVersion struct {
	Version int    `json:"version"`
	Tiers   []Tier `json:"tiers"`
}

// Scheme 是计价方案，可多次更新，每次更新追加一个新版本。
type Scheme struct {
	ID       string          `json:"id"`
	Versions []SchemeVersion `json:"versions"`
}

// Batch 是已登记的生产批次，保存登记时的方案版本与分配结果快照。
type Batch struct {
	ID            string           `json:"id"`
	SchemeID      string           `json:"scheme_id"`
	SchemeVersion int              `json:"scheme_version"`
	Month         string           `json:"month"`
	Qty           int64            `json:"qty"`
	Members       map[string]int64 `json:"members"`
	TotalCents    int64            `json:"total_cents"`
	Shares        map[string]int64 `json:"shares"`
	Voided        bool             `json:"voided"`
	VoidMonth     string           `json:"void_month,omitempty"`
}

// MonthBook 记录月份封账状态。
type MonthBook struct {
	Closed bool `json:"closed"`
}

// State 是全部持久化状态。
type State struct {
	Schemes map[string]*Scheme    `json:"schemes"`
	Batches map[string]*Batch     `json:"batches"`
	Months  map[string]*MonthBook `json:"months"`
	// Requests 保存批次编号对应的规范化请求内容，用于幂等重提判定。
	Requests map[string]string `json:"requests"`
}

func newState() *State {
	return &State{
		Schemes:  map[string]*Scheme{},
		Batches:  map[string]*Batch{},
		Months:   map[string]*MonthBook{},
		Requests: map[string]string{},
	}
}

func statePath(dir string) string {
	return filepath.Join(dir, "state.json")
}

// loadState 从目录加载状态；文件不存在时返回空状态。
func loadState(dir string) (*State, error) {
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return newState(), nil
		}
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}
	st := newState()
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %w", err)
	}
	return st, nil
}

// saveState 原子写入状态文件，保证重开结果一致。
func saveState(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(dir))
}

func (s *State) monthClosed(month string) bool {
	m, ok := s.Months[month]
	return ok && m.Closed
}

// canonicalMembers 将成员权重表规范化为可比较的字符串。
func canonicalMembers(m map[string]int64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ""
	for _, k := range keys {
		out += fmt.Sprintf("%s=%d;", k, m[k])
	}
	return out
}
