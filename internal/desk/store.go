package desk

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Event 是追加到 JSONL 日志中的一条事实记录。
type Event struct {
	Seq  int             `json:"seq"`
	Op   string          `json:"op"`
	Data json.RawMessage `json:"data"`
}

// Store 管理指定目录下的事件日志，并通过重放得到内存状态。
type Store struct {
	dir     string
	logPath string
	mu      sync.Mutex
	events  []Event
	state   *State
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, logPath: filepath.Join(dir, "events.jsonl")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	f, err := os.Open(s.logPath)
	if errors.Is(err, os.ErrNotExist) {
		s.state = NewState()
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("日志损坏: %w", err)
		}
		s.events = append(s.events, ev)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return s.replay()
}

func (s *Store) replay() error {
	st := NewState()
	for _, ev := range s.events {
		if err := st.apply(ev); err != nil {
			return fmt.Errorf("重放事件 seq=%d 失败: %w", ev.Seq, err)
		}
	}
	s.state = st
	return nil
}

// Append 校验并追加一个事件，返回应用后的结果。
func (s *Store) Append(op string, data any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	ev := Event{Seq: len(s.events) + 1, Op: op, Data: raw}
	res, err := s.state.applyResult(ev)
	if err != nil {
		return nil, err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	s.events = append(s.events, ev)
	return res, nil
}

func (s *Store) State() *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}
