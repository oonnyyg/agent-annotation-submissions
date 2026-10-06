package main

import "testing"

func rollAll(t *testing.T, g *Game, pins ...int) {
	t.Helper()
	for _, p := range pins {
		if err := g.AddRoll(p); err != nil {
			t.Fatalf("AddRoll(%d): %v", p, err)
		}
	}
}

func gutterFrames(t *testing.T, g *Game, n int) {
	t.Helper()
	for i := 0; i < n*2; i++ {
		if err := g.AddRoll(0); err != nil {
			t.Fatalf("gutter: %v", err)
		}
	}
}

func TestPerfectGame(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	rollAll(t, g, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	sc := g.Score()
	if !sc.Complete {
		t.Fatal("expected complete")
	}
	if sc.Total == nil || *sc.Total != 300 {
		t.Fatalf("total = %v, want 300", sc.Total)
	}
	if len(sc.Frames) != 10 {
		t.Fatalf("frames = %d", len(sc.Frames))
	}
	// 已完成球局拒绝继续投球
	if err := g.AddRoll(1); err != ErrGameComplete {
		t.Fatalf("expected ErrGameComplete, got %v", err)
	}
}

func TestPendingScoresNotZeroFilled(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	rollAll(t, g, 10, 10, 3)
	sc := g.Score()
	// 第 1 轮全中需后两球（10,3）→ 23 确定
	if sc.Frames[0].Score == nil || *sc.Frames[0].Score != 23 {
		t.Fatalf("frame1 score = %v", sc.Frames[0].Score)
	}
	// 第 2 轮全中只有一球后续，待定
	if sc.Frames[1].Score != nil {
		t.Fatalf("frame2 should be pending, got %v", *sc.Frames[1].Score)
	}
	// 第 2 轮起累计分不得公布
	if sc.Frames[1].Cumulative != nil {
		t.Fatal("frame2 cumulative should be hidden")
	}
	if sc.Frames[0].Cumulative == nil || *sc.Frames[0].Cumulative != 23 {
		t.Fatalf("frame1 cumulative = %v", sc.Frames[0].Cumulative)
	}
	if sc.Total != nil {
		t.Fatal("incomplete game must not expose total")
	}
	if sc.Position.Frame != 3 || sc.Position.RollInFrame != 2 || sc.Position.PinsStanding != 7 {
		t.Fatalf("position = %+v", sc.Position)
	}
}

func TestSpareBonus(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	rollAll(t, g, 4, 6, 5)
	sc := g.Score()
	if sc.Frames[0].Score == nil || *sc.Frames[0].Score != 15 {
		t.Fatalf("spare score = %v", sc.Frames[0].Score)
	}
}

func TestTenthFrameStrikeBonus(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	gutterFrames(t, g, 9)
	rollAll(t, g, 10, 10, 10)
	sc := g.Score()
	if !sc.Complete || sc.Total == nil || *sc.Total != 30 {
		t.Fatalf("total = %v, complete=%v", sc.Total, sc.Complete)
	}
	if len(sc.Frames[9].Rolls) != 3 {
		t.Fatalf("tenth rolls = %v", sc.Frames[9].Rolls)
	}
}

func TestTenthFrameSpareBonus(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	gutterFrames(t, g, 9)
	rollAll(t, g, 7, 3, 5)
	sc := g.Score()
	if !sc.Complete || sc.Total == nil || *sc.Total != 15 {
		t.Fatalf("total = %v", sc.Total)
	}
	if err := g.AddRoll(1); err != ErrGameComplete {
		t.Fatalf("expected complete, got %v", err)
	}
}

func TestTenthFrameRackReset(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	gutterFrames(t, g, 9)
	rollAll(t, g, 10, 3)
	// 第十轮首球全中后重新摆瓶，第二球 3，第三球最多 7
	if err := g.AddRoll(8); err != ErrInvalidPins {
		t.Fatalf("expected ErrInvalidPins, got %v", err)
	}
	if err := g.AddRoll(7); err != nil {
		t.Fatal(err)
	}
	if !g.Score().Complete {
		t.Fatal("expected complete")
	}
}

func TestOpenFramePinsLimit(t *testing.T) {
	g := &Game{Player: "a", ID: "g1"}
	rollAll(t, g, 4)
	if err := g.AddRoll(7); err != ErrInvalidPins {
		t.Fatalf("expected ErrInvalidPins, got %v", err)
	}
	if err := g.AddRoll(-1); err != ErrInvalidPins {
		t.Fatalf("negative pins should fail, got %v", err)
	}
}
