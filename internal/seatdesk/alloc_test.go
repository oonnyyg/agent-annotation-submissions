package seatdesk

import (
	"reflect"
	"testing"
)

func newHall(t *testing.T, rows, cols int, disabled []Seat) *Store {
	t.Helper()
	s := &Store{Orders: map[string]*Order{}}
	if err := s.InitHall(rows, cols, disabled); err != nil {
		t.Fatalf("InitHall: %v", err)
	}
	return s
}

func seatsOf(t *testing.T, s *Store, id string) []Seat {
	t.Helper()
	o, ok := s.Orders[id]
	if !ok {
		t.Fatalf("订单 %s 不存在", id)
	}
	return o.Seats
}

// 逐单贪心会让 A 先占首选列，导致 B 只能去远端；
// 整批最优应把 A 挪一列，让总代价更小。
func TestBatchBeatsGreedy(t *testing.T) {
	s := newHall(t, 1, 4, nil)
	orders, err := s.AllocateBatch([]Request{
		{ID: "A", Size: 1, Row: 1, Col: 3},
		{ID: "B", Size: 2, Row: 1, Col: 3},
	})
	if err != nil {
		t.Fatalf("AllocateBatch: %v", err)
	}
	total := 0
	for _, o := range orders {
		total += o.Cost
	}
	if total != 2 {
		t.Fatalf("总代价 = %d，期望 2（贪心会得到 3）", total)
	}
	if got := seatsOf(t, s, "A"); !reflect.DeepEqual(got, []Seat{{1, 2}}) {
		t.Fatalf("A 座位 = %v，期望 [{1 2}]", got)
	}
	if got := seatsOf(t, s, "B"); !reflect.DeepEqual(got, []Seat{{1, 3}, {1, 4}}) {
		t.Fatalf("B 座位 = %v，期望 [{1 3} {1 4}]", got)
	}
}

// 同代价时选择拼接坐标序列字典序最小者。
func TestTieBreakLexicographic(t *testing.T) {
	s := newHall(t, 1, 4, nil)
	if _, err := s.AllocateBatch([]Request{{ID: "A", Size: 2, Row: 1, Col: 3}}); err != nil {
		t.Fatalf("AllocateBatch: %v", err)
	}
	// {2,3} 与 {3,4} 代价同为 1，应取字典序更小的 {2,3}。
	if got := seatsOf(t, s, "A"); !reflect.DeepEqual(got, []Seat{{1, 2}, {1, 3}}) {
		t.Fatalf("A 座位 = %v，期望 [{1 2} {1 3}]", got)
	}
}

// 停用座位不能被跨过。
func TestDisabledNotCrossed(t *testing.T) {
	s := newHall(t, 1, 5, []Seat{{Row: 1, Col: 3}})
	if _, err := s.AllocateBatch([]Request{{ID: "A", Size: 3, Row: 1, Col: 3}}); err == nil {
		t.Fatal("跨越停用座位的分配应被拒绝")
	}
	if len(s.Orders) != 0 {
		t.Fatal("失败的批次不应留下任何占座")
	}
}

// 批内编号冲突或整批无解时全部拒绝。
func TestBatchAtomicReject(t *testing.T) {
	s := newHall(t, 1, 3, nil)
	_, err := s.AllocateBatch([]Request{
		{ID: "A", Size: 2, Row: 1, Col: 1},
		{ID: "A", Size: 1, Row: 1, Col: 3},
	})
	if err == nil {
		t.Fatal("批内编号冲突应被拒绝")
	}
	if len(s.Orders) != 0 {
		t.Fatal("冲突批次不应留下部分占座")
	}

	_, err = s.AllocateBatch([]Request{
		{ID: "B", Size: 2, Row: 1, Col: 1},
		{ID: "C", Size: 2, Row: 1, Col: 3},
	})
	if err == nil {
		t.Fatal("整批无解决方案应被拒绝")
	}
	if len(s.Orders) != 0 {
		t.Fatal("无解批次不应留下部分占座")
	}
}

// 换座失败时原订单内容与占座都保持不变；成功时允许复用原座。
func TestReseatFailureKeepsOriginal(t *testing.T) {
	s := newHall(t, 1, 3, nil)
	if _, err := s.AllocateBatch([]Request{
		{ID: "A", Size: 2, Row: 1, Col: 1},
		{ID: "B", Size: 1, Row: 1, Col: 3},
	}); err != nil {
		t.Fatalf("AllocateBatch: %v", err)
	}
	before := *s.Orders["A"]
	if _, err := s.Reseat("A", 3, 1, 2); err == nil {
		t.Fatal("换到 3 人应失败（B 占用第 3 列）")
	}
	if !reflect.DeepEqual(*s.Orders["A"], before) {
		t.Fatalf("换座失败后订单被改动: %+v -> %+v", before, *s.Orders["A"])
	}

	// 成功路径：复用自己的原座，其他订单不动。
	o, err := s.Reseat("A", 2, 1, 2)
	if err != nil {
		t.Fatalf("Reseat: %v", err)
	}
	if !reflect.DeepEqual(o.Seats, []Seat{{1, 1}, {1, 2}}) {
		t.Fatalf("换座后座位 = %v", o.Seats)
	}
	if got := seatsOf(t, s, "B"); !reflect.DeepEqual(got, []Seat{{1, 3}}) {
		t.Fatalf("B 座位被挪动: %v", got)
	}
}

// 释放后座位不再计入占用，可被后续订单使用。
func TestReleaseFreesSeats(t *testing.T) {
	s := newHall(t, 1, 2, nil)
	if _, err := s.AllocateBatch([]Request{{ID: "A", Size: 2, Row: 1, Col: 1}}); err != nil {
		t.Fatalf("AllocateBatch: %v", err)
	}
	if err := s.Release("A"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if occ := s.occupied(""); len(occ) != 0 {
		t.Fatalf("释放后仍有占用: %v", occ)
	}
	if _, err := s.AllocateBatch([]Request{{ID: "B", Size: 2, Row: 1, Col: 1}}); err != nil {
		t.Fatalf("释放后的座位应可再分配: %v", err)
	}
}
