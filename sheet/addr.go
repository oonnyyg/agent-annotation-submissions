package sheet

import (
	"fmt"
	"strconv"
	"strings"
)

// MaxCol/MaxRow 限定工作表为 A1..Z100。
const (
	MaxCol = 26
	MaxRow = 100
)

// Addr 表示一个单元格地址，列从 0 开始（0=A），行从 0 开始（0=1）。
type Addr struct {
	Col int
	Row int
}

// ParseAddr 解析形如 "A1" 的地址，越界或非法均返回错误。
func ParseAddr(s string) (Addr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Addr{}, fmt.Errorf("空地址")
	}
	i := 0
	if s[i] < 'A' || s[i] > 'Z' {
		return Addr{}, fmt.Errorf("非法地址 %q", s)
	}
	col := int(s[i] - 'A')
	i++
	if i >= len(s) {
		return Addr{}, fmt.Errorf("非法地址 %q", s)
	}
	rowStr := s[i:]
	row, err := strconv.Atoi(rowStr)
	if err != nil || row < 1 {
		return Addr{}, fmt.Errorf("非法地址 %q", s)
	}
	row--
	if col >= MaxCol || row >= MaxRow {
		return Addr{}, fmt.Errorf("地址 %q 超出 A1:Z100", s)
	}
	return Addr{Col: col, Row: row}, nil
}

// String 返回规范地址，如 "A1"。
func (a Addr) String() string {
	return fmt.Sprintf("%c%d", 'A'+a.Col, a.Row+1)
}

// Range 表示一个矩形区域（含两端）。
type Range struct {
	From Addr
	To   Addr
}

// ParseRange 解析形如 "A1:B3" 的区域，要求起止有序（From 在 To 左上）。
func ParseRange(s string) (Range, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return Range{}, fmt.Errorf("非法区域 %q", s)
	}
	from, err := ParseAddr(parts[0])
	if err != nil {
		return Range{}, err
	}
	to, err := ParseAddr(parts[1])
	if err != nil {
		return Range{}, err
	}
	if from.Col > to.Col || from.Row > to.Row {
		return Range{}, fmt.Errorf("区域 %q 上下界无序", s)
	}
	return Range{From: from, To: to}, nil
}

// Cells 按行优先顺序枚举区域内全部单元格。
func (r Range) Cells() []Addr {
	var out []Addr
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			out = append(out, Addr{Col: col, Row: row})
		}
	}
	return out
}
