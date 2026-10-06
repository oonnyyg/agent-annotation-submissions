package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Message is a single mail entry. UID is permanent and never reused
// within its mailbox; the sequence number is derived from position.
type Message struct {
	UID     uint64 `json:"uid"`
	Body    string `json:"body"`
	Seen    bool   `json:"seen"`
	Deleted bool   `json:"deleted"`
}

// Mailbox holds an ordered list of messages. NextUID only increases.
type Mailbox struct {
	Name     string    `json:"name"`
	NextUID  uint64    `json:"next_uid"`
	Messages []Message `json:"messages"`
}

// Store is the persistent root: a set of named mailboxes.
type Store struct {
	path      string
	Mailboxes map[string]*Mailbox `json:"mailboxes"`
}

var ErrMailboxExists = errors.New("mailbox already exists")
var ErrMailboxNotFound = errors.New("mailbox not found")

// LoadStore reads the store file, or returns an empty store when the
// file does not exist yet.
func LoadStore(path string) (*Store, error) {
	s := &Store{path: path, Mailboxes: map[string]*Mailbox{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("corrupt store file %s: %w", path, err)
	}
	s.path = path
	if s.Mailboxes == nil {
		s.Mailboxes = map[string]*Mailbox{}
	}
	for _, mb := range s.Mailboxes {
		if mb.NextUID == 0 {
			mb.NextUID = 1
		}
	}
	return s, nil
}

// Save writes the store atomically (temp file + rename).
func (s *Store) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".store-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

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

func (s *Store) MailboxByName(name string) (*Mailbox, error) {
	mb, ok := s.Mailboxes[name]
	if !ok {
		return nil, ErrMailboxNotFound
	}
	return mb, nil
}

// Append adds a message and returns its assigned UID.
func (mb *Mailbox) Append(body string, seen bool) uint64 {
	uid := mb.NextUID
	mb.NextUID++
	mb.Messages = append(mb.Messages, Message{UID: uid, Body: body, Seen: seen})
	return uid
}

// RemovedMessage identifies a message removed by expunge.
type RemovedMessage struct {
	Seq int    `json:"seq"`
	UID uint64 `json:"uid"`
}

// Expunge removes all messages flagged Deleted and reports the
// pre-expunge sequence numbers and UIDs of the removed messages.
func (mb *Mailbox) Expunge() []RemovedMessage {
	var removed []RemovedMessage
	kept := mb.Messages[:0]
	for i, m := range mb.Messages {
		if m.Deleted {
			removed = append(removed, RemovedMessage{Seq: i + 1, UID: m.UID})
		} else {
			kept = append(kept, m)
		}
	}
	for i := len(kept); i < len(mb.Messages); i++ {
		mb.Messages[i] = Message{}
	}
	mb.Messages = kept
	return removed
}

// MessageView is a message together with its current sequence number.
type MessageView struct {
	Seq     int    `json:"seq"`
	UID     uint64 `json:"uid"`
	Body    string `json:"body"`
	Seen    bool   `json:"seen"`
	Deleted bool   `json:"deleted"`
}

// Stats summarises a mailbox.
type Stats struct {
	Mailbox  string `json:"mailbox"`
	Messages int    `json:"messages"`
	Unseen   int    `json:"unseen"`
	Deleted  int    `json:"deleted"`
	NextUID  uint64 `json:"next_uid"`
}

func (mb *Mailbox) Stats() Stats {
	st := Stats{Mailbox: mb.Name, Messages: len(mb.Messages), NextUID: mb.NextUID}
	for _, m := range mb.Messages {
		if !m.Seen {
			st.Unseen++
		}
		if m.Deleted {
			st.Deleted++
		}
	}
	return st
}

// sortedUniqueIndexes normalises a set of 0-based indexes.
func sortedUniqueIndexes(indexes []int) []int {
	sort.Ints(indexes)
	out := indexes[:0]
	for i, v := range indexes {
		if i == 0 || v != indexes[i-1] {
			out = append(out, v)
		}
	}
	return out
}
