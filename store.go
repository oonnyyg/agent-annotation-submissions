package bowling

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrGameExists   = errors.New("game id already exists")
	ErrGameNotFound = errors.New("game not found")
)

// Store persists games as one JSON file per game in a directory.
type Store struct {
	Dir string
}

// NewStore creates the data directory if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func validID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if !ok {
			return false
		}
	}
	return true
}

func (s *Store) path(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("invalid game id %q", id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}

// Create stores a new game; the id must not exist yet.
func (s *Store) Create(g *Game) error {
	p, err := s.path(g.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return ErrGameExists
	}
	return s.Save(g)
}

// Save writes the game atomically (temp file + rename).
func (s *Store) Save(g *Game) error {
	p, err := s.path(g.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Load reads a game by id.
func (s *Store) Load(id string) (*Game, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrGameNotFound
	}
	if err != nil {
		return nil, err
	}
	var g Game
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// List returns summaries of all stored games, sorted by id.
func (s *Store) List() ([]*Game, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var games []*Game
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		g, err := s.Load(strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].ID < games[j].ID })
	return games, nil
}
