package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type Position string

const (
	Normal  Position = "normal"
	Reverse Position = "reverse"
)

func (p Position) valid() bool { return p == Normal || p == Reverse }

type Route struct {
	ID       string              `json:"id"`
	Sections []string            `json:"sections"`
	Switches map[string]Position `json:"switches"`
}

type Config struct {
	Sections []string            `json:"sections"`
	Switches map[string]Position `json:"switches"`
	Routes   []Route             `json:"routes"`
}

func LoadConfig(r io.Reader) (*Config, error) {
	var cfg Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Sections) == 0 {
		return fmt.Errorf("配置必须至少声明一个区段")
	}
	sections := map[string]bool{}
	for _, s := range c.Sections {
		if s == "" {
			return fmt.Errorf("区段编号不能为空")
		}
		if sections[s] {
			return fmt.Errorf("区段 %q 重复声明", s)
		}
		sections[s] = true
	}
	for w, p := range c.Switches {
		if !p.valid() {
			return fmt.Errorf("道岔 %q 初始位置非法: %q", w, p)
		}
	}
	routes := map[string]bool{}
	for i := range c.Routes {
		rt := &c.Routes[i]
		if rt.ID == "" {
			return fmt.Errorf("进路编号不能为空")
		}
		if routes[rt.ID] {
			return fmt.Errorf("进路 %q 重复声明", rt.ID)
		}
		routes[rt.ID] = true
		if len(rt.Sections) == 0 {
			return fmt.Errorf("进路 %q 至少需要一个区段", rt.ID)
		}
		seen := map[string]bool{}
		for _, s := range rt.Sections {
			if !sections[s] {
				return fmt.Errorf("进路 %q 引用了不存在的区段 %q", rt.ID, s)
			}
			if seen[s] {
				return fmt.Errorf("进路 %q 中区段 %q 重复出现", rt.ID, s)
			}
			seen[s] = true
		}
		for w, p := range rt.Switches {
			if _, ok := c.Switches[w]; !ok {
				return fmt.Errorf("进路 %q 引用了不存在的道岔 %q", rt.ID, w)
			}
			if !p.valid() {
				return fmt.Errorf("进路 %q 对道岔 %q 要求的位置非法: %q", rt.ID, w, p)
			}
		}
	}
	return nil
}
