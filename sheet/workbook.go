package sheet

import (
	"fmt"
	"math"
	"sort"
)

// 错误码：循环成员、循环下游/引用错误、除零、非有限结果。
const (
	ErrCycle = "#CYCLE!"
	ErrDep   = "#DEP!"
	ErrDiv0  = "#DIV/0!"
	ErrNum   = "#NUM!"
)

// Result 为单元格的计算结果：要么是有限数值，要么是错误码。
type Result struct {
	Value float64
	Err   string
}

func okResult(v float64) Result { return Result{Value: v} }
func errResult(e string) Result { return Result{Err: e} }

func (r Result) String() string {
	if r.Err != "" {
		return r.Err
	}
	return formatNum(r.Value)
}

func formatNum(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// Cell 保存原始内容、解析后的公式与最新结果。
type Cell struct {
	Raw    string
	Formula *Node
	Result Result
}

// Workbook 为单工作表。
type Workbook struct {
	cells map[Addr]*Cell
}

func New() *Workbook {
	return &Workbook{cells: map[Addr]*Cell{}}
}

// Get 返回单元格结果；空单元格按零。
func (wb *Workbook) Get(a Addr) Result {
	if c, ok := wb.cells[a]; ok {
		return c.Result
	}
	return okResult(0)
}

// Raw 返回单元格原始内容，未占用返回空串。
func (wb *Workbook) Raw(a Addr) string {
	if c, ok := wb.cells[a]; ok {
		return c.Raw
	}
	return ""
}

// Snapshot 以地址排序导出全部已占用单元格，用于保存。
func (wb *Workbook) Snapshot() map[Addr]Cell {
	out := make(map[Addr]Cell, len(wb.cells))
	for a, c := range wb.cells {
		out[a] = *c
	}
	return out
}

type evalErr struct{ code string }

func (wb *Workbook) evalNode(n *Node) (v float64, bad string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(evalErr); ok {
				bad = e.code
				return
			}
			panic(r)
		}
	}()
	return wb.evalRec(n), ""
}

func (wb *Workbook) evalRec(n *Node) float64 {
	switch n.kind {
	case ndNum:
		return n.num
	case ndRef:
		r := wb.Get(n.addr)
		if r.Err != "" {
			panic(evalErr{r.Err})
		}
		return r.Value
	case ndSum:
		sum := 0.0
		for _, a := range n.rng.Cells() {
			r := wb.Get(a)
			if r.Err != "" {
				panic(evalErr{r.Err})
			}
			sum += r.Value
		}
		if math.IsInf(sum, 0) || math.IsNaN(sum) {
			panic(evalErr{ErrNum})
		}
		return sum
	case ndBin:
		l := wb.evalRec(n.left)
		r := wb.evalRec(n.right)
		var v float64
		switch n.op {
		case '+':
			v = l + r
		case '-':
			v = l - r
		case '*':
			v = l * r
		case '/':
			if r == 0 {
				panic(evalErr{ErrDiv0})
			}
			v = l / r
		}
		if math.IsInf(v, 0) || math.IsNaN(v) {
			panic(evalErr{ErrNum})
		}
		return v
	}
	panic(evalErr{ErrNum})
}

