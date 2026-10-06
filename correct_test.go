package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCorrectionRecomputesAcrossFrames(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	// 第 1 轮 3/4，第 2 轮 5/2
	rollAll(t, g, 3, 4, 5, 2)
	sc := g.Score()
	if *sc.Frames[0].Score != 7 || *sc.Frames[0].Cumulative != 7 {
		t.Fatalf("before: %+v", sc.Frames[0])
	}
	// 把第 1 轮第二球 4 改成 7 → 补中，轮分变为 10+5=15，跨轮影响累计
	if err := g.Correct(1, 7); err != nil {
		t.Fatal(err)
	}
	sc = g.Score()
	if *sc.Frames[0].Score != 15 || *sc.Frames[0].Cumulative != 15 {
		t.Fatalf("after: %+v", sc.Frames[0])
	}
	if *sc.Frames[1].Cumulative != 22 {
		t.Fatalf("frame2 cumulative = %v", sc.Frames[1].Cumulative)
	}
	if len(g.History) != 1 || g.History[0].Old != 4 || g.History[0].New != 7 {
		t.Fatalf("history = %+v", g.History)
	}
}

func TestCorrectionRejectedKeepsOriginal(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	// 第 1 轮 2/3，第 2 轮 4/5
	rollAll(t, g, 2, 3, 4, 5)
	// 把第 1 球改成 8：则第二球 3 超过剩余 2 瓶 → 整个更正拒绝
	if err := g.Correct(0, 8); err != ErrInvalidPins {
		t.Fatalf("expected ErrInvalidPins, got %v", err)
	}
	want := []int{2, 3, 4, 5}
	for i, r := range g.Rolls {
		if r != want[i] {
			t.Fatalf("rolls mutated: %v", g.Rolls)
		}
	}
	if len(g.History) != 0 {
		t.Fatal("failed correction must not be recorded")
	}
}

func TestCorrectionToStrikeRevalidatesFollowing(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	rollAll(t, g, 3, 6, 2, 2)
	// 把第 1 球改为 10：原第 2 球 6 变成新一轮首球，后续仍合法
	if err := g.Correct(0, 10); err != nil {
		t.Fatal(err)
	}
	sc := g.Score()
	if len(sc.Frames[0].Rolls) != 1 || sc.Frames[0].Rolls[0] != 10 {
		t.Fatalf("frame1 = %v", sc.Frames[0].Rolls)
	}
}

func TestStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.jsonl")
	s, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("alice", "g1"); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Get("alice", "g1")
	rollAll(t, g, 10, 4, 6, 2)
	if err := g.Correct(1, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("bob", "g1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// 模拟重启
	s2, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := s2.Get("alice", "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(g2.Rolls) != 4 || len(g2.History) != 1 {
		t.Fatalf("restored game = %+v", g2)
	}
	// 多人多局互不影响
	if _, err := s2.Get("bob", "g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Get("alice", "nope"); err != ErrGameNotFound {
		t.Fatal("expected ErrGameNotFound")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
