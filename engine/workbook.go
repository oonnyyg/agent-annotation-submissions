package engine

import (
	"fmt"
	"math"
)

// ErrorKind 描述失败类型。失败绝不以零值替代。
type ErrorKind string

const (
	ErrCycle   ErrorKind = "CYCLE"  // 环成员
	ErrDepend  ErrorKind = "DEPEND" // 依赖了失败单元格的下游
	ErrDivZero ErrorKind = "DIV0"   // 除零（含 0/0）
	ErrNum     ErrorKind = "NUM"    // 非有限运算结果
)

// CellError 是公式单元格的失败结果。
type CellError struct {
	Kind ErrorKind `json:"kind"`
	Msg  string    `json:"msg"`
}

func (e CellError) Error() string { return string(e.Kind) + ": " + e.Msg }

// Result 是一个单元格当前对外可见的结果。
type Result struct {
	Value float64    `json:"value,omitempty"`
	Error *CellError `json:"error,omitempty"`
}

func valueResult(v float64) Result { return Result{Value: v} }
func errorResult(k ErrorKind, msg string) Result {
	return Result{Error: &CellError{Kind: k, Msg: msg}}
}

func sameResult(a, b Result) bool {
	if a.Error != nil || b.Error != nil {
		if a.Error == nil || b.Error == nil {
			return false
		}
		return a.Error.Kind == b.Error.Kind
	}
	return a.Value == b.Value
}

type cell struct {
	raw     string  // 保存的原始内容：空串=空，数字文本，=公式
	number  float64 // raw 为数字时的值
	formula Node    // raw 为公式时的语法树
	result  Result
}

// Workbook 是单工作表。并发访问需由调用方串行化（JSONL 服务天然串行）。
type Workbook struct {
	cells map[string]*cell
	// deps[a] = a 直接依赖的单元格集合（含 SUM 展开区域）。
	deps map[string]map[string]bool
}

// New 新建空工作簿。
func New() *Workbook {
	return &Workbook{
		cells: map[string]*cell{},
		deps:  map[string]map[string]bool{},
	}
}

// Raw 返回单元格原始内容（空、数字文本或 =公式）。
func (w *Workbook) Raw(addr string) string {
	if c := w.cells[addr]; c != nil {
		return c.raw
	}
	return ""
}

// Query 返回单元格当前结果；空单元格结果为 0 值。
func (w *Workbook) Query(addr string) Result {
	if c := w.cells[addr]; c != nil {
		return c.result
	}
	return valueResult(0)
}

// AllAddrs 返回全部非空单元格地址（排序由调用方按需处理）。
func (w *Workbook) AllAddrs() []string {
	out := make([]string, 0, len(w.cells))
	for addr := range w.cells {
		out = append(out, addr)
	}
	return out
}

// validateContent 校验单条内容并返回解析后的 cell；空串表示清空。
func validateContent(raw string) (*cell, error) {
	if raw == "" {
		return nil, nil
	}
	if raw[0] == '=' {
		if len(raw) == 1 {
			return nil, fmt.Errorf("公式为空：%q", raw)
		}
		node, err := ParseFormula(raw[1:])
		if err != nil {
			return nil, err
		}
		return &cell{raw: raw, formula: node}, nil
	}
	var value float64
	if err := parseFloat(raw, &value); err != nil {
		return nil, fmt.Errorf("内容 %q 既不是数字也不是 = 开头的公式", raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("内容 %q 不是有限数值", raw)
	}
	return &cell{raw: raw, number: value, result: valueResult(value)}, nil
}
