package seatdesk

import (
	"reflect"
	"testing"
)

// 保存后重新加载，影厅与订单状态应一致；已释放座位不算占用。
func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.InitHall(1, 4, []Seat{{Row: 1, Col: 4}}); err != nil {
		t.Fatalf("InitHall: %v", err)
	}
	if _, err := s.AllocateBatch([]Request{
		{ID: "A", Size: 2, Row: 1, Col: 1},
		{ID: "B", Size: 1, Row: 1, Col: 3},
	}); err != nil {
		t.Fatalf("AllocateBatch: %v", err)
	}
	if err := s.Release("B"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := Load(dir)
	if err != nil {
		t.Fatalf("重新 Load: %v", err)
	}
	if !reflect.DeepEqual(s2.Hall, s.Hall) {
		t.Fatalf("影厅状态不一致: %+v", s2.Hall)
	}
	if !reflect.DeepEqual(s2.Orders, s.Orders) {
		t.Fatalf("订单状态不一致: %+v", s2.Orders)
	}
	if occ := s2.occupied(""); len(occ) != 2 {
		t.Fatalf("重载后占用数 = %d，期望 2（B 已释放）", len(occ))
	}
	if _, err := s2.AllocateBatch([]Request{{ID: "C", Size: 1, Row: 1, Col: 3}}); err != nil {
		t.Fatalf("重载后已释放座位应可再分配: %v", err)
	}
}
