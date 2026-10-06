package bowling

import "testing"

func TestCorrectionReScoresAcrossFrames(t *testing.T) {
	g := NewGame("g1", "fred")
	// Frame 1: 4+2 open; frame 2: 5.
	rollAll(t, g, 4, 2, 5)
	v, _ := g.Score()
	if *v.Frames[0].Score != 6 {
		t.Fatalf("want 6, got %d", *v.Frames[0].Score)
	}
	// Correct second roll of frame 1 to 6 -> spare, bonus from roll 3.
	if err := g.Correct(1, 6); err != nil {
		t.Fatal(err)
	}
	v, _ = g.Score()
	if *v.Frames[0].Score != 15 {
		t.Fatalf("spare after correction want 15, got %d", *v.Frames[0].Score)
	}
	if *v.Frames[0].Cumulative != 15 {
		t.Fatalf("cumulative want 15, got %d", *v.Frames[0].Cumulative)
	}
	if len(g.Corrections) != 1 || g.Corrections[0].OldPins != 2 || g.Corrections[0].NewPins != 6 {
		t.Fatalf("correction history missing: %+v", g.Corrections)
	}
}

func TestCorrectionInvalidatingLaterRollsRejected(t *testing.T) {
	g := NewGame("g1", "gina")
	rollAll(t, g, 3, 4, 10)
	// Raising the first roll to 8 leaves only 2 pins, breaking the 4.
	if err := g.Correct(0, 8); err != ErrInvalidPins {
		t.Fatalf("want ErrInvalidPins, got %v", err)
	}
	if g.Rolls[0] != 3 || len(g.Rolls) != 3 {
		t.Fatalf("game must be untouched: %v", g.Rolls)
	}
	if len(g.Corrections) != 0 {
		t.Fatal("failed correction must not be recorded")
	}
}

func TestCorrectionOutOfRange(t *testing.T) {
	g := NewGame("g1", "hank")
	rollAll(t, g, 1, 1)
	if err := g.Correct(2, 5); err != ErrRollOutOfRange {
		t.Fatalf("want ErrRollOutOfRange, got %v", err)
	}
}

func TestCorrectionStrikeChangesFrameBoundary(t *testing.T) {
	g := NewGame("g1", "ivy")
	// 3+3 then 4+2: two open frames.
	rollAll(t, g, 3, 3, 4, 2)
	// Correct first roll to a strike: frame 1 becomes strike, the rest
	// re-frames as 3, 4+2.
	if err := g.Correct(0, 10); err != nil {
		t.Fatal(err)
	}
	v, _ := g.Score()
	if len(v.Frames) != 3 {
		t.Fatalf("want 3 frames, got %d", len(v.Frames))
	}
	if *v.Frames[0].Score != 17 { // 10 + 3 + 4
		t.Fatalf("strike frame want 17, got %d", *v.Frames[0].Score)
	}
	if *v.Frames[1].Score != 7 { // 3+4 open frame
		t.Fatalf("frame 2 want 7, got %d", *v.Frames[1].Score)
	}
	if v.Frames[2].Score != nil { // single roll 2, pending
		t.Fatalf("frame 3 should be pending, got %v", *v.Frames[2].Score)
	}
}
