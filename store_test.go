package bowling

import "testing"

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	g := NewGame("g1", "alice")
	rollAll(t, g, 10, 3, 4)
	if err := s.Create(g); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(NewGame("g1", "bob")); err != ErrGameExists {
		t.Fatalf("want ErrGameExists, got %v", err)
	}
	// Simulate restart: a fresh Store over the same directory.
	s2, _ := NewStore(dir)
	loaded, err := s2.Load("g1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Player != "alice" || len(loaded.Rolls) != 3 {
		t.Fatalf("bad reload: %+v", loaded)
	}
	if err := loaded.Roll(10); err != nil {
		t.Fatalf("continue after restart: %v", err)
	}
	if _, err := s2.Load("missing"); err != ErrGameNotFound {
		t.Fatalf("want ErrGameNotFound, got %v", err)
	}
}

func TestStoreListIsolatesGames(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	a := NewGame("a", "alice")
	b := NewGame("b", "bob")
	rollAll(t, a, 10)
	rollAll(t, b, 1, 2)
	if err := s.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(b); err != nil {
		t.Fatal(err)
	}
	games, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 || games[0].ID != "a" || games[1].ID != "b" {
		t.Fatalf("bad list: %+v", games)
	}
	if len(games[0].Rolls) != 1 || len(games[1].Rolls) != 2 {
		t.Fatal("games must not share rolls")
	}
}
