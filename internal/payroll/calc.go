package payroll

import (
	"errors"
	"sort"
)

// Price 按累进阶梯计算 quantity 件的总额（分）。
// 每一档只对该档区间内的件数计价，不会把最后一档
// 单价乘全部件数。
func Price(tiers []Tier, quantity int) (int, error) {
	if quantity <= 0 {
		return 0, errors.New("quantity must be positive")
	}
	if len(tiers) == 0 {
		return 0, errors.New("scheme has no tiers")
	}
	total := 0
	lower := 0
	remaining := quantity
	for i, t := range tiers {
		if t.UnitPrice < 0 {
			return 0, errors.New("unit price must be non-negative")
		}
		last := i == len(tiers)-1
		if t.UpTo <= lower && !last {
			return 0, errors.New("tier bounds must be increasing")
		}
		upper := t.UpTo
		if last && upper <= lower {
			upper = quantity // 最后一档不限量
		}
		if remaining <= 0 {
			break
		}
		span := upper - lower
		if span > remaining {
			span = remaining
		}
		total += span * t.UnitPrice
		remaining -= span
		lower = upper
	}
	if remaining > 0 {
		return 0, errors.New("tiers do not cover quantity")
	}
	return total, nil
}

// Distribute 按正整数权重把 total 分分给各成员。
// 先取份额整数部分，再按余数降序分完剩余分，
// 同余数按员工编号字典序。返回每人金额，和恒等于 total。
func Distribute(total int, members []MemberWeight) (map[string]int, error) {
	if total < 0 {
		return nil, errors.New("total must be non-negative")
	}
	if len(members) == 0 {
		return nil, errors.New("batch must have members")
	}
	seen := map[string]bool{}
	sumW := 0
	for _, m := range members {
		if m.EmpID == "" {
			return nil, errors.New("emp_id must not be empty")
		}
		if m.Weight <= 0 {
			return nil, errors.New("weight must be positive")
		}
		if seen[m.EmpID] {
			return nil, errors.New("duplicate emp_id")
		}
		seen[m.EmpID] = true
		sumW += m.Weight
	}
	type rem struct {
		emp string
		r   int
	}
	out := map[string]int{}
	rems := make([]rem, 0, len(members))
	assigned := 0
	for _, m := range members {
		exact := total * m.Weight
		share := exact / sumW
		out[m.EmpID] = share
		assigned += share
		rems = append(rems, rem{emp: m.EmpID, r: exact % sumW})
	}
	sort.Slice(rems, func(i, j int) bool {
		if rems[i].r != rems[j].r {
			return rems[i].r > rems[j].r
		}
		return rems[i].emp < rems[j].emp
	})
	leftover := total - assigned
	for i := 0; i < leftover; i++ {
		out[rems[i%len(rems)].emp]++
	}
	return out, nil
}
