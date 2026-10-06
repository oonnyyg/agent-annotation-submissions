package main

import (
	"fmt"
	"sort"
)

// validateTiers 校验阶梯：至少一档、费率非负、上限递增、仅最后一档可不封顶。
func validateTiers(tiers []Tier) error {
	if len(tiers) == 0 {
		return fmt.Errorf("阶梯不能为空")
	}
	prev := int64(-1)
	for i, t := range tiers {
		if t.RateCents < 0 {
			return fmt.Errorf("第 %d 档单价不能为负", i+1)
		}
		if t.UpTo == nil {
			if i != len(tiers)-1 {
				return fmt.Errorf("只有最后一档可以不设上限")
			}
			continue
		}
		if *t.UpTo <= prev {
			return fmt.Errorf("第 %d 档上限必须大于前一档", i+1)
		}
		prev = *t.UpTo
	}
	return nil
}

// priceQty 按累进阶梯计算 qty 件的总额（分）。
// 每一档只对落在该档区间内的件数计价，不会把最后一档价格乘全部件数。
func priceQty(tiers []Tier, qty int64) int64 {
	var total int64
	var lower int64
	for _, t := range tiers {
		upper := qty
		if t.UpTo != nil && *t.UpTo < upper {
			upper = *t.UpTo
		}
		if upper > lower {
			total += (upper - lower) * t.RateCents
			lower = upper
		}
		if lower >= qty {
			break
		}
	}
	return total
}

// memberShare 用于按余数排序的中间结构。
type memberShare struct {
	id        string
	weight    int64
	base      int64
	remainder int64
}

// splitShares 按正整数权重分配 total 分：
// 先取各人份额的整数部分，剩余分按余数降序、同余数按员工编号字典序依次分完。
// 保证个人金额之和等于 total。
func splitShares(members map[string]int64, total int64) (map[string]int64, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("成员不能为空")
	}
	var weightSum int64
	shares := make([]memberShare, 0, len(members))
	for id, w := range members {
		if w <= 0 {
			return nil, fmt.Errorf("成员 %s 的权重必须为正整数", id)
		}
		weightSum += w
		shares = append(shares, memberShare{id: id, weight: w})
	}
	var assigned int64
	for i := range shares {
		exact := total * shares[i].weight
		shares[i].base = exact / weightSum
		shares[i].remainder = exact % weightSum
		assigned += shares[i].base
	}
	sort.Slice(shares, func(i, j int) bool {
		if shares[i].remainder != shares[j].remainder {
			return shares[i].remainder > shares[j].remainder
		}
		return shares[i].id < shares[j].id
	})
	left := total - assigned
	for i := int64(0); i < left; i++ {
		shares[i%int64(len(shares))].base++
	}
	out := make(map[string]int64, len(shares))
	for _, s := range shares {
		out[s.id] = s.base
	}
	return out, nil
}
