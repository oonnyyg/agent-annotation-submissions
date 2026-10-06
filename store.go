package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
)

// Store 保存所有球局，键为 player/game。
type Store struct {
	Games map[string]*Game `json:"games"`
	path  string
}

var ErrGameExists = errors.New("game already exists")
var ErrGameNotFound = errors.New("game not found")

func gameKey(player, id string) string { return player + "/" + id }

// LoadStore 从 JSONL 文件加载；文件不存在时返回空库。
func LoadStore(path string) (*Store, error) {
	s := &Store{Games: map[string]*Game{}, path: path}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var g Game
		if err := json.Unmarshal(line, &g); err != nil {
			return nil, err
		}
		s.Games[gameKey(g.Player, g.ID)] = &g
	}
	return s, sc.Err()
}

// Save 将全部球局写回 JSONL 文件（每行一局）。
func (s *Store) Save() error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, g := range s.Games {
		b, err := json.Marshal(g)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Create(player, id string) (*Game, error) {
	k := gameKey(player, id)
	if _, ok := s.Games[k]; ok {
		return nil, ErrGameExists
	}
	g := &Game{Player: player, ID: id}
	s.Games[k] = g
	return g, nil
}

func (s *Store) Get(player, id string) (*Game, error) {
	g, ok := s.Games[gameKey(player, id)]
	if !ok {
		return nil, ErrGameNotFound
	}
	return g, nil
}
