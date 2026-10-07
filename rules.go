package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// KeyRule 描述单个排序键列的比较规则。
type KeyRule struct {
	Column string `json:"column"` // 表头列名
	Type   string `json:"type"`   // "text" 或 "decimal"
	Order  string `json:"order"`  // "asc" 或 "desc"
	Empty  string `json:"empty"`  // 空字段位置："first" 或 "last"
}

// Rules 为 CLI 接收的 JSON 规则。
type Rules struct {
	Keys          []KeyRule `json:"keys"`          // 排序键，至少一个
	Dedup         bool      `json:"dedup"`         // 是否按完整排序键去重
	MemoryRecords int       `json:"memoryRecords"` // 内存块最多容纳的记录数，>=1
	MergeFanIn    int       `json:"mergeFanIn"`    // 每轮最多打开的归并输入数，>=2
}

// LoadRules 读取并校验规则文件。
func LoadRules(path string) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取规则文件失败: %w", err)
	}
	var r Rules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("解析规则 JSON 失败: %w", err)
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Rules) validate() error {
	if len(r.Keys) == 0 {
		return fmt.Errorf("规则错误: keys 至少需要一个排序键")
	}
	for i, k := range r.Keys {
		if k.Column == "" {
			return fmt.Errorf("规则错误: keys[%d].column 不能为空", i)
		}
		switch k.Type {
		case "text", "decimal":
		default:
			return fmt.Errorf("规则错误: keys[%d].type 必须为 \"text\" 或 \"decimal\"", i)
		}
		switch k.Order {
		case "asc", "desc":
		default:
			return fmt.Errorf("规则错误: keys[%d].order 必须为 \"asc\" 或 \"desc\"", i)
		}
		switch k.Empty {
		case "first", "last":
		default:
			return fmt.Errorf("规则错误: keys[%d].empty 必须为 \"first\" 或 \"last\"", i)
		}
	}
	if r.MemoryRecords < 1 {
		return fmt.Errorf("规则错误: memoryRecords 必须为 >=1 的正整数，当前为 %d", r.MemoryRecords)
	}
	if r.MergeFanIn < 2 {
		return fmt.Errorf("规则错误: mergeFanIn 必须为 >=2 的正整数，当前为 %d", r.MergeFanIn)
	}
	return nil
}

// resolveColumns 将列名映射为表头下标。
func resolveColumns(header []string, keys []KeyRule) ([]int, error) {
	idx := make([]int, len(keys))
	for i, k := range keys {
		found := -1
		for j, h := range header {
			if h == k.Column {
				found = j
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("规则错误: 排序键列 %q 不在表头中", k.Column)
		}
		idx[i] = found
	}
	return idx, nil
}
