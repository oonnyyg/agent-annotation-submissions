package bowling

import "testing"

func rollAll(t *testing.T, g *Game, pins ...int) {
	t.Helper()
	for _, p := range pins {
		if err := g.Roll(p); err != nil {
			t.Fatalf("roll %d: %v", p, err)
		}
	}
}

func TestPerfectGame(t *testing.T) {
	g := NewGame("g1", "alice")
	rollAll(t, g, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	v, err := g.Score()
	if err != nil {
		t.Fatal(err)
	}
	if !v.Complete || v.Total == nil || *v.Total != 300 {
		t.Fatalf("want complete 300, got %+v", v)
	}
	if err := g.Roll(1); err != ErrGameComplete {
		t.Fatalf("want ErrGameComplete, got %v", err)
	}
}

func TestPendingStrikeNotZeroFilled(t *testing.T) {
	g := NewGame("g1", "alice")
	rollAll(t, g, 10)
	v, err := g.Score()
	if err != nil {
		t.Fatal(err)
	}
	if v.Frames[0].Score != nil || v.Frames[0].Cumulative != nil {
		t.Fatalf("strike frame must stay pending: %+v", v.Frames[0])
	}
	if v.Total != nil || v.Complete {
		t.Fatal("unfinished game must not publish a total")
	}
	if v.NextFrame != 2 || v.NextRoll != 1 {
		t.Fatalf("want next 2/1, got %d/%d", v.NextFrame, v.NextRoll)
	}
}

func TestSpareBonus(t *testing.T) {
	g := NewGame("g1", "bob")
	rollAll(t, g, 7, 3, 4)
	v, _ := g.Score()
	if v.Frames[0].Score == nil || *v.Frames[0].Score != 14 {
		t.Fatalf("spare frame want 14, got %+v", v.Frames[0].Score)
	}
	if v.Frames[1].Score != nil {
		t.Fatal("open frame with one roll must be pending")
	}
	if v.Frames[1].Cumulative != nil {
		t.Fatal("cumulative must stop at first pending frame")
	}
}

func TestTenthFrameSpareBonus(t *testing.T) {
	g := NewGame("g1", "carol")
	// Nine open frames of 1+1, then spare + bonus in the tenth.
	for i := 0; i < 9; i++ {
		rollAll(t, g, 1, 1)
	}
	rollAll(t, g, 4, 6)
	v, _ := g.Score()
	if v.Complete {
		t.Fatal("spare in tenth earns one bonus roll")
	}
	if v.NextFrame != 10 || v.NextRoll != 3 {
		t.Fatalf("want next 10/3, got %d/%d", v.NextFrame, v.NextRoll)
	}
	rollAll(t, g, 10) // fresh rack after spare
	v, _ = g.Score()
	if !v.Complete || *v.Total != 9*2+20 {
		t.Fatalf("want 38, got %+v", v.Total)
	}
}

func TestTenthFrameStrikeResetsRack(t *testing.T) {
	g := NewGame("g1", "dan")
	for i := 0; i < 9; i++ {
		rollAll(t, g, 0, 0)
	}
	rollAll(t, g, 10, 10, 10)
	v, _ := g.Score()
	if !v.Complete || *v.Total != 30 {
		t.Fatalf("want 30, got %+v", v.Total)
	}
	// Strike then partial rack: second roll 6 leaves 4 pins.
	g2 := NewGame("g2", "dan")
	for i := 0; i < 9; i++ {
		rollAll(t, g2, 0, 0)
	}
	rollAll(t, g2, 10, 6)
	if err := g2.Roll(5); err != ErrInvalidPins {
		t.Fatalf("want ErrInvalidPins, got %v", err)
	}
	rollAll(t, g2, 4)
	v2, _ := g2.Score()
	if !v2.Complete || *v2.Total != 20 {
		t.Fatalf("want 20, got %+v", v2.Total)
	}
}

func TestInvalidRollRejected(t *testing.T) {
	g := NewGame("g1", "erin")
	rollAll(t, g, 6)
	if err := g.Roll(5); err != ErrInvalidPins {
		t.Fatalf("want ErrInvalidPins, got %v", err)
	}
	if err := g.Roll(-1); err != ErrInvalidPins {
		t.Fatalf("want ErrInvalidPins, got %v", err)
	}
	if len(g.Rolls) != 1 {
		t.Fatalf("rejected rolls must not be recorded: %v", g.Rolls)
	}
}
