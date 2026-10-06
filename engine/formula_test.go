package engine

import "testing"

func TestParseAddr(t *testing.T) {
	cases := map[string]string{
		"A1":   "A1",
		"z99":  "Z99",
		"Z100": "Z100",
	}
	for in, want := range cases {
		addr, err := ParseAddr(in)
		if err != nil {
			t.Fatalf("ParseAddr(%q) 出错: %v", in, err)
		}
		if addr.String() != want {
			t.Fatalf("ParseAddr(%q) = %q, 期望 %q", in, addr.String(), want)
		}
	}
	bad := []string{"", "A", "AA1", "A0", "A101", "Z01", "1A", "A1.5"}
	for _, in := range bad {
		if _, err := ParseAddr(in); err == nil {
			t.Fatalf("ParseAddr(%q) 应当失败", in)
		}
	}
}

func TestParseRangeOrder(t *testing.T) {
	if _, err := ParseRange("B2:A1"); err == nil {
		t.Fatal("逆序区域 B2:A1 应当被拒绝")
	}
	rng, err := ParseRange("A1:B2")
	if err != nil {
		t.Fatalf("ParseRange 出错: %v", err)
	}
	cells := rng.Cells()
	want := []string{"A1", "B1", "A2", "B2"}
	if len(cells) != len(want) {
		t.Fatalf("区域单元格数 = %d, 期望 %d", len(cells), len(want))
	}
	for i := range want {
		if cells[i] != want[i] {
			t.Fatalf("区域顺序错误: %v", cells)
		}
	}
}

func TestParseFormulaDeps(t *testing.T) {
	node, err := ParseFormula("A1*2 + sum(B1:C2)/D4")
	if err != nil {
		t.Fatalf("ParseFormula 出错: %v", err)
	}
	got := map[string]bool{}
	for _, dep := range node.deps() {
		got[dep] = true
	}
	for _, want := range []string{"A1", "B1", "C1", "B2", "C2", "D4"} {
		if !got[want] {
			t.Fatalf("依赖中缺少 %s: %v", want, node.deps())
		}
	}
}

func TestParseFormulaErrors(t *testing.T) {
	bad := []string{
		"",
		"1 +",
		"(1+2",
		"1 2",
		"A1+",
		"FOO(1)",
		"SUM(A1)",
		"SUM(B2:A1)",
		"A101+1",
		"1/0", // 除零是运行期错误而非语法错误，这里实际合法
	}
	bad = bad[:len(bad)-1]
	for _, body := range bad {
		if _, err := ParseFormula(body); err == nil {
			t.Fatalf("公式 %q 应当解析失败", body)
		}
	}
}

func TestParseFormulaPrecedence(t *testing.T) {
	node, err := ParseFormula("1+2*3")
	if err != nil {
		t.Fatal(err)
	}
	bn, ok := node.(binaryNode)
	if !ok || bn.op != '+' {
		t.Fatal("顶层应为加法")
	}
	if _, ok := bn.right.(binaryNode); !ok {
		t.Fatal("乘法应作为右操作数嵌套")
	}
}
