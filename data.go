package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
)

// Record 是一条输入记录：标识 + 准标识值 + 敏感值。
type Record struct {
	ID        string
	QI        []string
	Sensitive string
}

// Dataset 保存记录与准标识列名（顺序即配置列顺序）。
type Dataset struct {
	QICols       []string
	SensitiveCol string
	Records      []Record
}

// LoadDataset 读取 CSV：首列为记录标识，末列为敏感列，中间一至四列为准标识。
func LoadDataset(path string, hier map[string]*Hierarchy) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 CSV 失败: %w", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV 至少需要表头和一行数据")
	}
	header := rows[0]
	if len(header) < 3 || len(header) > 6 {
		return nil, fmt.Errorf("CSV 需含标识列、一至四个准标识列和一个敏感列，实际 %d 列", len(header))
	}
	qiCols := header[1 : len(header)-1]
	for _, c := range qiCols {
		if _, ok := hier[c]; !ok {
			return nil, fmt.Errorf("准标识列 %q 缺少层级定义", c)
		}
	}
	ds := &Dataset{QICols: qiCols, SensitiveCol: header[len(header)-1]}
	seenIDs := map[string]bool{}
	for i, row := range rows[1:] {
		line := i + 2
		if len(row) != len(header) {
			return nil, fmt.Errorf("第 %d 行列数 %d 与表头 %d 不一致", line, len(row), len(header))
		}
		for j, v := range row {
			if v == "" {
				return nil, fmt.Errorf("第 %d 行第 %d 列存在缺失值", line, j+1)
			}
		}
		id := row[0]
		if seenIDs[id] {
			return nil, fmt.Errorf("记录标识 %q 重复", id)
		}
		seenIDs[id] = true
		qi := row[1 : len(row)-1]
		for j, v := range qi {
			h := hier[qiCols[j]]
			if _, known := h.parent[v]; !known && v != h.root {
				return nil, fmt.Errorf("第 %d 行准标识值 %q 不在列 %q 的层级树中", line, v, qiCols[j])
			}
			if !h.IsLeaf(v) {
				return nil, fmt.Errorf("第 %d 行准标识值 %q 不是列 %q 层级树的叶节点", line, v, qiCols[j])
			}
		}
		ds.Records = append(ds.Records, Record{ID: id, QI: qi, Sensitive: row[len(row)-1]})
	}
	return ds, nil
}

// Config 是匿名化配置。
type Config struct {
	K               int            `json:"k"`               // 每组最小人数（正整数）
	L               int            `json:"l"`               // 每组敏感值最小种类数（正整数）
	Budget          int            `json:"budget"`          // 抑制预算（非负整数）
	Weights         map[string]int `json:"weights"`         // 各准标识列步数权重（正整数）
	SuppressionCost int            `json:"suppressionCost"` // 每条抑制记录的代价（正整数）
}

func LoadConfig(path string, qiCols []string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("配置文件不是合法 JSON: %w", err)
	}
	if c.K <= 0 {
		return nil, fmt.Errorf("k 必须为正整数")
	}
	if c.L <= 0 {
		return nil, fmt.Errorf("l 必须为正整数")
	}
	if c.Budget < 0 {
		return nil, fmt.Errorf("抑制预算必须为非负整数")
	}
	if c.SuppressionCost <= 0 {
		return nil, fmt.Errorf("抑制代价必须为正整数")
	}
	for _, col := range qiCols {
		w, ok := c.Weights[col]
		if !ok {
			return nil, fmt.Errorf("缺少列 %q 的步数权重", col)
		}
		if w <= 0 {
			return nil, fmt.Errorf("列 %q 的步数权重必须为正整数", col)
		}
	}
	return &c, nil
}
