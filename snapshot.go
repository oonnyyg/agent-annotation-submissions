package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Snapshot 是单个实例在某个整数秒时刻的累计直方图快照。
type Snapshot struct {
	Instance string    `json:"instance"`
	Ts       int64     `json:"ts"`
	Boot     string    `json:"boot"`
	Bounds   []float64 `json:"bounds"` // 有限桶上界，严格递增且为正
	Counts   []uint64  `json:"counts"` // 累计桶计数，长度为 len(Bounds)+1，末桶为正无穷
	Sum      float64   `json:"sum"`    // 累计总耗时
}

func (s *Snapshot) validate() error {
	if s.Instance == "" {
		return fmt.Errorf("instance 不能为空")
	}
	if s.Boot == "" {
		return fmt.Errorf("实例 %s 时刻 %d: boot 不能为空", s.Instance, s.Ts)
	}
	if len(s.Bounds) < 1 {
		return fmt.Errorf("实例 %s 时刻 %d: 至少需要一个有限桶界", s.Instance, s.Ts)
	}
	for i, b := range s.Bounds {
		if !(b > 0) {
			return fmt.Errorf("实例 %s 时刻 %d: 桶界必须为正，得到 %v", s.Instance, s.Ts, b)
		}
		if i > 0 && b <= s.Bounds[i-1] {
			return fmt.Errorf("实例 %s 时刻 %d: 桶界必须严格递增", s.Instance, s.Ts)
		}
	}
	if len(s.Counts) != len(s.Bounds)+1 {
		return fmt.Errorf("实例 %s 时刻 %d: 计数长度 %d 应为桶界长度+1 (%d)",
			s.Instance, s.Ts, len(s.Counts), len(s.Bounds)+1)
	}
	for i := 1; i < len(s.Counts); i++ {
		if s.Counts[i] < s.Counts[i-1] {
			return fmt.Errorf("实例 %s 时刻 %d: 累计计数必须随桶界不减", s.Instance, s.Ts)
		}
	}
	if s.Sum < 0 {
		return fmt.Errorf("实例 %s 时刻 %d: 累计总耗时不能为负", s.Instance, s.Ts)
	}
	if s.Counts[len(s.Counts)-1] == 0 && s.Sum != 0 {
		return fmt.Errorf("实例 %s 时刻 %d: 总数为零时总耗时必须为零", s.Instance, s.Ts)
	}
	return nil
}

func (s *Snapshot) total() uint64 { return s.Counts[len(s.Counts)-1] }

// LoadSnapshots 从 JSONL 读取快照，输入可乱序；重复实例时刻拒绝。
func LoadSnapshots(r io.Reader) (map[string][]*Snapshot, error) {
	byInstance := make(map[string][]*Snapshot)
	seen := make(map[string]map[int64]bool)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var s Snapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("第 %d 行: JSON 解析失败: %w", line, err)
		}
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", line, err)
		}
		m := seen[s.Instance]
		if m == nil {
			m = make(map[int64]bool)
			seen[s.Instance] = m
		}
		if m[s.Ts] {
			return nil, fmt.Errorf("第 %d 行: 实例 %s 时刻 %d 重复", line, s.Instance, s.Ts)
		}
		m[s.Ts] = true
		snap := s
		byInstance[s.Instance] = append(byInstance[s.Instance], &snap)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for _, snaps := range byInstance {
		sort.Slice(snaps, func(i, j int) bool { return snaps[i].Ts < snaps[j].Ts })
	}
	return byInstance, nil
}

func sameBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
