package analyzer

import (
	"encoding/json"
	"fmt"
	"io"
)

type FieldType string

const (
	FieldEnum FieldType = "enum"
	FieldInt  FieldType = "int"
)

type Field struct {
	Name   string    `json:"name"`
	Type   FieldType `json:"type"`
	Values []string  `json:"values,omitempty"`
	Min    int       `json:"min,omitempty"`
	Max    int       `json:"max,omitempty"`
}

type IntRange struct {
	Min          *int `json:"min,omitempty"`
	Max          *int `json:"max,omitempty"`
	MinExclusive bool `json:"min_exclusive,omitempty"`
	MaxExclusive bool `json:"max_exclusive,omitempty"`
}

type Condition struct {
	Enum  []string  `json:"enum,omitempty"`
	Range *IntRange `json:"range,omitempty"`
}

type Rule struct {
	ID     string               `json:"id"`
	Output string               `json:"output,omitempty"`
	When   map[string]Condition `json:"when,omitempty"`
}

type Strategy string

const (
	StrategyUnique Strategy = "unique"
	StrategyFirst  Strategy = "first"
)

type Spec struct {
	Fields    []Field          `json:"fields"`
	Strategy  Strategy         `json:"strategy"`
	CellLimit int              `json:"cell_limit,omitempty"`
	Rules     []Rule           `json:"rules"`
	Samples   []map[string]any `json:"samples,omitempty"`
}

func LoadSpec(r io.Reader) (*Spec, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (s *Spec) Validate() error {
	if len(s.Fields) == 0 {
		return fmt.Errorf("至少需要声明一个字段")
	}
	if s.Strategy != StrategyUnique && s.Strategy != StrategyFirst {
		return fmt.Errorf("strategy 必须是 %q 或 %q", StrategyUnique, StrategyFirst)
	}
	if s.CellLimit <= 0 {
		s.CellLimit = 1_000_000
	}
	seen := map[string]bool{}
	for i, f := range s.Fields {
		if f.Name == "" {
			return fmt.Errorf("字段 %d 缺少 name", i)
		}
		if seen[f.Name] {
			return fmt.Errorf("字段 %q 重复声明", f.Name)
		}
		seen[f.Name] = true
		switch f.Type {
		case FieldEnum:
			if len(f.Values) == 0 {
				return fmt.Errorf("枚举字段 %q 缺少 values", f.Name)
			}
			vseen := map[string]bool{}
			for _, v := range f.Values {
				if vseen[v] {
					return fmt.Errorf("枚举字段 %q 的值 %q 重复", f.Name, v)
				}
				vseen[v] = true
			}
		case FieldInt:
			if f.Min > f.Max {
				return fmt.Errorf("整数字段 %q 的 min 大于 max", f.Name)
			}
		default:
			return fmt.Errorf("字段 %q 类型必须是 %q 或 %q", f.Name, FieldEnum, FieldInt)
		}
	}
	for i, rule := range s.Rules {
		if rule.ID == "" {
			return fmt.Errorf("规则 %d 缺少 id", i)
		}
		for name, cond := range rule.When {
			field := s.FieldByName(name)
			if field == nil {
				return fmt.Errorf("规则 %q 引用了未知字段 %q", rule.ID, name)
			}
			if err := validateCondition(rule.ID, field, cond); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCondition(ruleID string, f *Field, c Condition) error {
	switch f.Type {
	case FieldEnum:
		if c.Range != nil {
			return fmt.Errorf("规则 %q 字段 %q 是枚举，不能使用区间条件", ruleID, f.Name)
		}
		if len(c.Enum) == 0 {
			return fmt.Errorf("规则 %q 字段 %q 的枚举条件为空", ruleID, f.Name)
		}
		for _, v := range c.Enum {
			if !f.HasValue(v) {
				return fmt.Errorf("规则 %q 字段 %q 包含未知枚举值 %q", ruleID, f.Name, v)
			}
		}
	case FieldInt:
		if len(c.Enum) > 0 {
			return fmt.Errorf("规则 %q 字段 %q 是整数，不能使用枚举条件", ruleID, f.Name)
		}
		if c.Range == nil {
			return fmt.Errorf("规则 %q 字段 %q 缺少区间条件", ruleID, f.Name)
		}
		lo, hi, err := NormalizeRange(f, c.Range)
		if err != nil {
			return fmt.Errorf("规则 %q 字段 %q: %w", ruleID, f.Name, err)
		}
		if lo > hi {
			return fmt.Errorf("规则 %q 字段 %q 的条件区间为空（规范化后 [%d, %d]）", ruleID, f.Name, lo, hi)
		}
	}
	return nil
}

// NormalizeRange 把带开闭端点的区间规范化为闭区间整数边界。
func NormalizeRange(f *Field, r *IntRange) (int, int, error) {
	lo := f.Min
	hi := f.Max
	if r.Min != nil {
		lo = *r.Min
		if r.MinExclusive {
			lo++
		}
	}
	if r.Max != nil {
		hi = *r.Max
		if r.MaxExclusive {
			hi--
		}
	}
	if lo > f.Max || hi < f.Min {
		return lo, hi, fmt.Errorf("条件区间 [%d, %d] 完全在域 [%d, %d] 之外", lo, hi, f.Min, f.Max)
	}
	return lo, hi, nil
}

func (s *Spec) FieldByName(name string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

func (f *Field) HasValue(v string) bool {
	for _, x := range f.Values {
		if x == v {
			return true
		}
	}
	return false
}
