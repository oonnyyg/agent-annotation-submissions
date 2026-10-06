package payroll

import "testing"

func TestPriceProgressive(t *testing.T) {
	tiers := []Tier{
		{UpTo: 100, UnitPrice: 50},
		{UpTo: 200, UnitPrice: 80},
		{UpTo: 0, UnitPrice: 120},
	}
	cases := []struct {
		qty  int
		want int
	}{
		{1, 50},
		{100, 5000},              // 阶梯边界：恰好第一档
		{101, 5000 + 80},         // 跨档只对新件数用新价
		{200, 5000 + 8000},       // 第二档边界
		{250, 5000 + 8000 + 6000}, // 第三档
	}
	for _, c := range cases {
		got, err := Price(tiers, c.qty)
		if err != nil {
			t.Fatalf("qty %d: %v", c.qty, err)
		}
		if got != c.want {
			t.Errorf("qty %d: got %d want %d", c.qty, got, c.want)
		}
	}
	// 不能把最后一档价格乘全部件数
	got, _ := Price(tiers, 250)
	if got == 250*120 {
		t.Errorf("flat-rate bug: %d", got)
	}
}

func TestPriceErrors(t *testing.T) {
	if _, err := Price(nil, 10); err == nil {
		t.Error("empty tiers should fail")
	}
	if _, err := Price([]Tier{{UpTo: 10, UnitPrice: 5}}, 0); err == nil {
		t.Error("zero quantity should fail")
	}
	if _, err := Price([]Tier{{UpTo: 10, UnitPrice: 5}}, 20); err == nil {
		t.Error("uncovered quantity should fail")
	}
}

func TestDistributeRemainder(t *testing.T) {
	// 100 分按 1:1:1 分配：33/33/33 余 1，
	// 余数相同按员工编号字典序，E1 多得 1 分。
	got, err := Distribute(100, []MemberWeight{
		{EmpID: "E3", Weight: 1},
		{EmpID: "E1", Weight: 1},
		{EmpID: "E2", Weight: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["E1"] != 34 || got["E2"] != 33 || got["E3"] != 33 {
		t.Errorf("got %v", got)
	}
	sum := 0
	for _, v := range got {
		sum += v
	}
	if sum != 100 {
		t.Errorf("sum %d != 100", sum)
	}
}

func TestDistributeWeightedRemainder(t *testing.T) {
	// 总额 10，权重 1:1 -> 5/5 无余数
	got, _ := Distribute(10, []MemberWeight{{"E1", 1}, {"E2", 1}})
	if got["E1"] != 5 || got["E2"] != 5 {
		t.Errorf("got %v", got)
	}
	// 总额 7，权重 2:1 -> 4/2 余 1，E1 余数大得余分
	got, _ = Distribute(7, []MemberWeight{{"E1", 2}, {"E2", 1}})
	if got["E1"] != 5 || got["E2"] != 2 {
		t.Errorf("got %v", got)
	}
}

func TestDistributeInvalid(t *testing.T) {
	if _, err := Distribute(10, nil); err == nil {
		t.Error("no members should fail")
	}
	if _, err := Distribute(10, []MemberWeight{{"E1", 0}}); err == nil {
		t.Error("zero weight should fail")
	}
	if _, err := Distribute(10, []MemberWeight{{"E1", 1}, {"E1", 1}}); err == nil {
		t.Error("duplicate emp should fail")
	}
}
