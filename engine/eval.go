package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func parseFloat(s string, out *float64) error {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*out = v
	return nil
}

// recalc 全量重算所有公式单元格。
// 环成员标 CYCLE；其下游标 DEPEND；除零标 DIV0；非有限标 NUM；
// 其它失败传播为 DEPEND。失败绝不以零替代。
func (w *Workbook) recalc() {
	cycleSet := w.findCycleMembers()

	// 数字单元格结果固定；空单元格不存在于 map；公式单元格先重置。
	for addr, c := range w.cells {
		if c.formula != nil {
			c.result = Result{}
			_ = addr
		}
	}

	memo := map[string]Result{}
	var eval func(addr string, stack map[string]bool) Result
	eval = func(addr string, stack map[string]bool) Result {
		if r, ok := memo[addr]; ok {
			return r
		}
		c := w.cells[addr]
		if c == nil {
			// 空单元格按零参与计算。
			return valueResult(0)
		}
		if c.formula == nil {
			return c.result // 数字常量
		}
		if cycleSet[addr] {
			r := errorResult(ErrCycle, "单元格处于循环引用中")
			memo[addr] = r
			return r
		}
		if stack[addr] {
			// 理论上环成员已被 SCC 覆盖，防御性处理。
			r := errorResult(ErrCycle, "单元格处于循环引用中")
			memo[addr] = r
			return r
		}
		stack[addr] = true
		r := w.evalNode(c.formula, addr, stack, eval)
		delete(stack, addr)
		memo[addr] = r
		return r
	}

	addrs := make([]string, 0, len(w.cells))
	for addr := range w.cells {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	for _, addr := range addrs {
		c := w.cells[addr]
		if c.formula == nil {
			continue
		}
		c.result = eval(addr, map[string]bool{})
	}
}

func (w *Workbook) evalNode(node Node, owner string, stack map[string]bool,
	eval func(string, map[string]bool) Result) Result {
	switch n := node.(type) {
	case numberNode:
		return valueResult(float64(n))
	case refNode:
		r := eval(n.addr, stack)
		if r.Error != nil {
			return errorResult(ErrDepend, "依赖的 "+n.addr+" 存在错误")
		}
		return r
	case unaryNode:
		r := w.evalNode(n.operand, owner, stack, eval)
		if r.Error != nil {
			return errorResult(ErrDepend, "一元运算的操作数存在错误")
		}
		return valueResult(-r.Value)
	case binaryNode:
		l := w.evalNode(n.left, owner, stack, eval)
		if l.Error != nil {
			return errorResult(ErrDepend, "左操作数存在错误")
		}
		rr := w.evalNode(n.right, owner, stack, eval)
		if rr.Error != nil {
			return errorResult(ErrDepend, "右操作数存在错误")
		}
		x, y := l.Value, rr.Value
		var v float64
		switch n.op {
		case '+':
			v = x + y
		case '-':
			v = x - y
		case '*':
			v = x * y
		case '/':
			if y == 0 {
				return errorResult(ErrDivZero, fmt.Sprintf("%g/%g 除零", x, y))
			}
			v = x / y
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errorResult(ErrNum, fmt.Sprintf("运算结果 %s 非有限", formatFloat(v)))
		}
		return valueResult(v)
	case sumNode:
		var sum float64
		for _, dep := range n.cells {
			r := eval(dep, stack)
			if r.Error != nil {
				// 区域中的错误不能被求和忽略。
				return errorResult(ErrDepend, "SUM 区域内的 "+dep+" 存在错误")
			}
			sum += r.Value
		}
		if math.IsNaN(sum) || math.IsInf(sum, 0) {
			return errorResult(ErrNum, "SUM 结果非有限")
		}
		return valueResult(sum)
	}
	return errorResult(ErrDepend, "未知公式节点")
}

func formatFloat(v float64) string {
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	if math.IsInf(v, -1) {
		return "-Inf"
	}
	if math.IsNaN(v) {
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// findCycleMembers 用 Tarjan SCC 找出依赖图中所有真正处于环上的单元格。
func (w *Workbook) findCycleMembers() map[string]bool {
	index := 0
	indices := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	cycles := map[string]bool{}

	addrs := make([]string, 0, len(w.deps))
	for addr := range w.deps {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		neighbors := sortedDeps(w.deps[v])
		for _, u := range neighbors {
			if w.deps[u] == nil && w.cells[u] == nil {
				continue // 空/无公式单元格不会形成回边
			}
			if _, seen := indices[u]; !seen {
				if w.deps[u] == nil {
					continue
				}
				strongConnect(u)
				if low[u] < low[v] {
					low[v] = low[u]
				}
			} else if onStack[u] {
				if indices[u] < low[v] {
					low[v] = indices[u]
				}
			}
		}

		if low[v] == indices[v] {
			component := []string{}
			for {
				u := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[u] = false
				component = append(component, u)
				if u == v {
					break
				}
			}
			// SCC 大小 >1 即环；自环（v 依赖 v）也是环。
			if len(component) > 1 || w.deps[v][v] {
				for _, u := range component {
					cycles[u] = true
				}
			}
		}
	}

	for _, addr := range addrs {
		if _, seen := indices[addr]; !seen {
			strongConnect(addr)
		}
	}
	return cycles
}

func sortedDeps(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