// recalc 全量重算，返回结果发生变化的地址（旧结果 -> 新结果）。
func (wb *Workbook) recalc() map[Addr][2]Result {
	old := map[Addr]Result{}
	for a, c := range wb.cells {
		old[a] = c.Result
	}

	// 重置
	for _, c := range wb.cells {
		c.Result = Result{}
	}

	// 依赖：formula 地址 -> 它引用的地址集合
	deps := map[Addr]map[Addr]bool{}
	for a, c := range wb.cells {
		if c.Formula != nil {
			m := map[Addr]bool{}
			for _, r := range c.Formula.Refs() {
				m[r] = true
			}
			deps[a] = m
		}
	}

	cyclic := findCyclic(deps)

	// 循环成员直接标记
	for a := range cyclic {
		if c, ok := wb.cells[a]; ok {
			c.Result = errResult(ErrCycle)
		}
	}

	// 拓扑求值：反向依赖
	rdeps := map[Addr]map[Addr]bool{}
	for a, refs := range deps {
		for r := range refs {
			if rdeps[r] == nil {
				rdeps[r] = map[Addr]bool{}
			}
			rdeps[r][a] = true
		}
	}

	// 常量（含空单元格）视为已就绪；循环成员视为阻塞。
	state := map[Addr]int{} // 0 ready-like, 1 blocked
	var blocked map[Addr]bool
	_ = blocked

	// 收集所有相关地址（公式格 + 引用目标）
	all := map[Addr]bool{}
	for a, refs := range deps {
		all[a] = true
		for r := range refs {
			all[r] = true
		}
	}

	// 每个公式格剩余“未就绪依赖”数
	remain := map[Addr]int{}
	queue := []Addr{}
	ready := map[Addr]bool{}

	// 空/数值格总是就绪
	for a := range all {
		if c, ok := wb.cells[a]; !ok || c.Formula == nil {
			ready[a] = true
			queue = append(queue, a)
		}
	}
	for a := range cyclic {
		ready[a] = true // 以错误形态就绪
		queue = append(queue, a)
	}
	for a, refs := range deps {
		if cyclic[a] {
			continue
		}
		n := 0
		for r := range refs {
			if !ready[r] {
				n++
			}
		}
		remain[a] = n
		if n == 0 {
			queue = append(queue, a)
			ready[a] = true
		}
	}

	evaluated := map[Addr]bool{}
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		c, isCell := wb.cells[a]
		// 只对非循环公式格求值
		if isCell && c.Formula != nil && !cyclic[a] && !evaluated[a] {
			v, bad := wb.evalNode(c.Formula)
			if bad != "" {
				c.Result = errResult(bad)
			} else {
				c.Result = okResult(v)
			}
			evaluated[a] = true
		}
		for d := range rdeps[a] {
			if cyclic[d] || evaluated[d] {
				continue
			}
			if _, isFormula := deps[d]; !isFormula {
				continue
			}
			remain[d]--
			if remain[d] == 0 {
				ready[d] = true
				queue = append(queue, d)
			}
		}
	}

	// 未求值且非循环成员的公式格：依赖了循环 -> #DEP!
	for a := range deps {
		if cyclic[a] || evaluated[a] {
			continue
		}
		if c, ok := wb.cells[a]; ok {
			c.Result = errResult(ErrDep)
			evaluated[a] = true
		}
	}

	// 未占用格不产生结果；数值格结果即其值
	for _, c := range wb.cells {
		if c.Formula == nil && c.Raw != "" {
			c.Result = okResult(parseNum(c.Raw))
		}
	}

	changes := map[Addr][2]Result{}
	for a, c := range wb.cells {
		o, had := old[a]
		if !had {
			o = okResult(0)
		}
		if !resultEqual(o, c.Result) {
			changes[a] = [2]Result{o, c.Result}
		}
	}
	for a, o := range old {
		if _, ok := wb.cells[a]; !ok && !resultEqual(o, okResult(0)) {
			changes[a] = [2]Result{o, okResult(0)}
		}
	}
	return changes
}

func resultEqual(a, b Result) bool {
	if a.Err != b.Err {
		return false
	}
	if a.Err != "" {
		return true
}
	return a.Value == b.Value
}

func parseNum(raw string) float64 {
	v, _ := strconv.ParseFloat(raw, 64)
	return v
}

// SortedChanges 返回按地址排序的变化列表。
func SortedChanges(changes map[Addr][2]Result) []Addr {
	out := make([]Addr, 0, len(changes))
	for a := range changes {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
	if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		return out[i].Col < out[j].Col
	})
	return out
}

var _ = fmt.Sprintf
