package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Command is one JSONL request line read from stdin.
type Command struct {
	Cmd     string     `json:"cmd"`
	Name    string     `json:"name"`    // mailbox name for create_mailbox
	Mailbox string     `json:"mailbox"` // source mailbox
	To      string     `json:"to"`      // target mailbox for copy
	Body    string     `json:"body"`
	Seen    *bool      `json:"seen"`    // append default / store flag
	Deleted *bool      `json:"deleted"` // store flag
	Set     *Selection `json:"set"`
}

// Response is one JSONL result line written to stdout.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	UID    uint64 `json:"uid,omitempty"`
	Seq    int    `json:"seq,omitempty"`
	*Stats `json:"stats,omitempty"`

	Messages  []MessageView    `json:"messages,omitempty"`
	Removed   []RemovedMessage `json:"removed,omitempty"`
	Copied    []CopyMapping    `json:"copied,omitempty"`
	Mailboxes []string         `json:"mailboxes,omitempty"`
}

// CopyMapping records where one copied message landed.
type CopyMapping struct {
	SrcSeq int    `json:"src_seq"`
	SrcUID uint64 `json:"src_uid"`
	DstSeq int    `json:"dst_seq"`
	DstUID uint64 `json:"dst_uid"`
}

// Execute runs one command against the store. Mutating commands are
// validated fully before any change is applied, and the store is saved
// after a successful mutation; a failed command changes nothing.
func Execute(s *Store, c *Command) Response {
	switch c.Cmd {
	case "create_mailbox":
		if err := s.CreateMailbox(c.Name); err != nil {
			return fail(err)
		}
		return save(s)
	case "list_mailboxes":
		names := make([]string, 0, len(s.Mailboxes))
		for name := range s.Mailboxes {
			names = append(names, name)
		}
		sort.Strings(names)
		return Response{OK: true, Mailboxes: names}
	case "append":
		mb, err := s.MailboxByName(c.Mailbox)
		if err != nil {
			return fail(err)
		}
		seen := c.Seen != nil && *c.Seen
		uid := mb.Append(c.Body, seen)
		if r := save(s); !r.OK {
			return r
		}
		return Response{OK: true, UID: uid, Seq: len(mb.Messages)}
	case "fetch":
		mb, indexes, err := resolveSet(s, c)
		if err != nil {
			return fail(err)
		}
		views := make([]MessageView, 0, len(indexes))
		for _, i := range indexes {
			m := mb.Messages[i]
			views = append(views, MessageView{
				Seq: i + 1, UID: m.UID, Body: m.Body,
				Seen: m.Seen, Deleted: m.Deleted,
			})
		}
		// Reading bodies never changes the seen flag.
		return Response{OK: true, Messages: views}
	case "store":
		mb, indexes, err := resolveSet(s, c)
		if err != nil {
			return fail(err)
		}
		if c.Seen == nil && c.Deleted == nil {
			return fail(fmt.Errorf("store requires \"seen\" and/or \"deleted\""))
		}
		for _, i := range indexes {
			if c.Seen != nil {
				mb.Messages[i].Seen = *c.Seen
			}
			if c.Deleted != nil {
				mb.Messages[i].Deleted = *c.Deleted
			}
		}
		return save(s)
	case "expunge":
		mb, err := s.MailboxByName(c.Mailbox)
		if err != nil {
			return fail(err)
		}
		removed := mb.Expunge()
		if r := save(s); !r.OK {
			return r
		}
		if removed == nil {
			removed = []RemovedMessage{}
		}
		return Response{OK: true, Removed: removed}
	case "copy":
		src, err := s.MailboxByName(c.Mailbox)
		if err != nil {
			return fail(err)
		}
		dst, err := s.MailboxByName(c.To)
		if err != nil {
			return fail(fmt.Errorf("copy target: %w", err))
		}
		if c.Set == nil {
			return fail(fmt.Errorf("copy requires \"set\""))
		}
		indexes, err := c.Set.resolve(src)
		if err != nil {
			return fail(err)
		}
		// Copy in source sequence order; bodies and flags are preserved,
		// the target assigns fresh UIDs. The source is untouched.
		mappings := make([]CopyMapping, 0, len(indexes))
		for _, i := range indexes {
			m := src.Messages[i]
			dstUID := dst.Append(m.Body, m.Seen)
			dst.Messages[len(dst.Messages)-1].Deleted = m.Deleted
			mappings = append(mappings, CopyMapping{
				SrcSeq: i + 1, SrcUID: m.UID,
				DstSeq: len(dst.Messages), DstUID: dstUID,
			})
		}
		if r := save(s); !r.OK {
			return r
		}
		return Response{OK: true, Copied: mappings}
	case "stats":
		mb, err := s.MailboxByName(c.Mailbox)
		if err != nil {
			return fail(err)
		}
		st := mb.Stats()
		return Response{OK: true, Stats: &st}
	default:
		return fail(fmt.Errorf("unknown cmd %q", c.Cmd))
	}
}

func resolveSet(s *Store, c *Command) (*Mailbox, []int, error) {
	mb, err := s.MailboxByName(c.Mailbox)
	if err != nil {
		return nil, nil, err
	}
	if c.Set == nil {
		return nil, nil, fmt.Errorf("command %q requires \"set\"", c.Cmd)
	}
	indexes, err := c.Set.resolve(mb)
	if err != nil {
		return nil, nil, err
	}
	return mb, indexes, nil
}

func fail(err error) Response {
	return Response{OK: false, Error: err.Error()}
}

func save(s *Store) Response {
	if err := s.Save(); err != nil {
		return fail(fmt.Errorf("save: %w", err))
	}
	return Response{OK: true}
}

// ParseCommand decodes one JSONL input line.
func ParseCommand(line []byte) (*Command, error) {
	var c Command
	if err := json.Unmarshal(line, &c); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &c, nil
}
