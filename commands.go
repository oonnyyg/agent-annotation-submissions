package main

import (
	"fmt"
)

// Command is one JSONL request line.
type Command struct {
	Cmd     string     `json:"cmd"`
	Mailbox string     `json:"mailbox,omitempty"`
	To      string     `json:"to,omitempty"`
	Body    string     `json:"body,omitempty"`
	Select  *Selection `json:"select,omitempty"`
	Seen    *bool      `json:"seen,omitempty"`
	Deleted *bool      `json:"deleted,omitempty"`
}

// Response is one JSONL result line.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Result payloads (only the relevant ones are set).
	UID       uint64        `json:"uid,omitempty"`
	Seq       int           `json:"seq,omitempty"`
	Messages  []MessageInfo `json:"messages,omitempty"`
	Removed   []RemovedInfo `json:"removed,omitempty"`
	Copied    []CopyInfo    `json:"copied,omitempty"`
	Stats     *StatsInfo    `json:"stats,omitempty"`
	Mailboxes []string      `json:"mailboxes,omitempty"`
}

// MessageInfo reports both numbering schemes for a message.
type MessageInfo struct {
	Seq     int    `json:"seq"`
	UID     uint64 `json:"uid"`
	Seen    bool   `json:"seen"`
	Deleted bool   `json:"deleted"`
	Body    string `json:"body,omitempty"`
}

// RemovedInfo identifies an expunged message by its old sequence number
// and its permanent UID.
type RemovedInfo struct {
	OldSeq int    `json:"old_seq"`
	UID    uint64 `json:"uid"`
}

// CopyInfo maps a source UID to the newly assigned destination UID.
type CopyInfo struct {
	SrcUID uint64 `json:"src_uid"`
	DstUID uint64 `json:"dst_uid"`
}

// StatsInfo summarizes a mailbox.
type StatsInfo struct {
	Messages int    `json:"messages"`
	Unseen   int    `json:"unseen"`
	Deleted  int    `json:"deleted"`
	NextUID  uint64 `json:"next_uid"`
}

// Execute runs one command against the store. It reports whether the store
// was mutated (and therefore needs saving). All validation happens before
// any mutation, so a failed command never leaves partial changes.
func Execute(s *Store, c *Command) (resp Response, changed bool) {
	resp.OK = true
	var err error
	switch c.Cmd {
	case "create":
		err = s.CreateMailbox(c.Mailbox)
		changed = err == nil
	case "mailboxes":
		resp.Mailboxes = s.MailboxNames()
	case "append":
		var mb *Mailbox
		mb, err = s.Mailbox(c.Mailbox)
		if err == nil {
			resp.UID = mb.Append(c.Body)
			resp.Seq = len(mb.Messages)
			changed = true
		}
	case "list":
		resp.Messages, err = cmdList(s, c, false)
	case "fetch":
		resp.Messages, err = cmdList(s, c, true)
	case "store":
		changed, err = cmdStore(s, c)
	case "expunge":
		resp.Removed, changed, err = cmdExpunge(s, c)
	case "copy":
		resp.Copied, changed, err = cmdCopy(s, c)
	case "stats":
		resp.Stats, err = cmdStats(s, c)
	default:
		err = fmt.Errorf("unknown command %q", c.Cmd)
	}
	if err != nil {
		return Response{OK: false, Error: err.Error()}, false
	}
	return resp, changed
}

func messageInfo(mb *Mailbox, idx int, withBody bool) MessageInfo {
	m := mb.Messages[idx]
	info := MessageInfo{Seq: idx + 1, UID: m.UID, Seen: m.Seen, Deleted: m.Deleted}
	if withBody {
		info.Body = m.Body
	}
	return info
}

// cmdList implements "list" (metadata) and "fetch" (with bodies). Reading
// never changes the seen flag. Without a selection it covers all messages.
func cmdList(s *Store, c *Command, withBody bool) ([]MessageInfo, error) {
	mb, err := s.Mailbox(c.Mailbox)
	if err != nil {
		return nil, err
	}
	var indexes []int
	if c.Select != nil {
		indexes, err = resolveSelection(mb, c.Select)
		if err != nil {
			return nil, err
		}
	} else {
		for i := range mb.Messages {
			indexes = append(indexes, i)
		}
	}
	out := make([]MessageInfo, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, messageInfo(mb, i, withBody))
	}
	return out, nil
}

// cmdStore sets seen/deleted flags on the selected messages.
func cmdStore(s *Store, c *Command) (bool, error) {
	if c.Seen == nil && c.Deleted == nil {
		return false, fmt.Errorf("store requires at least one of \"seen\" or \"deleted\"")
	}
	mb, err := s.Mailbox(c.Mailbox)
	if err != nil {
		return false, err
	}
	if c.Select == nil {
		return false, fmt.Errorf("store requires a selection")
	}
	indexes, err := resolveSelection(mb, c.Select)
	if err != nil {
		return false, err
	}
	for _, i := range indexes {
		if c.Seen != nil {
			mb.Messages[i].Seen = *c.Seen
		}
		if c.Deleted != nil {
			mb.Messages[i].Deleted = *c.Deleted
		}
	}
	return true, nil
}

// cmdExpunge removes deleted messages; survivors are renumbered from 1.
func cmdExpunge(s *Store, c *Command) ([]RemovedInfo, bool, error) {
	mb, err := s.Mailbox(c.Mailbox)
	if err != nil {
		return nil, false, err
	}
	oldSeq := make(map[uint64]int, len(mb.Messages))
	for i, m := range mb.Messages {
		oldSeq[m.UID] = i + 1
	}
	removed := mb.Expunge()
	out := make([]RemovedInfo, 0, len(removed))
	for _, m := range removed {
		out = append(out, RemovedInfo{OldSeq: oldSeq[m.UID], UID: m.UID})
	}
	return out, len(removed) > 0, nil
}

// cmdCopy copies selected messages in source-sequence order into the
// destination mailbox, preserving bodies and flags. The destination gets
// fresh UIDs; the source is untouched. A missing destination mailbox
// invalidates the whole command before anything changes.
func cmdCopy(s *Store, c *Command) ([]CopyInfo, bool, error) {
	src, err := s.Mailbox(c.Mailbox)
	if err != nil {
		return nil, false, err
	}
	dst, err := s.Mailbox(c.To)
	if err != nil {
		return nil, false, fmt.Errorf("copy destination: %w", err)
	}
	if c.Select == nil {
		return nil, false, fmt.Errorf("copy requires a selection")
	}
	indexes, err := resolveSelection(src, c.Select)
	if err != nil {
		return nil, false, err
	}
	out := make([]CopyInfo, 0, len(indexes))
	for _, i := range indexes {
		m := src.Messages[i]
		uid := dst.Append(m.Body)
		dst.Messages[len(dst.Messages)-1].Seen = m.Seen
		dst.Messages[len(dst.Messages)-1].Deleted = m.Deleted
		out = append(out, CopyInfo{SrcUID: m.UID, DstUID: uid})
	}
	return out, len(out) > 0, nil
}

func cmdStats(s *Store, c *Command) (*StatsInfo, error) {
	mb, err := s.Mailbox(c.Mailbox)
	if err != nil {
		return nil, err
	}
	st := &StatsInfo{Messages: len(mb.Messages), NextUID: mb.NextUID}
	for _, m := range mb.Messages {
		if !m.Seen {
			st.Unseen++
		}
		if m.Deleted {
			st.Deleted++
		}
	}
	return st, nil
}
