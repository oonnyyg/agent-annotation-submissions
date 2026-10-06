package main

import "testing"

func i64(v int64) *int64 { return &v }

var tierSet = []Tier{
	{UpTo: i64(100), RateCents: 10},
	{UpTo: i64(200), RateCents: 15},
	{UpTo: nil, RateCents: 20},
}

func TestPriceQtyBoundaries(t *testing.T) {
	cases := []struct {
		qty  int64
		want int64
	}{
		{0, 0},
		{1, 10},
		{100, 1000},          // 恰好第一档边界
		{101, 1000 + 15},     // 跨入第二档一件
		{200, 1000 + 1500},   // 恰好第二档边界
		{201, 2500 + 20},     // 跨入第三档一件
		{350, 2500 + 150*20}, // 最后一档只乘超出部分
	}
	for _, c := range cases {
		if got := priceQty(tierSet, c.qty); got != c.want {
			t.Errorf("priceQty(%d) = %d, want %d", c.qty, got, c.want)
		}
	}
}

func TestPriceQtyNotFlatRate(t *testing.T) {
	// 350 件若错误地用最后一档 20 分乘全部件数会得到 7000。
	if got := priceQty(tierSet, 350); got == 7000 {
		t.Fatalf("阶梯计价被错误地按最后一档乘全部件数: %d", got)
	}
}

func TestValidateTiers(t *testing.T) {
	if err := validateTiers(nil); err == nil {
		t.Error("空阶梯应被拒绝")
	}
	bad := []Tier{{UpTo: i64(50), RateCents: 10}, {UpTo: i64(50), RateCents: 20}}
	if err := validateTiers(bad); err == nil {
		t.Error("上限不递增应被拒绝")
	}
	mid := []Tier{{UpTo: nil, RateCents: 10}, {UpTo: i64(50), RateCents: 20}}
	if err := validateTiers(mid); err == nil {
		t.Error("非最后一档不封顶应被拒绝")
	}
	if err := validateTiers(tierSet); err != nil {
		t.Errorf("合法阶梯被误拒: %v", err)
	}
}

func TestSplitSharesRemainder(t *testing.T) {
	// 100 分按 1:1:1 分配：各 33 分余 1 分，余数相同按编号字典序分给 a01。
	got, err := splitShares(map[string]int64{"a01": 1, "a02": 1, "a03": 1}, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"a01": 34, "a02": 33, "a03": 33}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("shares[%s] = %d, want %d", k, got[k], v)
		}
	}
}

func TestSplitSharesSumEqualsTotal(t *testing.T) {
	members := map[string]int64{"e1": 3, "e2": 5, "e3": 7, "e4": 2}
	for _, total := range []int64{1, 7, 100, 999, 1234567} {
		got, err := splitShares(members, total)
		if err != nil {
			t.Fatal(err)
		}
		var sum int64
		for _, v := range got {
			sum += v
		}
		if sum != total {
			t.Errorf("total=%d 时个人金额之和为 %d", total, sum)
		}
	}
}

func TestSplitSharesRejectsBadWeight(t *testing.T) {
	if _, err := splitShares(map[string]int64{"a": 0}, 10); err == nil {
		t.Error("零权重应被拒绝")
	}
	if _, err := splitShares(map[string]int64{}, 10); err == nil {
		t.Error("空成员应被拒绝")
	}
}
