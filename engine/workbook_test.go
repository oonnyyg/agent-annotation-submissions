package engine

import "testing"

func mustApply(t *testing.T, w *Workbook, ops ...Op) []Change {
	t.Helper()
	changes, err := w.Apply(ops)
	if err != nil {
		t.Fatalf("Apply 出错: %v", err)
	}
	return changes
}

func TestArithmeticAndForwardRef(t *testing.T) {
	w := New()
	mustApply(t, w,
		Op{Addr: "C1", Content: "=A1+B1*2"}, // 前向引用，输入顺序无关
		Op{Addr: "A1", Content: "3"},
		Op{Addr: "B1", Content: "4"},
	)
	if r := w.Query("C1"); r.Error != nil || r.Value != 11 {
		t.Fatalf("C1 = %+v, 期望 11", r)
	}
	// 空单元格按零参与。
	mustApply(t, w, Op{Addr: "D1", Content: "=Z99+1"})
	if r := w.Query("D1"); r.Error != nil || r.Value != 1 {
		t.Fatalf("D1 = %+v, 期望 1", r)
	}
}

func TestSumRangeWithError(t *testing.T) {
	w := New()
	mustApply(t, w,
		Op{Addr: "A1", Content: "1"},
		Op{Addr: "A2", Content: "2"},
		Op{Addr: "A3", Content: "=SUM(A1:A2)"},
		Op{Addr: "B1", Content: "=1/0"},
		Op{Addr: "B2", Content: "=SUM(A1:A2)+SUM(B1:B1)"},
	)
	if r := w.Query("A3"); r.Error != nil || r.Value != 3 {
		t.Fatalf("A3 = %+v, 期望 3", r)
	}
	r := w.Query("B2")
	if r.Error == nil || r.Error.Kind != ErrDepend {
		t.Fatalf("B2 = %+v, 期望 DEPEND 错误", r)
	}
}

func TestCycleFormAndResolve(t *testing.T) {
	w := New()
	mustApply(t, w,
		Op{Addr: "A1", Content: "=B1+1"},
		Op{Addr: "B1", Content: "=A1+1"},
		Op{Addr: "C1", Content: "=A1"},
	)
	if r := w.Query("A1"); r.Error == nil || r.Error.Kind != ErrCycle {
		t.Fatalf("A1 = %+v, 期望 CYCLE", r)
	}
	if r := w.Query("C1"); r.Error == nil || r.Error.Kind != ErrDepend {
		t.Fatalf("C1 = %+v, 期望 DEPEND", r)
	}
	// 解除环后错误应恢复。
	mustApply(t, w, Op{Addr: "B1", Content: "1"})
	if r := w.Query("A1"); r.Error != nil || r.Value != 2 {
		t.Fatalf("A1 = %+v, 期望 2", r)
	}
	if r := w.Query("C1"); r.Error != nil || r.Value != 2 {
		t.Fatalf("C1 = %+v, 期望 2", r)
	}
}

func TestDivZeroRecoverAndNonFinite(t *testing.T) {
	w := New()
	mustApply(t, w, Op{Addr: "A1", Content: "=1/B1"}, Op{Addr: "B1", Content: "0"})
	if r := w.Query("A1"); r.Error == nil || r.Error.Kind != ErrDivZero {
		t.Fatalf("A1 = %+v, 期望 DIV0", r)
	}
	changes := mustApply(t, w, Op{Addr: "B1", Content: "4"})
	if r := w.Query("A1"); r.Error != nil || r.Value != 0.25 {
		t.Fatalf("A1 = %+v, 期望 0.25", r)
	}
	if len(changes) != 1 || changes[0].Addr != "A1" {
		t.Fatalf("变化集合应为 [A1], 实际 %+v", changes)
	}
}

func TestBatchRollbackOnError(t *testing.T) {
	w := New()
	mustApply(t, w, Op{Addr: "A1", Content: "10"})
	_, err := w.Apply([]Op{
		{Addr: "A1", Content: "20"},
		{Addr: "BAD", Content: "1"},
	})
	if err == nil {
		t.Fatal("坏地址应当导致整批失败")
	}
	if w.Raw("A1") != "10" {
		t.Fatalf("回滚失败，A1 被改成了 %q", w.Raw("A1"))
	}
	_, err = w.Apply([]Op{
		{Addr: "A2", Content: "=1+"},
		{Addr: "A2", Content: "2"},
	})
	if err == nil {
		t.Fatal("重复地址/语法错误应当失败")
	}
}
