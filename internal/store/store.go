package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Event is the single append-only record format stored as JSONL.
// All state changes in the system are expressed as one Event.
type Event struct {
	Seq       int              `json:"seq"`
	Type      string           `json:"type"`
	Stocktake string           `json:"stocktake,omitempty"`
	Location  string           `json:"location,omitempty"`
	SKU       string           `json:"sku,omitempty"`
	Qty       int64            `json:"qty,omitempty"`
	RecordID  string           `json:"record_id,omitempty"`
	Baseline  map[string]int64 `json:"baseline,omitempty"`
	Details   []CountDetail    `json:"details,omitempty"`
	Diff      map[string]int64 `json:"diff,omitempty"`
	Memo      string           `json:"memo,omitempty"`
}

// CountDetail is one sku line of a sealed stocktake.
type CountDetail struct {
	SKU      string `json:"sku"`
	Baseline int64  `json:"baseline"`
	Counted  int64  `json:"counted"`
	Diff     int64  `json:"diff"`
}

// Store is an append-only JSONL event log in a directory.
type Store struct {
	mu   sync.Mutex
	path string
	f    *os.File
	seq  int
}

const logName = "events.jsonl"

// Open opens (creating if needed) the event log in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	p := filepath.Join(dir, logName)
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	st := &Store{path: p, f: f}
	seq, err := st.scan()
	if err != nil {
		f.Close()
		return nil, err
	}
	st.seq = seq
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) scan() (int, error) {
	if _, err := s.f.Seek(0, 0); err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(s.f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	seq := 0
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return 0, fmt.Errorf("corrupt event log: %w", err)
		}
		if e.Seq <= seq {
			return 0, fmt.Errorf("event seq out of order: %d after %d", e.Seq, seq)
		}
		seq = e.Seq
	}
	return seq, sc.Err()
}

// Append writes one event, assigning the next sequence number.
func (s *Store) Append(e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	e.Seq = s.seq
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := s.f.Write(append(b, '\n')); err != nil {
		return err
	}
	return s.f.Sync()
}

// Close releases the underlying file.
func (s *Store) Close() error { return s.f.Close() }
