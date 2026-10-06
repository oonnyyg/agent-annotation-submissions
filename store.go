package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Message is a single mail entry. UID is permanent and never reused within
// its mailbox; the sequence number is the 1-based position in Messages and
// is recomputed implicitly whenever messages are removed.
type Message struct {
	UID     uint64 `json:"uid"`
	Body    string `json:"body"`
	Seen    bool   `json:"seen"`
	Deleted bool   `json:"deleted"`
}

// Mailbox owns an independent UID counter.
type Mailbox struct {
	Name     string    `json:"name"`
	NextUID  uint64    `json:"next_uid"`
	Messages []Message `json:"messages"`
}

// Store is the persistent root state.
type Store struct {
	Mailboxes map[string]*Mailbox `json:"mailboxes"`
}

func NewStore() *Store {
	return &Store{Mailboxes: map[string]*Mailbox{}}
}

var ErrMailboxExists = errors.New("mailbox already exists")

func (s *Store) CreateMailbox(name string) error {
	if name == "" {
		return errors.New("mailbox name must not be empty")
	}
	if _, ok := s.Mailboxes[name]; ok {
		return ErrMailboxExists
	}
	s.Mailboxes[name] = &Mailbox{Name: name, NextUID: 1}
	return nil
}

func (s *Store) Mailbox(name string) (*Mailbox, error) {
	mb, ok := s.Mailboxes[name]
	if !ok {
		return nil, fmt.Errorf("mailbox %q does not exist", name)
	}
	return mb, nil
}

func (s *Store) MailboxNames() []string {
	names := make([]string, 0, len(s.Mailboxes))
	for n := range s.Mailboxes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Append assigns the next UID and returns it.
func (m *Mailbox) Append(body string) uint64 {
	uid := m.NextUID
	m.NextUID++
	m.Messages = append(m.Messages, Message{UID: uid, Body: body})
	return uid
}

// Expunge removes all messages flagged deleted and returns the removed
// messages together with their pre-expunge sequence numbers.
func (m *Mailbox) Expunge() []Message {
	var removed []Message
	kept := m.Messages[:0]
	for _, msg := range m.Messages {
		if msg.Deleted {
			removed = append(removed, msg)
		} else {
			kept = append(kept, msg)
		}
	}
	m.Messages = kept
	return removed
}

// LoadStore reads state from path; a missing file yields an empty store.
func LoadStore(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewStore(), nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", path, err)
	}
	if s.Mailboxes == nil {
		s.Mailboxes = map[string]*Mailbox{}
	}
	return &s, nil
}

// Save writes state atomically (temp file + rename) so a crash cannot
// leave a half-written state file.
func (s *Store) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// statePath resolves the state file location.
func statePath(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("MAILBOX_STATE"); env != "" {
		return env
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "mailbox-state.json")
}
