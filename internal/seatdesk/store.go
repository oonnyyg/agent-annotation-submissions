package seatdesk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Seat 用正整数排号 Row 和列号 Col 定位一个座位。
type Seat struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// Order 表示一笔订单及其当前占座。
type Order struct {
	ID    string `json:"id"`
	Size  int    `json:"size"`
	Row   int    `json:"row"` // 首选坐标
	Col   int    `json:"col"`
	Seats []Seat `json:"seats"`
	Cost  int    `json:"cost"`
}

// Hall 描述影厅布局，最多 20 个座位。
type Hall struct {
	Rows     int    `json:"rows"`
	Cols     int    `json:"cols"`
	Disabled []Seat `json:"disabled"`
}

// Store 是持久化在目录中的影厅与订单状态。
type Store struct {
	Hall   *Hall             `json:"hall"`
	Orders map[string]*Order `json:"orders"`

	dir string
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

// Load 从目录恢复状态；目录中尚无状态时返回空 Store。
func Load(dir string) (*Store, error) {
	s := &Store{dir: dir, Orders: map[string]*Order{}}
	data, err := os.ReadFile(statePath(dir))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("状态文件损坏: %w", err)
	}
	s.dir = dir
	if s.Orders == nil {
		s.Orders = map[string]*Order{}
	}
	return s, nil
}

// Save 原子地写入状态文件，避免崩溃留下半截文件。
func (s *Store) Save() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(s.dir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(s.dir))
}

func (s *Store) disabledSet() map[Seat]bool {
	set := map[Seat]bool{}
	if s.Hall != nil {
		for _, d := range s.Hall.Disabled {
			set[d] = true
		}
	}
	return set
}

// occupied 返回除 exclude 订单外所有已占座位。
func (s *Store) occupied(exclude string) map[Seat]string {
	occ := map[Seat]string{}
	for id, o := range s.Orders {
		if id == exclude {
			continue
		}
		for _, st := range o.Seats {
			occ[st] = id
		}
	}
	return occ
}

// InitHall 初始化影厅；重复初始化会被拒绝。
func (s *Store) InitHall(rows, cols int, disabled []Seat) error {
	if s.Hall != nil {
		return fmt.Errorf("影厅已初始化")
	}
	if rows < 1 || cols < 1 || rows*cols > 20 {
		return fmt.Errorf("影厅规模非法：座位总数须为 1..20")
	}
	for _, d := range disabled {
		if d.Row < 1 || d.Row > rows || d.Col < 1 || d.Col > cols {
			return fmt.Errorf("停用坐标越界: %+v", d)
		}
	}
	s.Hall = &Hall{Rows: rows, Cols: cols, Disabled: disabled}
	return nil
}
