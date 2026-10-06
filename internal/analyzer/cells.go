package analyzer

import (
	"sort"
	"strconv"
)

// Cell 是一个等价单元：每个字段取一个原子区间（枚举为单值，整数为闭区间）。
// 同一单元内所有输入的规则匹配结果完全一致。
type Cell struct {
	EnumIdx []int // 每个枚举字段的取值下标（按字段声明顺序，仅枚举字段有值）
	IntLo   []int // 每个整数字段区间的下界
	IntHi   []int // 每个整数字段区间的上界
}

// fieldCells 返回每个字段的原子区间列表。
// 枚举字段：每个值一个单元。整数字段：按所有规则边界切分。
func fieldCells(s *Spec) [][]struct{ lo, hi int } {
	result := make([][]struct{ lo, hi int }, len(s.Fields))
	for fi, f := range s.Fields {
		switch f.Type {
		case FieldEnum:
			cells := make([]struct{ lo, hi int }, len(f.Values))
			for i := range f.Values {
				cells[i] = struct{ lo, hi int }{i, i}
			}
			result[fi] = cells
		case FieldInt:
			cuts := map[int]bool{f.Min: true}
			for _, rule := range s.Rules {
				cond, ok := rule.When[f.Name]
				if !ok || cond.Range == nil {
					continue
				}
				lo, hi, err := NormalizeRange(&f, cond.Range)
				if err != nil {
					continue // 校验阶段已报告
				}
				if lo > f.Min {
					cuts[lo] = true
				}
				if hi+1 <= f.Max {
					cuts[hi+1] = true
				}
			}
			bounds := make([]int, 0, len(cuts))
			for c := range cuts {
				bounds = append(bounds, c)
			}
			sort.Ints(bounds)
			var cells []struct{ lo, hi int }
			for i, lo := range bounds {
				hi := f.Max
				if i+1 < len(bounds) {
					hi = bounds[i+1] - 1
				}
				cells = append(cells, struct{ lo, hi int }{lo, hi})
			}
			result[fi] = cells
		}
	}
	return result
}

// TotalCells 计算等价单元总数（提前溢出即返回，ok=false 表示超限）。
func TotalCells(s *Spec) (total int, limit int, ok bool) {
	fc := fieldCells(s)
	total = 1
	for _, cells := range fc {
		total *= len(cells)
		if total > s.CellLimit {
			return total, s.CellLimit, false
		}
	}
	return total, s.CellLimit, true
}

// EnumCells 按字段声明顺序、每字段按单元顺序枚举全部等价单元。
func EnumCells(s *Spec) []Cell {
	fc := fieldCells(s)
	var cells []Cell
	idx := make([]int, len(s.Fields))
	var walk func(fi int)
	walk = func(fi int) {
		if fi == len(s.Fields) {
			c := Cell{
				EnumIdx: make([]int, len(s.Fields)),
				IntLo:   make([]int, len(s.Fields)),
				IntHi:   make([]int, len(s.Fields)),
			}
			for i, f := range s.Fields {
				if f.Type == FieldEnum {
					c.EnumIdx[i] = idx[i]
				} else {
					c.IntLo[i] = fc[i][idx[i]].lo
					c.IntHi[i] = fc[i][idx[i]].hi
				}
			}
			cells = append(cells, c)
			return
		}
		for i := range fc[fi] {
			idx[fi] = i
			walk(fi + 1)
		}
	}
	walk(0)
	return cells
}

// Representative 返回单元内按“最小输入”顺序的代表输入：
// 枚举取该单元值（单元即单值），整数取区间下界。
func (s *Spec) Representative(c Cell) map[string]any {
	m := make(map[string]any, len(s.Fields))
	for i, f := range s.Fields {
		if f.Type == FieldEnum {
			m[f.Name] = f.Values[c.EnumIdx[i]]
		} else {
			m[f.Name] = c.IntLo[i]
		}
	}
	return m
}

// Describe 以字段声明顺序输出单元的可读边界描述。
func (s *Spec) Describe(c Cell) string {
	out := ""
	for i, f := range s.Fields {
		if i > 0 {
			out += ", "
		}
		if f.Type == FieldEnum {
			out += f.Name + "=" + f.Values[c.EnumIdx[i]]
		} else if c.IntLo[i] == c.IntHi[i] {
			out += f.Name + "=" + strconv.Itoa(c.IntLo[i])
		} else {
			out += f.Name + "∈[" + strconv.Itoa(c.IntLo[i]) + "," + strconv.Itoa(c.IntHi[i]) + "]"
		}
	}
	return out
}
