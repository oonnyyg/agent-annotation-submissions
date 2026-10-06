package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// maxRow 限定单工作表地址范围 A1..Z100。
const maxCol = 26
const maxRow = 100

// Addr 是规范化后的单元格地址（列字母大写，如 A1）。
type Addr struct {
	Col int // 1..26
	Row int // 1..100
}

// String 返回规范地址文本。
func (a Addr) String() string {
	return fmt.Sprintf("%c%d", 'A'+a.Col-1, a.Row)
}

// ParseAddr 解析 A1..Z100，列字母大小写不敏感，输出规范大写形式。
func ParseAddr(s string) (Addr, error) {
	if len(s) < 2 {
		return Addr{}, fmt.Errorf("地址 %q 非法：应为 列字母+行号（如 A1）", s)
	}
	col := s[0]
	switch {
	case col >= 'A' && col <= 'Z':
	case col >= 'a' && col <= 'z':
		col -= 'a' - 'A'
	default:
		return Addr{}, fmt.Errorf("地址 %q 非法：列必须是 A-Z", s)
	}
	for _, ch := range s[1:] {
		if ch < '0' || ch > '9' {
			return Addr{}, fmt.Errorf("地址 %q 非法：行号必须全为数字", s)
		}
	}
	if len(s) > 2 && s[1] == '0' {
		return Addr{}, fmt.Errorf("地址 %q 非法：行号不允许前导零", s)
	}
	row, err := strconv.Atoi(s[1:])
	if err != nil || row < 1 || row > maxRow {
		return Addr{}, fmt.Errorf("地址 %q 非法：行号必须是 1-%d", s, maxRow)
	}
	return Addr{Col: int(col-'A') + 1, Row: row}, nil
}

// CellRange 表示一个上下界有序的矩形区域。
type CellRange struct {
	From Addr
	To   Addr
}

// ParseRange 解析形如 A1:B2 的矩形区域，要求左上界在前、右下界在后。
func ParseRange(s string) (CellRange, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return CellRange{}, fmt.Errorf("区域 %q 非法：应为 A1:B2 形式", s)
	}
	from, err := ParseAddr(parts[0])
	if err != nil {
		return CellRange{}, err
	}
	to, err := ParseAddr(parts[1])
	if err != nil {
		return CellRange{}, err
	}
	if from.Col > to.Col || from.Row > to.Row {
		return CellRange{}, fmt.Errorf("区域 %q 非法：左上界必须先于右下界", s)
	}
	return CellRange{From: from, To: to}, nil
}

// Cells 按行优先顺序枚举区域内全部地址。
func (r CellRange) Cells() []string {
	var out []string
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			out = append(out, Addr{Col: col, Row: row}.String())
		}
	}
	return out
}
